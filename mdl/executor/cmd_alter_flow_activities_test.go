// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

const (
	salesModID = model.ID("mod-sales")
	otherModID = model.ID("mod-other")
)

// flowFixture wires a two-module project with one microflow in each, and
// records the SetActivitiesDisabled calls the handler makes.
type flowFixture struct {
	calls  []model.ID
	result map[model.ID]backend.FlowActivityChange
}

func newFlowFixture(t *testing.T, result map[model.ID]backend.FlowActivityChange) (*ExecContext, *flowFixture, *strings.Builder) {
	t.Helper()
	fx := &flowFixture{result: result}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{
				{BaseElement: model.BaseElement{ID: salesModID}, Name: "Sales"},
				{BaseElement: model.BaseElement{ID: otherModID}, Name: "Other"},
			}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				{BaseElement: model.BaseElement{ID: "mf-a"}, ContainerID: salesModID, Name: "ACT_A"},
				{BaseElement: model.BaseElement{ID: "mf-b"}, ContainerID: otherModID, Name: "ACT_B"},
			}, nil
		},
		SetActivitiesDisabledFunc: func(unitID model.ID, _ types.ActivityFilter, _ bool) (backend.FlowActivityChange, error) {
			fx.calls = append(fx.calls, unitID)
			return fx.result[unitID], nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(
		&model.Module{BaseElement: model.BaseElement{ID: salesModID}, Name: "Sales"},
		&model.Module{BaseElement: model.BaseElement{ID: otherModID}, Name: "Other"},
	)))
	var sb strings.Builder
	_ = buf
	ctx.Output = &sb
	return ctx, fx, &sb
}

func logDebugFilter() types.ActivityFilter {
	return types.ActivityFilter{Conditions: []types.ActivityCondition{
		{Column: types.ActivityColAction, Values: []string{"log"}},
		{Column: types.ActivityColLevel, Values: []string{"debug"}},
	}}
}

// `IN <module>` scopes the statement, and the whole point of the bulk form is
// that it does not reach outside it.
func TestAlterFlowActivitiesScopesToTheModule(t *testing.T) {
	ctx, fx, out := newFlowFixture(t, map[model.ID]backend.FlowActivityChange{
		"mf-a": {Changed: 2, Matched: 2},
		"mf-b": {Changed: 9, Matched: 9},
	})
	err := execAlterFlowActivities(ctx, &ast.AlterFlowActivitiesStmt{
		Flavour: ast.FlowMicroflow, Bulk: true, Module: "Sales", Disable: true, Filter: logDebugFilter(),
	})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(fx.calls) != 1 || fx.calls[0] != "mf-a" {
		t.Fatalf("touched %v; a module-scoped statement must reach only Sales", fx.calls)
	}
	if got := out.String(); !strings.Contains(got, "Disabled 2") || !strings.Contains(got, "Sales.ACT_A") {
		t.Errorf("report does not name what changed: %q", got)
	}
}

