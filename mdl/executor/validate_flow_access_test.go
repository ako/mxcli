// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// CE0106, measured with mxbuild 11.14.0 on a fresh project (see the finding in
// .claude/skills/fix-issue/findings/mdl-executor/):
//
//	"At least one allowed role must be selected if the microflow is used from
//	 navigation, a page, a nanoflow or a published service."
//
// fires at security level Prototype AND Production, never at Off, for a flow
// with no allowed roles that is named by a page button, a page data source, a
// snippet, a navigation menu item, a menu document or a nanoflow (even an unused
// nanoflow / snippet / menu document). It does NOT fire for a microflow behind a
// published REST operation, one called only from another microflow, an unused
// one, or one referenced only from an EXCLUDED page.
//
// The reported shape: a flow rebuilt with `drop microflow` in one exec run and
// `create microflow` in a later one comes back with no access, while the page
// that calls it is still there — and nothing before mxbuild says so.

// flowAccessFixture is a project with module Shop holding role Shop.Admin and a
// page Shop.P_Main whose button calls Shop.MF_Button.
type flowAccessFixture struct {
	level      string
	moduleRole []*security.ModuleRole
	mfs        []*microflows.Microflow
	nfs        []*microflows.Nanoflow
	units      []*types.RawUnit
}

func newFlowAccessFixture() *flowAccessFixture {
	return &flowAccessFixture{
		level:      security.SecurityLevelProduction,
		moduleRole: []*security.ModuleRole{{Name: "Admin"}},
	}
}

var flowAccessShop = &model.Module{BaseElement: model.BaseElement{ID: "mod-shop"}, Name: "Shop"}

func (f *flowAccessFixture) addMicroflow(name string, roles ...string) {
	mf := mkMicroflow(flowAccessShop.ID, name)
	for _, r := range roles {
		mf.AllowedModuleRoles = append(mf.AllowedModuleRoles, model.ID(r))
	}
	f.mfs = append(f.mfs, mf)
}

// addPage stores a page whose button calls the microflow (and whose data view is
// fed by dsFlow when given).
func (f *flowAccessFixture) addPage(name, callsMicroflow string, excluded bool) {
	doc := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Name", Value: name},
		{Key: "Excluded", Value: excluded},
		{Key: "FormCall", Value: bson.D{
			{Key: "$Type", Value: "Forms$LayoutCall"},
			{Key: "Arguments", Value: bson.A{int32(2), bson.D{
				{Key: "$Type", Value: "Forms$FormCallArgument"},
				{Key: "Widgets", Value: bson.A{int32(2), bson.D{
					{Key: "$Type", Value: "Forms$ActionButton"},
					{Key: "Action", Value: bson.D{
						{Key: "$Type", Value: "Forms$MicroflowAction"},
						{Key: "MicroflowSettings", Value: bson.D{
							{Key: "$Type", Value: "Forms$MicroflowSettings"},
							{Key: "Microflow", Value: callsMicroflow},
						}},
					}},
				}}},
			}}},
		}},
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		panic(err)
	}
	f.units = append(f.units, &types.RawUnit{
		ID: nextID("pg"), ContainerID: flowAccessShop.ID, Type: "Forms$Page", Contents: raw,
	})
}

func (f *flowAccessFixture) ctx(t *testing.T) *ExecContext {
	t.Helper()
	ms := &security.ModuleSecurity{
		BaseElement: model.BaseElement{ID: "ms-shop"},
		ContainerID: flowAccessShop.ID,
		ModuleRoles: f.moduleRole,
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{flowAccessShop}, nil },
		GetModuleSecurityFunc: func(id model.ID) (*security.ModuleSecurity, error) {
			if id == flowAccessShop.ID {
				return ms, nil
			}
			return nil, nil
		},
		GetProjectSecurityFunc: func() (*security.ProjectSecurity, error) {
			return &security.ProjectSecurity{SecurityLevel: f.level}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return f.mfs, nil },
		ListNanoflowsFunc:  func() ([]*microflows.Nanoflow, error) { return f.nfs, nil },
		ListRawUnitsByTypeFunc: func(prefix string) ([]*types.RawUnit, error) {
			var out []*types.RawUnit
			for _, u := range f.units {
				if strings.HasPrefix(u.Type, prefix) {
					out = append(out, u)
				}
			}
			return out, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(flowAccessShop)))
	return ctx
}

