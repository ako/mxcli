// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func testIconIndex() *iconIndex {
	return &iconIndex{
		collections: map[string]map[string]bool{
			"Atlas_Core.Atlas":        {"home": true, "pencil": true, "pencil-write-paper": true},
			"Atlas_Core.Atlas_Filled": {"home": true, "pencil": true},
		},
		order: []string{"Atlas_Core.Atlas", "Atlas_Core.Atlas_Filled"},
	}
}

// TestIconIndexCheck covers the resolution rules. An icon reference was written
// straight through to BSON with nothing resolving it, so a typo first surfaced
// as CE1613 from MxBuild — long after `mxcli check` had passed.
func TestIconIndexCheck(t *testing.T) {
	idx := testIconIndex()
	tests := []struct {
		name      string
		value     string
		wantErr   bool
		wantParts []string
	}{
		{
			name:  "valid reference",
			value: "Atlas_Core.Atlas_Filled.pencil",
		},
		{
			name:  "valid reference in the other collection",
			value: "Atlas_Core.Atlas.home",
		},
		{
			name:      "unknown icon in a known collection",
			value:     "Atlas_Core.Atlas_Filled.no-such-icon",
			wantErr:   true,
			wantParts: []string{"no-such-icon", "Atlas_Core.Atlas_Filled", "CE1613", "describe icon collection"},
		},
		{
			name:    "unknown collection names the ones that exist",
			value:   "Atlas_Core.Nope.pencil",
			wantErr: true,
			// Listing the real collections is the actionable half: the typo is
			// usually in the collection, not the icon.
			wantParts: []string{"unknown icon collection", "Atlas_Core.Nope", "Atlas_Core.Atlas_Filled"},
		},
		{
			name:      "not a qualified reference",
			value:     "pencil",
			wantErr:   true,
			wantParts: []string{"not a qualified icon reference", "Module.Collection.IconName"},
		},
		{
			name:      "trailing dot",
			value:     "Atlas_Core.Atlas.",
			wantErr:   true,
			wantParts: []string{"not a qualified icon reference"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := idx.check(iconRef{value: tt.value, where: "actionbutton 'btn'"})
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("unexpected error for %q: %v", tt.value, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected an error for %q", tt.value)
			}
			for _, want := range tt.wantParts {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not mention %q", err.Error(), want)
				}
			}
		})
	}
}

// TestNearestIcons pins the suggestion behaviour: a near miss gets a hint, a
// name with nothing in common does not get a misleading one.
func TestNearestIcons(t *testing.T) {
	icons := map[string]bool{"pencil": true, "pencil-write-paper": true, "home": true}

	got := nearestIcons(icons, "penci")
	if len(got) != 2 || got[0] != "pencil" || got[1] != "pencil-write-paper" {
		t.Errorf("nearestIcons(penci) = %v, want [pencil pencil-write-paper]", got)
	}
	if got := nearestIcons(icons, "zzzzz"); len(got) != 0 {
		t.Errorf("nearestIcons(zzzzz) = %v, want none — a wrong hint is worse than no hint", got)
	}
	// An exact match is not a suggestion for itself.
	if got := nearestIcons(icons, "home"); len(got) != 0 {
		t.Errorf("nearestIcons(home) = %v, want none", got)
	}
}

// TestIconRefsInStatement covers collection from every statement shape that can
// carry an icon — a reference the walker misses is a reference nothing checks.
func TestIconRefsInStatement(t *testing.T) {
	btn := func(name, icon string) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "ACTIONBUTTON", Name: name, Properties: map[string]any{"Icon": icon}}
	}

	tests := []struct {
		name string
		stmt ast.Statement
		want []string
	}{
		{
			name: "create page, nested widgets",
			stmt: &ast.CreatePageStmtV3{Widgets: []*ast.WidgetV3{
				{Type: "CONTAINER", Name: "c1", Children: []*ast.WidgetV3{
					btn("btnA", "Atlas_Core.Atlas.home"),
					btn("btnB", "Atlas_Core.Atlas.pencil"),
				}},
			}},
			want: []string{"Atlas_Core.Atlas.home", "Atlas_Core.Atlas.pencil"},
		},
		{
			name: "alter page set",
			stmt: &ast.AlterPageStmt{Operations: []ast.AlterPageOperation{
				&ast.SetPropertyOp{
					Target:     ast.WidgetRef{Widget: "btnSave"},
					Properties: map[string]any{"Icon": "Atlas_Core.Atlas.home"},
				},
			}},
			want: []string{"Atlas_Core.Atlas.home"},
		},
		{
			name: "alter page insert and replace",
			stmt: &ast.AlterPageStmt{Operations: []ast.AlterPageOperation{
				&ast.InsertWidgetOp{Widgets: []*ast.WidgetV3{btn("btnI", "Atlas_Core.Atlas.a")}},
				&ast.ReplaceWidgetOp{NewWidgets: []*ast.WidgetV3{btn("btnR", "Atlas_Core.Atlas.b")}},
			}},
			want: []string{"Atlas_Core.Atlas.a", "Atlas_Core.Atlas.b"},
		},
		{
			name: "navigation menu, including sub-items",
			stmt: &ast.AlterNavigationStmt{MenuItems: []ast.NavMenuItemDef{
				{Caption: "Home", Icon: "Atlas_Core.Atlas.home", Items: []ast.NavMenuItemDef{
					{Caption: "Nested", Icon: "Atlas_Core.Atlas.pencil"},
				}},
			}},
			want: []string{"Atlas_Core.Atlas.home", "Atlas_Core.Atlas.pencil"},
		},
		{
			name: "no icons at all",
			stmt: &ast.CreatePageStmtV3{Widgets: []*ast.WidgetV3{
				{Type: "CONTAINER", Name: "c1"},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs := iconRefsInStatement(tt.stmt)
			if len(refs) != len(tt.want) {
				t.Fatalf("got %d refs %v, want %d %v", len(refs), refs, len(tt.want), tt.want)
			}
			for i, w := range tt.want {
				if refs[i].value != w {
					t.Errorf("ref[%d] = %q, want %q", i, refs[i].value, w)
				}
				if refs[i].where == "" {
					t.Errorf("ref[%d] has no location for the message", i)
				}
			}
		})
	}
}

// TestIconPropValue covers the quoting and casing MDL allows — `Icon:` and
// `icon:` are the same property, and the value may arrive quoted.
func TestIconPropValue(t *testing.T) {
	tests := []struct {
		props map[string]any
		want  string
	}{
		{map[string]any{"Icon": "Atlas_Core.Atlas.home"}, "Atlas_Core.Atlas.home"},
		{map[string]any{"icon": "Atlas_Core.Atlas.home"}, "Atlas_Core.Atlas.home"},
		{map[string]any{"ICON": "'Atlas_Core.Atlas.home'"}, "Atlas_Core.Atlas.home"},
		{map[string]any{"Icon": "  Atlas_Core.Atlas.home  "}, "Atlas_Core.Atlas.home"},
		{map[string]any{"Caption": "not an icon"}, ""},
		{map[string]any{"Icon": 42}, ""}, // non-string must not panic
		{nil, ""},
	}
	for _, tt := range tests {
		if got := iconPropValue(tt.props); got != tt.want {
			t.Errorf("iconPropValue(%v) = %q, want %q", tt.props, got, tt.want)
		}
	}
}
