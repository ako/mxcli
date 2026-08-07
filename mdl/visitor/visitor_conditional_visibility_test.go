// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// Issue #627 — conditional-visibility/editability expressions must root bare
// attribute references in the widget data context ($currentObject), or Studio
// Pro rejects them with CE0117.
func TestConditionalVisibility_PrefixesContext(t *testing.T) {
	cases := []struct {
		name string
		expr string // what goes inside Visible: [ ... ]
		want string
	}{
		{"bare attr not-empty", "Name != ''", "$currentObject/Name != ''"},
		{"bare boolean attr", "Active", "$currentObject/Active"},
		{"bare attr empty keyword", "Name != empty", "$currentObject/Name != empty"},
		{"already qualified currentObject", "$currentObject/Name != ''", "$currentObject/Name != ''"},
		{"already qualified param", "$Customer/Name != ''", "$Customer/Name != ''"},
		{"and of two attrs", "Active and Name != ''", "$currentObject/Active and $currentObject/Name != ''"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := "CREATE PAGE M.P (Title: 'P') { CONTAINER ctn (Visible: [" + c.expr + "]) { DYNAMICTEXT t (Content: 'x') } };"
			prog, errs := Build(input)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			ctn := findWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, "ctn")
			if ctn == nil {
				t.Fatal("container ctn not found")
			}
			got, _ := ctn.Properties["VisibleIf"].(string)
			if got != c.want {
				t.Errorf("VisibleIf = %q, want %q", got, c.want)
			}
		})
	}
}

// Editable uses the same transform.
func TestConditionalEditability_PrefixesContext(t *testing.T) {
	input := "CREATE PAGE M.P (Title: 'P') { TEXTBOX tb (Label: 'N', Attribute: Name, Editable: [Status = 'Open']) };"
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	tb := findWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, "tb")
	if tb == nil {
		t.Fatal("textbox tb not found")
	}
	if got, _ := tb.Properties["EditableIf"].(string); got != "$currentObject/Status = 'Open'" {
		t.Errorf("EditableIf = %q, want %q", got, "$currentObject/Status = 'Open'")
	}
}

// findWidgetV3 finds a widget by name anywhere in the V3 widget tree.
func findWidgetV3(widgets []*ast.WidgetV3, name string) *ast.WidgetV3 {
	for _, w := range widgets {
		if w.Name == name {
			return w
		}
		if found := findWidgetV3(w.Children, name); found != nil {
			return found
		}
	}
	return nil
}

// Regression: a qualified enum value in a conditional-visibility expression must
// be preserved as the qualified literal (Module.Enum.Value), NOT stringified to
// 'Value' like an XPath datasource constraint — that produced CE0117 in v0.13.0.
func TestConditionalVisibility_EnumLiteralPreserved(t *testing.T) {
	input := "CREATE PAGE M.P (Title: 'P') { CONTAINER ctn (Visible: [$currentObject/Status = MES.EquipmentStatus.Running]) { DYNAMICTEXT t (Content: 'x') } };"
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	ctn := findWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, "ctn")
	got, _ := ctn.Properties["VisibleIf"].(string)
	want := "$currentObject/Status = MES.EquipmentStatus.Running"
	if got != want {
		t.Errorf("VisibleIf = %q, want %q", got, want)
	}
}

// Issue #852 — a widget conditional expression that calls a function whose name
// is also an MDL lexer keyword (trim, length, …) silently dropped the whole
// property: `xpathFunctionName` only admitted IDENTIFIER/HYPHENATED_ID plus a
// handful of keywords, so `trim(…)` never matched xpathFunctionCall and the
// expression built to nothing. The property then vanished from the page, and a
// dropped Visible defaults to "always visible" — a wrong-behaviour failure that
// passed both `mxcli check` and `mx check`.
//
// Non-keyword function names (toUpperCase, isMatch) were never affected; they
// are plain IDENTIFIERs. They are covered here so a future narrowing of the rule
// cannot regress them silently.
//
// Scope: this asserts the PARSER builds the call and the property survives, not
// that Mendix accepts the function. The two are deliberately separate — MDL does
// not adjudicate Mendix expression semantics at the grammar layer. `count` and
// `empty` are included because they are keyword tokens (the thing under test)
// even though mxbuild rejects them in a client expression with CE0117: `empty`
// is a literal (`$x != empty`), not a call. Verified against mxbuild 11.6.6,
// where trim/length/find pass and count/empty do not. The shipped example
// mdl-examples/bug-tests/852-conditional-keyword-functions.mdl uses only the
// valid set so it stays `mx check`-clean.
func TestConditionalVisibility_KeywordFunctionNames(t *testing.T) {
	cases := []struct {
		name string
		expr string // what goes inside Visible: [ ... ]
		want string
	}{
		{"trim", "trim($currentObject/Slug) != ''", "trim($currentObject/Slug) != ''"},
		{"length", "length($currentObject/Slug) > 0", "length($currentObject/Slug) > 0"},
		{"empty", "empty($currentObject/Slug)", "empty($currentObject/Slug)"},
		{"count", "count($currentObject/Items) > 0", "count($currentObject/Items) > 0"},
		{"find", "find($currentObject/Slug, 'x') >= 0", "find($currentObject/Slug, 'x') >= 0"},
		// Already worked — guard against regressing them.
		{"contains", "contains($currentObject/Slug, 'x')", "contains($currentObject/Slug, 'x')"},
		{"toUpperCase", "toUpperCase($currentObject/Slug) != ''", "toUpperCase($currentObject/Slug) != ''"},
		// A bare attribute inside a keyword-named call still gets rooted.
		{"trim roots bare attr", "trim(Slug) != ''", "trim($currentObject/Slug) != ''"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			input := "CREATE PAGE M.P (Title: 'P') { CONTAINER ctn (Visible: [" + c.expr + "]) { DYNAMICTEXT t (Content: 'x') } };"
			prog, errs := Build(input)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			ctn := findWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets, "ctn")
			if ctn == nil {
				t.Fatal("container ctn not found")
			}
			got, ok := ctn.Properties["VisibleIf"].(string)
			if !ok {
				t.Fatalf("VisibleIf missing entirely — the property was dropped (Properties: %v)", ctn.Properties)
			}
			if got != c.want {
				t.Errorf("VisibleIf = %q, want %q", got, c.want)
			}
		})
	}
}
