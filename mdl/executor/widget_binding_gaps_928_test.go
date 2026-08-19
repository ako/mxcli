// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// upstream #928, bug 2. `editable:` was accepted on any widget and silently
// dropped, because the allow-lists behind MDL-WIDGET01/07 are widget-type
// AGNOSTIC — isBuiltinPropName answers "is this a real MDL property name
// anywhere", and both validators read it as "is this valid on THIS widget".
//
// A button that stays enabled is the dangerous shape: check passes, the build
// passes, and only the running app is wrong.
func TestMDLWIDGET20_EditableOnAWidgetWithoutEditability(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // the property key the message should quote
	}{
		{
			name: "plain form on a button (the reported case)",
			src:  buttonPage(`editable: 'false'`),
			want: "Editable",
		},
		{
			// The bracket form is the one that WORKS on inputs, so leaving it
			// unflagged on a button would silently drop the shape the docs
			// recommend — the worse half of the bug.
			name: "bracket form on a button",
			src:  buttonPage(`editable: [false]`),
			want: "EditableIf",
		},
		{
			name: "bracket form with a real expression",
			src:  buttonPage(`editable: [$currentObject/Name != '']`),
			want: "EditableIf",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := widgetViolations(t, tc.src, "MDL-WIDGET20")
			if len(got) != 1 {
				t.Fatalf("got %d MDL-WIDGET20 violations, want 1", len(got))
			}
			if got[0].Severity != linter.SeverityWarning {
				t.Errorf("severity = %v, want Warning (matching MDL-WIDGET07's family)", got[0].Severity)
			}
			if !strings.Contains(got[0].Message, tc.want) {
				t.Errorf("message should quote the property as spelled (%s), got %q", tc.want, got[0].Message)
			}
			if !strings.Contains(got[0].Suggestion, "visible") {
				t.Errorf("suggestion should point at conditional visibility, which buttons DO support: %q", got[0].Suggestion)
			}
		})
	}
}

// The control. An input widget genuinely has editability — flagging it would
// refuse the documented, working idiom, and 8 shipped examples use it.
func TestMDLWIDGET20_InputWidgetsAreClean(t *testing.T) {
	for _, w := range []string{
		`TEXTBOX tb ( Attribute: Name, editable: [false] )`,
		`TEXTAREA ta ( Attribute: Name, editable: [false] )`,
		`CHECKBOX cb ( Attribute: Name, editable: [false] )`,
		`DATEPICKER dp ( Attribute: Name, editable: [false] )`,
	} {
		t.Run(strings.Fields(w)[0], func(t *testing.T) {
			if got := widgetViolations(t, listviewPage(w), "MDL-WIDGET20"); len(got) != 0 {
				t.Errorf("MDL-WIDGET20 fired on an editable-capable widget: %#v", got)
			}
		})
	}
}

// A widget with no editability property at all must stay silent — the rule keys
// on the property being present, not on the widget type.
func TestMDLWIDGET20_SilentWithoutTheProperty(t *testing.T) {
	if got := widgetViolations(t, buttonPage(`Class: 'btn'`), "MDL-WIDGET20"); len(got) != 0 {
		t.Errorf("MDL-WIDGET20 fired on a button with no editable property: %#v", got)
	}
}

// editableWidgetTypes is a hand-maintained bridge between MDL type names and
// Mendix's, so it can drift from the metamodel it claims to mirror. This reads
// generated/metamodel — the arbiter per CLAUDE.md — and fails if the set of
// Pages types carrying editability changes, which is the event that would make
// the rule wrong in either direction.
func TestEditableWidgetTypesMatchMetamodel(t *testing.T) {
	fset := token.NewFileSet()
	path := filepath.Join("..", "..", "generated", "metamodel", "types.go")
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse metamodel: %v", err)
	}

	withEditability := map[string]bool{}
	ast.Inspect(f, func(n ast.Node) bool {
		ts, ok := n.(*ast.TypeSpec)
		if !ok || !strings.HasPrefix(ts.Name.Name, "Pages") {
			return true
		}
		st, ok := ts.Type.(*ast.StructType)
		if !ok {
			return true
		}
		for _, fld := range st.Fields.List {
			for _, nm := range fld.Names {
				if nm.Name == "Editability" || nm.Name == "ConditionalEditabilitySettings" {
					withEditability[ts.Name.Name] = true
				}
			}
		}
		return true
	})

	if len(withEditability) == 0 {
		t.Fatal("found no Pages type with editability — the parse is wrong, and a passing run would prove nothing")
	}

	// Every metamodel type must be reachable from the rule's list, via the
	// comment that names it. A new editable widget in Mendix therefore fails
	// here rather than silently becoming a false positive.
	src, err := parseRuleComments()
	if err != nil {
		t.Fatalf("read the rule's list: %v", err)
	}
	for typeName := range withEditability {
		mendixName := "Pages$" + strings.TrimPrefix(typeName, "Pages")
		if !strings.Contains(src, mendixName) {
			t.Errorf("%s carries editability in the metamodel but is not named in editableWidgetTypes — "+
				"a widget mxcli would now wrongly warn about", mendixName)
		}
	}
	if len(withEditability) != 11 {
		t.Errorf("the metamodel now has %d editable Pages types, not the 11 measured for #928 (%v) — "+
			"re-check editableWidgetTypes against it", len(withEditability), withEditability)
	}
}

func parseRuleComments() (string, error) {
	b, err := os.ReadFile("validate_widget_editability.go")
	return string(b), err
}

func buttonPage(prop string) string {
	return listviewPage(`ACTIONBUTTON btn ( Caption: 'Go', Action: NOTHING, ` + prop + ` )`)
}

func listviewPage(widget string) string {
	return `create page W.P ( Title: 'P' )
{
  LISTVIEW lv (DataSource: DATABASE W.Product) {
    ` + widget + `
  }
}`
}

// widgetViolations parses a page and returns the violations of one rule.
func widgetViolations(t *testing.T, src, ruleID string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	var out []linter.Violation
	for _, v := range ValidateWidgetProperties(prog, "") {
		if v.RuleID == ruleID {
			out = append(out, v)
		}
	}
	return out
}
