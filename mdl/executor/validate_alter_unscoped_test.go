// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/backend/pagemutator"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// storedScopesPage is a page with the three insertion contexts that matter:
//
//   - dvFlow: a data view sourced by a nanoflow the project does not contain —
//     the shape of Feedback v4.0.2's ShareFeedback_Logo, whose
//     FeedbackModule.DS_FeedbackForm is missing. Nothing puts an entity in scope.
//   - dvEntity: a data view with a database source, so MyModule.Customer is in
//     scope for its children.
//   - tbTop: a widget at the top level, outside any data container.
func storedScopesPage() bson.D {
	textbox := func(name string) bson.D {
		return bson.D{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "Name", Value: name}}
	}
	dvFlow := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvFlow"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$NanoflowSource"},
			{Key: "Nanoflow", Value: "MyModule.DS_Missing"},
		}},
		{Key: "Widgets", Value: bson.A{int32(2), textbox("tbFlow")}},
	}
	dvEntity := bson.D{
		{Key: "$Type", Value: "Forms$DataView"},
		{Key: "Name", Value: "dvEntity"},
		{Key: "DataSource", Value: bson.D{
			{Key: "$Type", Value: "Forms$DataViewSource"},
			{Key: "EntityRef", Value: bson.D{
				{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
				{Key: "Entity", Value: "MyModule.Customer"},
			}},
		}},
		{Key: "Widgets", Value: bson.A{int32(2), textbox("tbEntity")}},
	}
	return bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: bson.A{int32(2), textbox("tbTop"), dvFlow, dvEntity}}},
			}},
		}},
	}
}

func scopesPageCtx(t *testing.T) (*ExecContext, *countingDeps) {
	t.Helper()
	mod := mkModule("MyModule")
	pg := mkPage(mod.ID, "P_Scopes")
	deps := &countingDeps{}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListFoldersFunc: func() ([]*types.FolderInfo, error) { return nil, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{pg}, nil },
		OpenPageForMutationFunc: func(unitID model.ID) (backend.PageMutator, error) {
			return pagemutator.New(storedScopesPage(), unitID, deps), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return ctx, deps
}

func checkAlterUnscoped(t *testing.T, ctx *ExecContext, src string) []error {
	t.Helper()
	prog := parseMDL(t, src)
	sc := newScriptContext()
	sc.collectDefinitions(prog)
	return validateAlterUnscopedBindings(ctx, prog, sc)
}

// TestAlterUnscoped_MissingFlowSource is the reported gap: an image inserted
// next to a widget inside a data view whose nanoflow is missing, with its URL
// parameter bound bare. check --references passed; exec wrote a bare
// AttributeRef and `mx check` could not LOAD the project.
func TestAlterUnscoped_MissingFlowSource(t *testing.T) {
	ctx, deps := scopesPageCtx(t)
	errs := checkAlterUnscoped(t, ctx, `alter page MyModule.P_Scopes {
		insert after tbFlow { image zzImg (ImageType: imageUrl, ImageUrl: '{1}', ImageUrlParams: [{1} = ImageB64]) }
	}`)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	msg := errs[0].Error()
	for _, want := range []string{"MyModule.P_Scopes", "zzImg", "ImageB64", "MyModule.DS_Missing", "Module.Entity.Attribute"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %q", msg, want)
		}
	}
	if deps.saves != 0 {
		t.Errorf("validation wrote to storage %d times, want 0", deps.saves)
	}
}

// TestAlterUnscoped_ReplaceAndInto — REPLACE resolves the sibling context and
// INSERT INTO the target's own; both land in the flow-sourced data view.
func TestAlterUnscoped_ReplaceAndInto(t *testing.T) {
	ctx, _ := scopesPageCtx(t)
	for _, src := range []string{
		`alter page MyModule.P_Scopes { replace tbFlow with { textbox tbNew (Attribute: Subject) } }`,
		`alter page MyModule.P_Scopes { insert into dvFlow { textbox tbNew (Attribute: Subject) } }`,
	} {
		errs := checkAlterUnscoped(t, ctx, src)
		if len(errs) != 1 || !strings.Contains(errs[0].Error(), "Attribute: Subject") {
			t.Errorf("%s:\n  got %v, want one error naming Attribute: Subject", src, errs)
		}
	}
}

// TestAlterUnscoped_TopLevel — a widget inserted outside every data container
// has no entity either. Measured on 11.13.0: the image parameter was written
// bare (project unloadable) and a text box's attribute was silently dropped.
func TestAlterUnscoped_TopLevel(t *testing.T) {
	ctx, _ := scopesPageCtx(t)
	errs := checkAlterUnscoped(t, ctx, `alter page MyModule.P_Scopes {
		insert before tbTop { image zzTop (ImageType: imageUrl, ImageUrl: '{1}', ImageUrlParams: [{1} = FullName]) }
	}`)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "FullName") ||
		!strings.Contains(errs[0].Error(), "no data container") {
		t.Fatalf("got %v, want one error naming FullName and the missing container", errs)
	}
}

// ---------------------------------------------------------------------------
// Controls — a bare binding with an entity in scope is the ordinary case
// ---------------------------------------------------------------------------

func TestAlterUnscoped_Controls(t *testing.T) {
	cases := map[string]string{
		"entity in scope (sibling)": `alter page MyModule.P_Scopes {
			insert after tbEntity { textbox t (Attribute: Name) } }`,
		"entity in scope (into)": `alter page MyModule.P_Scopes {
			insert into dvEntity { textbox t (Attribute: Name) } }`,
		"qualified binding under the missing flow": `alter page MyModule.P_Scopes {
			insert after tbFlow { image zzImg (ImageType: imageUrl, ImageUrl: '{1}', ImageUrlParams: [{1} = MyModule.Feedback.ImageB64]) } }`,
		"nested container scopes its own children": `alter page MyModule.P_Scopes {
			insert before tbTop { dataview dvNew (DataSource: database MyModule.Customer) { textbox t (Attribute: Name) } } }`,
		"page parameter path": `alter page MyModule.P_Scopes {
			insert after tbFlow { dynamictext d (Content: '{1}', ContentParams: [{1} = $Customer/Name]) } }`,
		"unknown target is someone else's finding": `alter page MyModule.P_Scopes {
			insert after noSuchWidget { textbox t (Attribute: Name) } }`,
		// The flow is created by the script: its declared return type is what
		// exec will resolve against, so the bindings are in scope.
		"flow defined in the script": `create nanoflow MyModule.DS_Missing () returns MyModule.Customer begin
			return empty; end;
			alter page MyModule.P_Scopes { insert after tbFlow { textbox t (Attribute: Name) } }`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, _ := scopesPageCtx(t)
			if errs := checkAlterUnscoped(t, ctx, src); len(errs) != 0 {
				t.Errorf("got %v, want none", errs)
			}
		})
	}
}
