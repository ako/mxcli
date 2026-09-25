// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// An EXCLUDED page may reference documents that do not exist. Mendix does not
// validate excluded documents: Feedback v4.0.2 ships
// FeedbackModule.ShareFeedback_Logo as an excluded example page bound to five
// nanoflows the module does not contain, and the untouched project passes
// `mx check` with 0 errors. describe → exec of that page was refused:
//
//	Reference error: statement 1: page 'FeedbackModule.ShareFeedback_Logo' has reference errors:
//	  - nanoflow not found: FeedbackModule.DS_FeedbackForm
//	  - nanoflow not found: FeedbackModule.ACT_TriggerScreenshotMode
//	  ...
//
// and with --no-check the page builder refused the same names a second time
// ("failed to resolve nanoflow"). The references are still reported — as
// warnings — so nothing is hidden.
//
// Except a DATA SOURCE: its flow is what puts an entity in scope, and writing a
// container without one left unqualified attribute bindings that made mx
// unable to LOAD the project (measured, 11.13.0). That one still blocks.

func excludedPageStmt(excluded bool) *ast.CreatePageStmtV3 {
	return &ast.CreatePageStmtV3{
		Name:     ast.QualifiedName{Module: "Feedback", Name: "ShareFeedback_Logo"},
		IsModify: true,
		Excluded: excluded,
		Widgets:  actionWidget("nanoflow", "Feedback.ACT_ClearForm"),
	}
}

func TestValidateExcludedPage_DanglingRefsAreWarnings(t *testing.T) {
	ctx, _ := newMockCtx(t)
	sc := newScriptContext()
	sc.modules["Feedback"] = true

	if err := validateWithContext(ctx, excludedPageStmt(true), sc); err != nil {
		t.Fatalf("an excluded page's dangling reference must not block; got:\n%v", err)
	}
	if len(sc.warnings) != 1 || !strings.Contains(sc.warnings[0], "nanoflow not found: Feedback.ACT_ClearForm") ||
		!strings.Contains(sc.warnings[0], "excluded") {
		t.Errorf("the dangling reference must still be reported as a warning; got %q", sc.warnings)
	}
}

// CONTROL: the same page, not excluded, is still refused.
func TestValidateLivePage_DanglingRefsAreErrors(t *testing.T) {
	ctx, _ := newMockCtx(t)
	sc := newScriptContext()
	sc.modules["Feedback"] = true

	err := validateWithContext(ctx, excludedPageStmt(false), sc)
	if err == nil || !strings.Contains(err.Error(), "has reference errors") ||
		!strings.Contains(err.Error(), "nanoflow not found: Feedback.ACT_ClearForm") {
		t.Fatalf("a live page's dangling reference must be refused; got %v", err)
	}
	if len(sc.warnings) != 0 {
		t.Errorf("no warnings expected for a refused statement; got %q", sc.warnings)
	}
}

// exec keeps a page excluded when every stored page of that name is excluded,
// even if the statement does not say @excluded (#914). The check has to agree
// with what exec will write, not only with what the statement says.
func TestValidateCarriedExclusion_DanglingRefsAreWarnings(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: "mod1"}, Name: "Feedback"}
	stored := &pages.Page{BaseElement: model.BaseElement{ID: "pg1"}, ContainerID: "mod1",
		Name: "ShareFeedback_Logo", Excluded: true}
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{stored}, nil },
	}))
	sc := newScriptContext()

	if err := validateWithContext(ctx, excludedPageStmt(false), sc); err != nil {
		t.Fatalf("a page exec keeps excluded must not be refused; got:\n%v", err)
	}
	if len(sc.warnings) != 1 {
		t.Errorf("want one warning, got %q", sc.warnings)
	}

	// CONTROL: a plain CREATE does not carry anything, and a live twin wins.
	stored.Excluded = false
	if err := validateWithContext(ctx, excludedPageStmt(false), newScriptContext()); err == nil {
		t.Error("with a live page of the same name the rewrite is live, and must be refused")
	}
}

// The builder is the second refusal: resolveNanoflowByName fails on the
// missing name. For an excluded page the name is kept (the writer stores it
// BY_NAME; the ID is never serialized) and the build continues.
func TestBuildExcludedPage_DanglingFlowIsKeptByName(t *testing.T) {
	ctx, _ := newMockCtx(t, withBackend(&mock.MockBackend{
		IsConnectedFunc:   func() bool { return true },
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) { return nil, nil },
	}))
	newPB := func(tolerate bool) *pageBuilder {
		pb := newPopupPageBuilder()
		pb.ctx, pb.backend = ctx, ctx.Backend
		pb.tolerateDanglingRefs = tolerate
		return pb
	}
	cases := []*ast.ActionV3{
		{Type: "nanoflow", Target: "Feedback.ACT_ClearForm"},
		{Type: "microflow", Target: "Feedback.ACT_Missing"},
		{Type: "showPage", Target: "Feedback.Missing_Page"},
	}
	for _, act := range cases {
		t.Run(act.Type, func(t *testing.T) {
			got, err := newPB(true).buildClientActionV3(act)
			if err != nil {
				t.Fatalf("excluded page: %v", err)
			}
			var name string
			switch a := got.(type) {
			case *pages.NanoflowClientAction:
				name = a.NanoflowName
			case *pages.MicroflowClientAction:
				name = a.MicroflowName
			case *pages.PageClientAction:
				name = a.PageName
			}
			if name != act.Target {
				t.Errorf("reference must be kept by name; got %q", name)
			}
			// CONTROL: a live page still refuses.
			if _, err := newPB(false).buildClientActionV3(act); err == nil {
				t.Error("a live page must still refuse a dangling reference")
			}
		})
	}

	// A data source is never tolerated, excluded or not.
	ds := &ast.DataSourceV3{Type: "nanoflow", Reference: "Feedback.DS_FeedbackForm"}
	if _, _, err := newPB(true).buildDataSourceV3(ds); err == nil {
		t.Error("a dangling data source must be refused even on an excluded page")
	}
}

// A dangling data source on an excluded page still blocks, with the reason;
// the action targets beside it are still only warnings.
func TestValidateExcludedPage_DanglingDataSourceBlocks(t *testing.T) {
	ctx, _ := newMockCtx(t)
	sc := newScriptContext()
	sc.modules["Feedback"] = true
	s := excludedPageStmt(true)
	s.Widgets = []*ast.WidgetV3{{
		Name: "dv", Type: "dataview",
		Properties: map[string]any{
			"DataSource": &ast.DataSourceV3{Type: "nanoflow", Reference: "Feedback.DS_FeedbackForm"},
		},
		Children: actionWidget("nanoflow", "Feedback.ACT_ClearForm"),
	}}

	err := validateWithContext(ctx, s, sc)
	if err == nil || !strings.Contains(err.Error(), "nanoflow not found: Feedback.DS_FeedbackForm (data source)") ||
		!strings.Contains(err.Error(), "is excluded, but a data source") {
		t.Fatalf("want the data source refused with its reason; got %v", err)
	}
	if strings.Contains(err.Error(), "ACT_ClearForm") {
		t.Errorf("the action target must not be in the refusal: %v", err)
	}
	if len(sc.warnings) != 1 || !strings.Contains(sc.warnings[0], "nanoflow not found: Feedback.ACT_ClearForm") {
		t.Errorf("the action target must still be a warning; got %q", sc.warnings)
	}
}