func checkFlowAccessSrc(t *testing.T, f *flowAccessFixture, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	var out []linter.Violation
	for _, v := range CheckFlowAccess(f.ctx(t), prog) {
		if v.RuleID == flowAccessRule {
			out = append(out, v)
		}
	}
	return out
}

// The reporter's case: the page is in the project, the microflow was dropped by
// an earlier run, and this run creates it again in a module that has roles.
func TestFlowAccess_RecreatedFlowUsedByStoredPage(t *testing.T) {
	f := newFlowAccessFixture()
	f.addPage("P_Main", "Shop.MF_Button", false)

	got := checkFlowAccessSrc(t, f, "create microflow Shop.MF_Button () begin end;")
	if len(got) != 1 {
		t.Fatalf("want 1 %s violation, got %d: %+v", flowAccessRule, len(got), got)
	}
	v := got[0]
	if v.Severity != linter.SeverityError {
		t.Errorf("security level Production builds this to CE0106; want error, got %v", v.Severity)
	}
	for _, want := range []string{"Shop.MF_Button", "CE0106", "page Shop.P_Main"} {
		if !strings.Contains(v.Message, want) {
			t.Errorf("message lacks %q: %s", want, v.Message)
		}
	}
	for _, want := range []string{"grant execute on microflow Shop.MF_Button to Shop.Admin", "create or modify"} {
		if !strings.Contains(v.Suggestion, want) {
			t.Errorf("suggestion lacks %q: %s", want, v.Suggestion)
		}
	}
}

// Control for the case above: the same script with the grant is clean.
func TestFlowAccess_GrantInScriptIsClean(t *testing.T) {
	f := newFlowAccessFixture()
	f.addPage("P_Main", "Shop.MF_Button", false)
	got := checkFlowAccessSrc(t, f, `create microflow Shop.MF_Button () begin end;
grant execute on microflow Shop.MF_Button to Shop.Admin;`)
	if len(got) != 0 {
		t.Fatalf("granted flow reported: %+v", got)
	}
}

// Prototype is CE0106 too (measured), Off is not: there the advice is a warning.
func TestFlowAccess_SeverityFollowsSecurityLevel(t *testing.T) {
	for level, want := range map[string]linter.Severity{
		security.SecurityLevelPrototype:  linter.SeverityError,
		security.SecurityLevelProduction: linter.SeverityError,
		security.SecurityLevelOff:        linter.SeverityWarning,
	} {
		f := newFlowAccessFixture()
		f.level = level
		f.addPage("P_Main", "Shop.MF_Button", false)
		got := checkFlowAccessSrc(t, f, "create microflow Shop.MF_Button () begin end;")
		if len(got) != 1 || got[0].Severity != want {
			t.Errorf("level %s: want one violation at %v, got %+v", level, want, got)
		}
	}
	// A script that raises the level itself is judged at the level it sets.
	f := newFlowAccessFixture()
	f.level = security.SecurityLevelOff
	f.addPage("P_Main", "Shop.MF_Button", false)
	got := checkFlowAccessSrc(t, f, `alter app security ( SecurityLevel: prototype );
create microflow Shop.MF_Button () begin end;`)
	if len(got) != 1 || got[0].Severity != linter.SeverityError {
		t.Errorf("script raising the level: want one error, got %+v", got)
	}
}

// `create or modify`, and drop + create in ONE script, keep the stored roles —
// exec carries them (preserveAllowedRoles / consumeDroppedMicroflow).
func TestFlowAccess_RewritesThatKeepRolesAreClean(t *testing.T) {
	for name, src := range map[string]string{
		"create or modify":   "create or modify microflow Shop.MF_Button () begin end;",
		"drop+create in one": "drop microflow Shop.MF_Button;\ncreate microflow Shop.MF_Button () begin end;",
	} {
		f := newFlowAccessFixture()
		f.addPage("P_Main", "Shop.MF_Button", false)
		f.addMicroflow("MF_Button", "Shop.Admin")
		if got := checkFlowAccessSrc(t, f, src); len(got) != 0 {
			t.Errorf("%s: roles are carried, nothing to report; got %+v", name, got)
		}
	}
}

// Revoking the last role is the same defect reached a different way.
func TestFlowAccess_RevokeLastRole(t *testing.T) {
	f := newFlowAccessFixture()
	f.addPage("P_Main", "Shop.MF_Button", false)
	f.addMicroflow("MF_Button", "Shop.Admin")
	got := checkFlowAccessSrc(t, f, "revoke execute on microflow Shop.MF_Button from Shop.Admin;")
	if len(got) != 1 {
		t.Fatalf("want 1 violation, got %+v", got)
	}
}

