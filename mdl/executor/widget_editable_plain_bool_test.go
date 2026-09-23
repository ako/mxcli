// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// ako/mxcli#510.
//
// MDL-WIDGET20 warned that a List View's `Editable` is "silently dropped on
// write and the widget stays enabled". Every clause after the "but" was wrong:
// mxcli writes it, `describe page` reads it back, and mxbuild accepts the
// result at 0 errors on 11.12.2.
//
// The rule's premise holds for the bug it was written for (#928, `editable:` on
// a button): Mendix models EDITABILITY — `Editability` /
// `ConditionalEditabilitySettings` — on input widgets only. Pages$ListView
// carries neither. What it has is a plain `Editable bool`, which is a DIFFERENT
// property with a different meaning: it makes the inputs INSIDE the list view
// editable. buildListViewV3 writes it, and its comment records what happens
// without it — every input renders as <div class="form-control-static">, "a
// value, not a field", with entity access ReadWrite and `mx check` at 0 errors.
//
// So the warning told authors to stop setting the one property that fixes a
// symptom its own code comment calls hard to diagnose. Its suggestion ("buttons
// do support conditional visibility") is addressed to a different widget, which
// is the tell.
func TestMDLWIDGET20_PlainEditableBoolIsWritten(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			name: "listview",
			src: `create page W.P ( Title: 'P' )
{
  LISTVIEW lv (DataSource: DATABASE W.Product, Editable: true) {
    DYNAMICTEXT dt ( Content: 'x' )
  }
}`,
		},
		{
			name: "grid column",
			src: `create page W.P ( Title: 'P' )
{
  LEGACYDATAGRID dg (DataSource: DATABASE W.Product) {
    COLUMN colName ( Attribute: Name, Caption: 'Name', Editable: true )
  }
}`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := widgetViolations(t, tc.src, "MDL-WIDGET20"); len(got) != 0 {
				t.Errorf("MDL-WIDGET20 claimed a written property is dropped: %#v", got)
			}
		})
	}
}

// The control that stops the fix from becoming "stop warning". A plain
// `Editable bool` is not conditional editability: Pages$ListView has no
// ConditionalEditabilitySettings, so the BRACKET form genuinely is dropped and
// must still be reported. Getting this wrong in the other direction would
// silently drop the shape the docs recommend — the worse half of #928.
func TestMDLWIDGET20_BracketFormStillReportedOnPlainEditableTypes(t *testing.T) {
	src := `create page W.P ( Title: 'P' )
{
  LISTVIEW lv (DataSource: DATABASE W.Product, Editable: [$currentObject/Name != '']) {
    DYNAMICTEXT dt ( Content: 'x' )
  }
}`
	got := widgetViolations(t, src, "MDL-WIDGET20")
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET20 violations for the bracket form on a list view, want 1", len(got))
	}
	if !strings.Contains(got[0].Message, "EditableIf") {
		t.Errorf("message should quote the property as spelled (EditableIf): %q", got[0].Message)
	}
}

// The #928 case must keep firing, or the rule has simply been deleted.
func TestMDLWIDGET20_ButtonStillReported(t *testing.T) {
	if got := widgetViolations(t, buttonPage(`editable: 'false'`), "MDL-WIDGET20"); len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET20 violations on a button, want 1 — the #928 case", len(got))
	}
}

// The sibling of TestEditableWidgetTypesMatchMetamodel, for the other property.
//
// That test keeps editableWidgetTypes in sync with the types carrying
// Editability. It cannot see this class at all, which is why the bug survived:
// a type with a plain `Editable bool` and no Editability is invisible to it.
// This reads generated/metamodel — the arbiter per CLAUDE.md — and fails when a
// new one appears, so it becomes a failing test rather than a false positive.
func TestPlainEditableBoolTypesMatchMetamodel(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "../../generated/metamodel/types.go", nil, 0)
	if err != nil {
		t.Fatalf("parse metamodel: %v", err)
	}

	plain := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasPrefix(ts.Name.Name, "Pages") {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		hasPlain, hasEditability := false, false
		for _, fld := range st.Fields.List {
			ident, isIdent := fld.Type.(*ast.Ident)
			for _, nm := range fld.Names {
				switch nm.Name {
				case "Editable":
					if isIdent && ident.Name == "bool" {
						hasPlain = true
					}
				case "Editability", "ConditionalEditabilitySettings":
					hasEditability = true
				}
			}
		}
		if hasPlain && !hasEditability {
			plain[ts.Name.Name] = true
		}
		return true
	})

	if len(plain) == 0 {
		t.Fatal("found no Pages type with a plain Editable bool — the parse is wrong, and a passing run would prove nothing")
	}

	src, err := parseRuleComments()
	if err != nil {
		t.Fatalf("read the rule's list: %v", err)
	}
	for typeName := range plain {
		mendixName := "Pages$" + strings.TrimPrefix(typeName, "Pages")
		if !strings.Contains(src, mendixName) {
			t.Errorf("%s has a plain Editable bool and no Editability, but is not named in "+
				"validate_widget_editability.go — mxcli would wrongly warn that its `editable:` is dropped",
				mendixName)
		}
	}
	if len(plain) != 2 {
		t.Errorf("the metamodel now has %d Pages types with a plain Editable bool, not the 2 measured "+
			"for #510 (%v) — re-check plainEditableWidgetTypes against it", len(plain), plain)
	}
}
