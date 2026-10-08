// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"
)

// A DataGrid 2 column's Visible is an expression property, read by the column
// builder (widgetobj.BuildDataGrid2Column) from the key "Visible" as a string.
// The visitor routes every expression spelling — `Visible: [cond]`, `Visible:
// if … then … else …`, `Visible: $currentObject/Flag` — to "VisibleIf", and
// `Visible: false` arrives as a bool, so each one reached the builder as "no
// value" and the column was written always visible: check clean, exec
// reporting "Created page". columnSpecProperties is where the column spec's
// properties are put in the builder's terms.
func TestDataGridColumnVisible_EverySpellingReachesTheBuilder(t *testing.T) {
	cases := []struct {
		name, prop string
		want       any // nil: the builder's default (always visible)
	}{
		{"bracket condition", `Visible: [Price > 100]`, "$currentObject/Price > 100"},
		{"if expression", `Visible: if $currentObject/Price > 100 then true else false`, "if $currentObject/Price > 100 then true else false"},
		{"attribute path", `Visible: $currentObject/Featured`, "$currentObject/Featured"},
		{"lowercase key", `visible: $currentObject/Featured`, "$currentObject/Featured"},
		{"static false", `Visible: false`, "false"},
		{"control: quoted expression (mdl 0)", `Visible: 'if $currentObject/Price > 100 then true else false'`, "if $currentObject/Price > 100 then true else false"},
		{"control: static true", `Visible: true`, nil},
		{"control: absent", `Sortable: true`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := pageWidgets(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  datagrid dg (datasource: database M.Thing) {
    column (attribute: Name, caption: 'N', `+tc.prop+`)
  }
}`)
			props := columnSpecProperties(ws["dg"].Children[0])
			got, present := props["Visible"]
			if tc.want == nil {
				if present {
					t.Fatalf("Visible = %#v, want it absent (always visible)", got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("Visible = %#v, want %q — the column would be written always visible", got, tc.want)
			}
		})
	}
}

// The builder reads DynamicCellClass by its canonical key; the visitor keeps a
// named expression property under the key as written.
func TestDataGridColumnDynamicCellClass_AnyCase(t *testing.T) {
	ws := pageWidgets(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  datagrid dg (datasource: database M.Thing) {
    column (attribute: Name, caption: 'N', dynamiccellclass: if $currentObject/Price > 100 then 'hi' else '')
  }
}`)
	props := columnSpecProperties(ws["dg"].Children[0])
	if got := props["DynamicCellClass"]; got != "if $currentObject/Price > 100 then 'hi' else ''" {
		t.Fatalf("DynamicCellClass = %#v, want the expression", got)
	}
}

// describe prints a column's Visible in the form a page widget's is printed,
// and re-executing that output stores the same expression.
func TestDescribeDataGridColumnVisible_RoundTrip(t *testing.T) {
	for _, expr := range []string{
		"$currentObject/Price > 100",
		"if $currentObject/Price > 100 then true else false",
		"$currentObject/Featured",
		"false",
	} {
		var out bytes.Buffer
		outputDataGrid2ColumnV3(&ExecContext{Output: &out}, "", rawDataGridColumn{Attribute: "Name", Caption: "N", Visible: expr})
		line := strings.TrimSpace(out.String())
		if strings.Contains(line, "Visible: '") {
			t.Errorf("describe quoted the expression: %s", line)
		}
		ws := pageWidgets(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  datagrid dg (datasource: database M.Thing) {
    `+line+`
  }
}`)
		if got := columnSpecProperties(ws["dg"].Children[0])["Visible"]; got != expr {
			t.Errorf("re-exec of %q stores Visible %#v, want %q", line, got, expr)
		}
	}
}

// MDL-WIDGET43: measured on Mendix 11.14.0, every column whose Visible names
// $currentObject is CE0117 in mx check, and `Visible: false` is clean.
func TestMDLWIDGET43_ColumnVisibleHasNoRowObject(t *testing.T) {
	cases := []struct {
		prop string
		want int
	}{
		{`Visible: [Price > 100]`, 1},
		{`Visible: $currentObject/Featured`, 1},
		{`Visible: if $currentObject/Price > 100 then true else false`, 1},
		{`Visible: 'if $currentObject/Price > 100 then true else false'`, 1},
		{`Visible: false`, 0},
		{`Visible: $ShowPrices`, 0},
		{`DynamicCellClass: if $currentObject/Featured then 'hi' else ''`, 0},
	}
	for _, tc := range cases {
		src := `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  datagrid dg (datasource: database M.Thing) {
    column (attribute: Name, caption: 'N', ` + tc.prop + `)
  }
};`
		if got := widgetViolations(t, src, "MDL-WIDGET43"); len(got) != tc.want {
			t.Errorf("%s: MDL-WIDGET43 = %d, want %d: %v", tc.prop, len(got), tc.want, got)
		}
	}
	// A page widget's conditional visibility has the widget's row object.
	if got := widgetViolations(t, `create page M.P (title: 'P', layout: Atlas_Core.Atlas_Default) {
  dataview dv (datasource: $Thing) { textbox t (attribute: Name, Visible: $currentObject/Featured) }
};`, "MDL-WIDGET43"); len(got) != 0 {
		t.Errorf("control: a textbox's Visible was refused: %v", got)
	}
}

func TestMDLWIDGET43_AlterSetAndInsert(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`alter page M.P { set (Visible: $currentObject/Featured) on dg column('N') };`, 1},
		{`alter page M.P { set (Visible: [Price > 1]) on dg column(Name) };`, 1},
		{`alter page M.P { insert after dg column('N') { column (attribute: Name, caption: 'X', Visible: $currentObject/F) } };`, 1},
		{`alter page M.P { set (Visible: false) on dg column('N') };`, 0},
		{`alter page M.P { set (Visible: $currentObject/Featured) on txt1 };`, 0},
	} {
		var got int
		for _, v := range ValidateWidgetProperties(parseMDL(t, tc.src), "") {
			if v.RuleID == "MDL-WIDGET43" {
				got++
			}
		}
		if got != tc.want {
			t.Errorf("%s: MDL-WIDGET43 = %d, want %d", tc.src, got, tc.want)
		}
	}
}
