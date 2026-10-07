// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func tabPageNode(name, caption string) *ast.WidgetV3 {
	return &ast.WidgetV3{
		Type:       "tabpage",
		Name:       name,
		Properties: map[string]any{"Caption": caption},
		Children: []*ast.WidgetV3{{
			Type: "dynamictext", Name: "txt" + name, Properties: map[string]any{"Content": "two"},
		}},
	}
}

// mendixlabs/mxcli#1215: `insert after tpOne { tabpage tpTwo … }` and
// `insert into tabsMain { tabpage tpTwo … }` both failed with "failed to build
// widget tpTwo: tabpage must be a direct child of tabcontainer" — the widgets of
// an INSERT were built one by one, and a lone tab page is refused by the
// builder. Both forms must build the tab page and hand it to InsertTabPages
// with the position and target the statement named.
func TestAlterPageInsertTabPage_RoutesToTabPages(t *testing.T) {
	for _, tc := range []struct{ position, target string }{
		{"AFTER", "tpOne"},
		{"BEFORE", "tpOne"},
		{"INTO", "tabsMain"},
	} {
		t.Run(tc.position, func(t *testing.T) {
			var gotTarget string
			var gotPos backend.InsertPosition
			var gotPages []*pages.TabPage
			mutator := &mock.MockPageMutator{
				InsertTabPagesFunc: func(target string, pos backend.InsertPosition, tps []*pages.TabPage) error {
					gotTarget, gotPos, gotPages = target, pos, tps
					return nil
				},
				InsertWidgetFunc: func(string, string, backend.InsertPosition, []pages.Widget) error {
					t.Error("a tab page went through InsertWidget — it would land in a widget list")
					return nil
				},
			}
			err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
				Position: tc.position,
				Target:   ast.WidgetRef{Widget: tc.target},
				Widgets:  []*ast.WidgetV3{tabPageNode("tpTwo", "Two")},
			})
			if err != nil {
				t.Fatalf("insert %s %s { tabpage tpTwo }: %v", tc.position, tc.target, err)
			}
			if gotTarget != tc.target || !strings.EqualFold(string(gotPos), tc.position) {
				t.Errorf("InsertTabPages(%q, %q), want (%q, %q)", gotTarget, gotPos, tc.target, tc.position)
			}
			if len(gotPages) != 1 {
				t.Fatalf("got %d tab pages, want 1", len(gotPages))
			}
			tp := gotPages[0]
			if tp.Name != "tpTwo" || tp.TypeName != "Forms$TabPage" {
				t.Errorf("tab page = %s %q, want Forms$TabPage tpTwo", tp.TypeName, tp.Name)
			}
			if tp.Caption == nil || !captionHas(tp.Caption.Translations, "Two") {
				t.Errorf("caption = %+v, want Two", tp.Caption)
			}
			if len(tp.Widgets) != 1 || tp.Widgets[0].GetName() != "txttpTwo" {
				t.Errorf("tab page widgets = %d, want the dynamic text", len(tp.Widgets))
			}
		})
	}
}

func captionHas(tr map[string]string, want string) bool {
	for _, v := range tr {
		if v == want {
			return true
		}
	}
	return false
}

// A tab page and an ordinary widget go to different lists; one INSERT cannot
// mean both, so it is refused rather than half-applied.
func TestAlterPageInsertTabPage_MixedRefused(t *testing.T) {
	mutator := &mock.MockPageMutator{
		InsertTabPagesFunc: func(string, backend.InsertPosition, []*pages.TabPage) error {
			t.Error("InsertTabPages called for a mixed insert")
			return nil
		},
	}
	err := alterPageWith(t, mutator, &ast.InsertWidgetOp{
		Position: "AFTER",
		Target:   ast.WidgetRef{Widget: "tpOne"},
		Widgets: []*ast.WidgetV3{
			tabPageNode("tpTwo", "Two"),
			{Type: "dynamictext", Name: "plain", Properties: map[string]any{"Content": "x"}},
		},
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "mixing `tabpage`")
}