// Uses measured NOT to raise CE0106 are not reported: an excluded page, a
// microflow caller, no caller at all.
func TestFlowAccess_UsesThatDoNotNeedRoles(t *testing.T) {
	f := newFlowAccessFixture()
	f.addPage("P_Excl", "Shop.MF_Button", true)
	if got := checkFlowAccessSrc(t, f, "create microflow Shop.MF_Button () begin end;"); len(got) != 0 {
		t.Errorf("excluded page: got %+v", got)
	}

	f = newFlowAccessFixture()
	got := checkFlowAccessSrc(t, f, `create microflow Shop.MF_Inner () begin end;
create microflow Shop.MF_Outer () begin call microflow Shop.MF_Inner(); end;`)
	if len(got) != 0 {
		t.Errorf("microflow-only caller / unused: got %+v", got)
	}
}

// A page, snippet, nanoflow or navigation the SCRIPT writes is a use as much as
// a stored one, and a nanoflow is held to the same rule.
func TestFlowAccess_ScriptUses(t *testing.T) {
	f := newFlowAccessFixture()
	got := checkFlowAccessSrc(t, f, `create microflow Shop.MF_A () begin end;
create microflow Shop.MF_B () begin end;
create microflow Shop.MF_C () begin end;
create microflow Shop.MF_D () begin end;
create nanoflow Shop.NF_E () begin call microflow Shop.MF_C(); end;
grant execute on nanoflow Shop.NF_E to Shop.Admin;
create nanoflow Shop.NF_F () begin end;
create page Shop.P_New ( Title: 'New', Layout: Atlas_Core.Atlas_Default ) {
  actionbutton b1 (caption: 'A', action: call microflow Shop.MF_A)
  actionbutton b2 (caption: 'F', action: call nanoflow Shop.NF_F)
};
create snippet Shop.Snip {
  actionbutton b3 (caption: 'B', action: call microflow Shop.MF_B)
};
create or modify navigation Responsive
  home page Shop.P_New
{
  menu item 'D' ( OnClick: call microflow Shop.MF_D )
};`)
	flagged := map[string]bool{}
	for _, v := range got {
		flagged[v.Location.DocumentName] = true
	}
	for _, want := range []string{"MF_A", "MF_B", "MF_C", "MF_D", "NF_F"} {
		if !flagged[want] {
			t.Errorf("%s has no roles and is used; not reported (got %+v)", want, got)
		}
	}
	if len(got) != 5 {
		t.Errorf("want exactly 5 violations, got %d: %+v", len(got), got)
	}
}

// A module with no roles gets the auto-created <Module>.User role on every new
// document, so its flows are never without access.
func TestFlowAccess_ModuleWithoutRolesGetsAutoRole(t *testing.T) {
	f := newFlowAccessFixture()
	f.moduleRole = nil
	f.addPage("P_Main", "Shop.MF_Button", false)
	if got := checkFlowAccessSrc(t, f, "create microflow Shop.MF_Button () begin end;"); len(got) != 0 {
		t.Errorf("auto-granted flow reported: %+v", got)
	}
	// ...unless the script gives the module a real role first, which switches
	// the default off.
	f = newFlowAccessFixture()
	f.moduleRole = nil
	f.addPage("P_Main", "Shop.MF_Button", false)
	got := checkFlowAccessSrc(t, f, `create module role Shop.Manager;
create microflow Shop.MF_Button () begin end;`)
	if len(got) != 1 || !strings.Contains(got[0].Suggestion, "Shop.Manager") {
		t.Errorf("want one violation suggesting Shop.Manager, got %+v", got)
	}
}

// A defect the project already has, which this script neither causes nor
// touches, belongs to lint, not to a check of the script.
func TestFlowAccess_PreexistingNotReported(t *testing.T) {
	f := newFlowAccessFixture()
	f.addPage("P_Main", "Shop.MF_Button", false)
	f.addMicroflow("MF_Button")
	if got := checkFlowAccessSrc(t, f, "create microflow Shop.MF_Other () begin end;"); len(got) != 0 {
		t.Errorf("pre-existing violation reported against an unrelated script: %+v", got)
	}
}
