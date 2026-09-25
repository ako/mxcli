// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// PROPOSAL_first_class_expressions.md slice 2, named properties: DynamicClasses
// (any widget) and a datagrid column's DynamicCellClass hold ONE Mendix
// expression, and MDL writes it as-is. A quoted value is a Mendix string — the
// same rule as the OData client's credentials (#676) — so the doubled-quote
// spelling `'if … then ''a'' else '''''` is no longer needed, and is refused.

func pageWidgets(t *testing.T, src string) map[string]*ast.WidgetV3 {
	t.Helper()
	out := map[string]*ast.WidgetV3{}
	var walk func(ws []*ast.WidgetV3)
	walk = func(ws []*ast.WidgetV3) {
		for _, w := range ws {
			out[w.Name] = w
			walk(w.Children)
		}
	}
	for _, s := range parseMDL(t, src).Statements {
		if p, ok := s.(*ast.CreatePageStmtV3); ok {
			walk(p.Widgets)
		}
	}
	return out
}

func TestWidgetExpressionProps_StoreTheExpressionAsWritten(t *testing.T) {
	ws := pageWidgets(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (dynamicclasses: if $currentObject/Featured then 'is-featured' else '') { }
  container c2 (dynamicclasses: 'is-featured') { }
  container c3 (DynamicClasses: $currentObject/Style + ' card') { }
  datagrid dg (datasource: database M.Thing) {
    column col1 (attribute: Name, caption: 'N', DynamicCellClass: if $currentObject/Price > 100 then 'highlight' else '')
  }
}`)
	for name, want := range map[string]string{
		"c1":   "if $currentObject/Featured then 'is-featured' else ''",
		"c2":   "'is-featured'",
		"c3":   "$currentObject/Style + ' card'",
		"col1": "if $currentObject/Price > 100 then 'highlight' else ''",
	} {
		w := ws[name]
		if w == nil {
			t.Fatalf("widget %s not parsed", name)
		}
		got := w.GetDynamicClasses()
		if name == "col1" {
			got = w.GetStringProp("DynamicCellClass")
		}
		if got != want {
			t.Errorf("%s stored %q, want the expression %q", name, got, want)
		}
	}
}

func TestWidgetExpressionProps_AlterStoresTheExpression(t *testing.T) {
	prog := parseMDL(t, `alter page M.P { set DynamicClasses = if $currentObject/Featured then 'a' else 'b' on c1 };`)
	stmt := prog.Statements[0].(*ast.AlterPageStmt)
	var got any
	for _, op := range stmt.Operations {
		if set, ok := op.(*ast.SetPropertyOp); ok {
			got = set.Properties["DynamicClasses"]
		}
	}
	if got != "if $currentObject/Featured then 'a' else 'b'" {
		t.Errorf("set DynamicClasses stored %#v", got)
	}
}

// Widening the value rule for two properties must not open a silent empty value
// for every other one.
func TestWidgetExpressionInAPlainProperty_IsAnError(t *testing.T) {
	for _, src := range []string{
		`create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) { combobox cb (emptyOptionText: 'a' + 'b') }`,
		`alter page M.P { set Caption = 'Save' + ' now' on btn1 };`,
	} {
		_, errs := visitor.Build(src)
		if len(errs) == 0 {
			t.Errorf("accepted an expression in a plain-value property: %s", src)
			continue
		}
		if !strings.Contains(errs[0].Error(), "expression") {
			t.Errorf("error %q should say the property does not take an expression", errs[0])
		}
	}
}

// MDL-WIDGET33: the old spelling — a quoted string holding the expression's
// text — now stores a string, so it is refused and the message names the
// unquoted expression.
func TestMDLWIDGET33_LegacyQuotedExpression(t *testing.T) {
	cases := []struct {
		name, value string
		want        int
	}{
		{"legacy if-expression", `'if $currentObject/Featured then ''is-featured'' else '''''`, 1},
		{"legacy attribute concatenation", `'$currentObject/Style + '' card'''`, 1},
		{"control: a class-name string", `'is-featured'`, 0},
		{"control: a class list", `'btn btn-lg'`, 0},
		{"control: the unquoted expression", `if $currentObject/Featured then 'is-featured' else ''`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (dynamicclasses: ` + tc.value + `) { }
  datagrid dg (datasource: database M.Thing) {
    column col1 (attribute: Name, caption: 'N', DynamicCellClass: ` + tc.value + `)
  }
}`
			got := widgetViolations(t, src, "MDL-WIDGET33")
			if len(got) != 2*tc.want {
				t.Fatalf("MDL-WIDGET33: got %d, want %d: %#v", len(got), 2*tc.want, got)
			}
			if tc.want > 0 && !strings.Contains(got[0].Suggestion, "if $currentObject") &&
				!strings.Contains(got[0].Suggestion, "$currentObject/Style") {
				t.Errorf("suggestion should give the unquoted expression: %s", got[0].Suggestion)
			}
		})
	}
}

func TestMDLWIDGET33_LegacyQuotedExpressionOnAlter(t *testing.T) {
	prog := parseMDL(t, `alter page M.P { set DynamicClasses = 'if $currentObject/F then ''a'' else ''b''' on c1 };`)
	var got []string
	for _, v := range ValidateWidgetProperties(prog, "") {
		if v.RuleID == "MDL-WIDGET33" {
			got = append(got, v.Message)
		}
	}
	if len(got) != 1 {
		t.Fatalf("MDL-WIDGET33 on ALTER: got %v, want one", got)
	}
}

// describe prints the stored expression as-is, and that output re-stores it.
func TestDescribeWidgetExpressionProps_RoundTrip(t *testing.T) {
	const expr = "if $currentObject/Featured then 'is-featured' else ''"
	props := appendAppearanceProps(nil, rawWidget{DynamicClasses: expr})
	var line string
	for _, p := range props {
		if strings.HasPrefix(p, "DynamicClasses:") {
			line = p
		}
	}
	if line != "DynamicClasses: "+expr {
		t.Fatalf("describe printed %q, want the expression unquoted", line)
	}
	ws := pageWidgets(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  container c1 (`+line+`) { }
}`)
	if got := ws["c1"].GetDynamicClasses(); got != expr {
		t.Errorf("re-exec of describe output stores %q, want %q", got, expr)
	}

	var out bytes.Buffer
	outputDataGrid2ColumnV3(&ExecContext{Output: &out}, "", "col1", rawDataGridColumn{DynamicCellClass: expr})
	if !strings.Contains(out.String(), "DynamicCellClass: "+expr) {
		t.Errorf("column describe should print the expression unquoted, got:\n%s", out.String())
	}
}
