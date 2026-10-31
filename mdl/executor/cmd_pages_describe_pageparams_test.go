// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// pageParamFixture builds a module with a target page that declares one
// parameter — the drill-down page a grid's link button opens.
func pageParamFixture(t *testing.T, params ...string) *ExecContext {
	t.Helper()
	return pageParamFixtureWith(t, nil, params...)
}

// pageParamFixtureWith is pageParamFixture with the named parameters optional
// (IsRequired false, Mendix 11.5+); every other one is required.
func pageParamFixtureWith(t *testing.T, optional map[string]bool, params ...string) *ExecContext {
	t.Helper()
	mod := mkModule("Formula1Frontend")

	target := &pages.Page{
		BaseElement: model.BaseElement{ID: "pg-weekend"},
		ContainerID: mod.ID,
		Name:        "Race_Weekend",
	}
	for _, p := range params {
		target.Parameters = append(target.Parameters, &pages.PageParameter{
			BaseElement: model.BaseElement{ID: model.ID("pp-" + p)},
			ContainerID: target.ID,
			Name:        p,
			IsRequired:  !optional[p],
		})
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListPagesFunc:   func() ([]*pages.Page, error) { return []*pages.Page{target}, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

// showPageAction is the BSON mxcli writes for SHOW_PAGE: ParameterMappings is a
// deliberately empty array, because Studio Pro infers the row object from the
// enclosing widget and rejects an explicit "$currentObject" Argument as CE0115.
func showPageAction(page string, mappings []any) map[string]any {
	settings := map[string]any{
		"$Type": "Forms$FormSettings",
		"Form":  page,
	}
	if mappings != nil {
		settings["ParameterMappings"] = mappings
	}
	return map[string]any{
		"$Type":        "Forms$FormAction",
		"FormSettings": settings,
	}
}

// mxcli-formula1 §39: `SHOW_PAGE P(Race = $currentObject)` described back as
// `show_page P`. The mapping was never in the model — mxcli stores it implicitly
// on purpose — but DESCRIBE had no compensating recovery, so its output read as
// a diagnosis ("the mapping was dropped, that is why the page gets an empty
// object") in the middle of a hunt for an unrelated bug. Three cycles were spent
// replacing a button that was correct.
func TestRenderShowPageAction_RecoversTheImplicitParameter(t *testing.T) {
	ctx := pageParamFixture(t, "Race")

	got := renderClientActionMDL(ctx, showPageAction("Formula1Frontend.Race_Weekend", nil))
	want := "show page Formula1Frontend.Race_Weekend(Race = $currentObject)"
	if got != want {
		t.Errorf("DESCRIBE lost the page parameter:\n got: %s\nwant: %s", got, want)
	}
}

// A page with several parameters gets all of them, in declaration order.
func TestRenderShowPageAction_RecoversEveryParameter(t *testing.T) {
	ctx := pageParamFixture(t, "Race", "Season")

	got := renderClientActionMDL(ctx, showPageAction("Formula1Frontend.Race_Weekend", nil))
	want := "show page Formula1Frontend.Race_Weekend(Race = $currentObject, Season = $currentObject)"
	if got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

// An optional parameter (Mendix 11.5+) with no stored mapping is not passed,
// so describe recovers no argument for it — inventing one made a menu item's
// description refused by exec (MDL-PAGEARG01, mendixlabs/mxcli#1640).
func TestRenderShowPageAction_SkipsAnOptionalParameter(t *testing.T) {
	ctx := pageParamFixtureWith(t, map[string]bool{"Season": true}, "Race", "Season")

	got := renderClientActionMDL(ctx, showPageAction("Formula1Frontend.Race_Weekend", nil))
	want := "show page Formula1Frontend.Race_Weekend(Race = $currentObject)"
	if got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
	ctx = pageParamFixtureWith(t, map[string]bool{"Race": true}, "Race")
	if got := renderClientActionMDL(ctx, showPageAction("Formula1Frontend.Race_Weekend", nil)); got != "show page Formula1Frontend.Race_Weekend" {
		t.Errorf("a page whose only parameter is optional grew an argument: %s", got)
	}
}

// A page that takes no parameters must not grow an argument list.
func TestRenderShowPageAction_NoParametersStaysBare(t *testing.T) {
	ctx := pageParamFixture(t)

	got := renderClientActionMDL(ctx, showPageAction("Formula1Frontend.Race_Weekend", nil))
	if want := "show page Formula1Frontend.Race_Weekend"; got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

// An unresolvable page yields no arguments rather than invented ones: a
// description that omits an argument is recoverable, one that names a parameter
// that does not exist is not.
func TestRenderShowPageAction_UnknownPageInventsNothing(t *testing.T) {
	ctx := pageParamFixture(t, "Race")

	got := renderClientActionMDL(ctx, showPageAction("OtherModule.Gone", nil))
	if want := "show page OtherModule.Gone"; got != want {
		t.Errorf("got:  %s\nwant: %s", got, want)
	}
}

// An explicit mapping in the model still wins — recovery fills a gap, it does
// not override what Studio Pro actually stored.
func TestRenderShowPageAction_ExplicitMappingWins(t *testing.T) {
	ctx := pageParamFixture(t, "Race")

	mappings := []any{
		int32(3),
		map[string]any{
			"$Type":     "Forms$PageParameterMapping",
			"Parameter": "Formula1Frontend.Race_Weekend.Race",
			"Argument":  "$SelectedRace",
		},
	}
	got := renderClientActionMDL(ctx, showPageAction("Formula1Frontend.Race_Weekend", mappings))
	want := "show page Formula1Frontend.Race_Weekend(Race = $SelectedRace)"
	if got != want {
		t.Errorf("an explicit mapping was overwritten:\n got: %s\nwant: %s", got, want)
	}
}