// Without IN the scope is the whole project — the same rule as
// ALTER PAGES … SET LAYOUT.
func TestAlterFlowActivitiesWithoutModuleCoversTheProject(t *testing.T) {
	ctx, fx, _ := newFlowFixture(t, map[model.ID]backend.FlowActivityChange{
		"mf-a": {Changed: 1, Matched: 1}, "mf-b": {Changed: 1, Matched: 1},
	})
	if err := execAlterFlowActivities(ctx, &ast.AlterFlowActivitiesStmt{
		Flavour: ast.FlowMicroflow, Bulk: true, Disable: true, Filter: logDebugFilter(),
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	if len(fx.calls) != 2 {
		t.Errorf("touched %d flows, want 2 (the whole project)", len(fx.calls))
	}
}

// The three outcomes that all write nothing but mean different things. Merging
// any two of them is how a typo'd filter reads as a successful no-op.
func TestAlterFlowActivitiesDistinguishesItsThreeOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result backend.FlowActivityChange
		want   string
	}{
		{"nothing matched", backend.FlowActivityChange{}, "No activity matched"},
		{"already in that state", backend.FlowActivityChange{Matched: 3}, "Unchanged"},
		{"a real change", backend.FlowActivityChange{Matched: 3, Changed: 3}, "Disabled 3"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _, out := newFlowFixture(t, map[model.ID]backend.FlowActivityChange{"mf-a": tc.result})
			if err := execAlterFlowActivities(ctx, &ast.AlterFlowActivitiesStmt{
				Flavour: ast.FlowMicroflow,
				Name:    ast.QualifiedName{Module: "Sales", Name: "ACT_A"},
				Disable: true, Filter: logDebugFilter(),
			}); err != nil {
				t.Fatalf("exec: %v", err)
			}
			if got := out.String(); !strings.Contains(got, tc.want) {
				t.Errorf("report = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

// A misspelled flow or module must be an ERROR, not "0 activities". The whole
// statement reports by counting, so a name that matches nothing would otherwise
// be indistinguishable from a filter that matches nothing.
func TestAlterFlowActivitiesRefusesANameThatResolvesToNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		stmt *ast.AlterFlowActivitiesStmt
		want string
	}{
		{"unknown microflow", &ast.AlterFlowActivitiesStmt{
			Flavour: ast.FlowMicroflow,
			Name:    ast.QualifiedName{Module: "Sales", Name: "Nope"},
			Disable: true, Filter: logDebugFilter()}, "Nope"},
		{"unknown module", &ast.AlterFlowActivitiesStmt{
			Flavour: ast.FlowMicroflow, Bulk: true, Module: "Nope",
			Disable: true, Filter: logDebugFilter()}, "Nope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, fx, _ := newFlowFixture(t, map[model.ID]backend.FlowActivityChange{})
			err := execAlterFlowActivities(ctx, tc.stmt)
			if err == nil {
				t.Fatal("a name matching nothing was accepted; it reports a clean success")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not name what was not found", err)
			}
			if len(fx.calls) != 0 {
				t.Errorf("wrote to %v after failing to resolve the name", fx.calls)
			}
		})
	}
}

// The filter is validated with the function `check` calls, so exec cannot
// accept what check refuses. Nothing is written when it fails.
func TestAlterFlowActivitiesRefusesABadFilterBeforeWriting(t *testing.T) {
	ctx, fx, _ := newFlowFixture(t, map[model.ID]backend.FlowActivityChange{"mf-a": {Changed: 1, Matched: 1}})
	err := execAlterFlowActivities(ctx, &ast.AlterFlowActivitiesStmt{
		Flavour: ast.FlowMicroflow, Bulk: true, Module: "Sales", Disable: true,
		Filter: types.ActivityFilter{Conditions: []types.ActivityCondition{
			{Column: types.ActivityColAction, Values: []string{"frobnicate"}},
		}},
	})
	if err == nil {
		t.Fatal("an unresolvable action word was accepted")
	}
	if len(fx.calls) != 0 {
		t.Errorf("wrote to %v despite a filter that selects nothing", fx.calls)
	}
	// Control: the same fixture with a sound filter DOES write, so the zero
	// above is about the filter and not about the fixture never writing.
	ctx2, fx2, _ := newFlowFixture(t, map[model.ID]backend.FlowActivityChange{"mf-a": {Changed: 1, Matched: 1}})
	if err := execAlterFlowActivities(ctx2, &ast.AlterFlowActivitiesStmt{
		Flavour: ast.FlowMicroflow, Bulk: true, Module: "Sales", Disable: true, Filter: logDebugFilter(),
	}); err != nil {
		t.Fatalf("the control statement failed: %v", err)
	}
	if len(fx2.calls) != 1 {
		t.Error("the control statement wrote nothing; the assertion above proves nothing")
	}
}

// An activity whose document has no Disabled key predates Mendix 9.12. The
// backend leaves it alone; the handler has to SAY so, or the user sees a
// statement that matched something and changed nothing with no reason given.
func TestAlterFlowActivitiesReportsPre912Documents(t *testing.T) {
	ctx, _, out := newFlowFixture(t, map[model.ID]backend.FlowActivityChange{
		"mf-a": {Matched: 2, Changed: 0, WithoutField: 2},
	})
	if err := execAlterFlowActivities(ctx, &ast.AlterFlowActivitiesStmt{
		Flavour: ast.FlowMicroflow,
		Name:    ast.QualifiedName{Module: "Sales", Name: "ACT_A"},
		Disable: true, Filter: logDebugFilter(),
	}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "9.12") {
		t.Errorf("report does not explain why nothing changed: %q", got)
	}
}
