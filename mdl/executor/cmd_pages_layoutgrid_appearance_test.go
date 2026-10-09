// SPDX-License-Identifier: Apache-2.0

// A layout-grid row's and column's appearance and alignment did not survive
// describe → exec. Describe never read them, the builder never set them and the
// writer hardcoded an empty Forms$Appearance with alignment "None", so a
// Studio Pro page lost every Atlas "Flex container" / "Column gap" / "Cards
// style" on its grid and each centred row came back uncentred (ledger
// MyFirstModule.Home_Web: 46 design-property values → 33).

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func storedDesignProp(key, valueType, option string) map[string]any {
	v := map[string]any{"$Type": valueType}
	if option != "" {
		v["Option"] = option
	}
	return map[string]any{"$Type": "Forms$DesignPropertyValue", "Key": key, "Value": v}
}

// buildDescribedGrid describes a stored layout grid, parses the output and
// builds its rows the way exec does.
func buildDescribedGrid(t *testing.T, rows ...any) ([]*pages.LayoutGridRow, string) {
	t.Helper()
	ctx, _ := newMockCtx(t)
	raw := map[string]any{"$Type": "Forms$LayoutGrid", "Name": "lg", "Rows": rows}
	widgets := parseRawWidget(ctx, raw)
	if len(widgets) != 1 {
		t.Fatalf("parseRawWidget: %d widgets, want 1", len(widgets))
	}
	var buf bytes.Buffer
	outputWidgetMDLV3(&ExecContext{Output: &buf}, widgets[0], 1)
	mdl := buf.String()
	src := "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + mdl + "};"
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("described layout grid does not parse: %v\n%s", errs, src)
	}
	page := prog.Statements[0].(*ast.CreatePageStmtV3)
	lg, err := (&pageBuilder{}).buildLayoutGridV3(page.Widgets[0])
	if err != nil {
		t.Fatalf("build: %v\n%s", err, mdl)
	}
	return lg.Rows, mdl
}

func TestLayoutGridRowColumnAppearanceRoundTrip(t *testing.T) {
	row := map[string]any{
		"$Type":                 "Forms$LayoutGridRow",
		"VerticalAlignment":     "Center",
		"HorizontalAlignment":   "End",
		"SpacingBetweenColumns": false,
		"Appearance": map[string]any{
			"$Type": "Forms$Appearance",
			"Class": "row-class",
			"DesignProperties": []any{int32(2),
				storedDesignProp("Column gap", "Forms$OptionDesignPropertyValue", "Large"),
				storedDesignProp("Cards style", "Forms$ToggleDesignPropertyValue", ""),
			},
		},
		"Columns": []any{map[string]any{
			"$Type":             "Forms$LayoutGridColumn",
			"Weight":            int64(6),
			"VerticalAlignment": "Center",
			"Appearance": map[string]any{
				"$Type": "Forms$Appearance",
				"Style": "padding: 0",
				"DesignProperties": []any{int32(2),
					storedDesignProp("Flex container", "Forms$OptionDesignPropertyValue", "Vertical (column)"),
				},
			},
		}},
	}
	rows, mdl := buildDescribedGrid(t, row)
	r := rows[0]
	if r.VerticalAlignment != "Center" || r.HorizontalAlignment != "End" || !r.NoSpacingBetweenColumns {
		t.Errorf("row alignment: vertical %q, horizontal %q, no spacing %v; want Center, End, true\n%s",
			r.VerticalAlignment, r.HorizontalAlignment, r.NoSpacingBetweenColumns, mdl)
	}
	if r.Class != "row-class" {
		t.Errorf("row class %q, want row-class\n%s", r.Class, mdl)
	}
	wantRowDPs := []pages.DesignPropertyValue{
		{Key: "Column gap", ValueType: "option", Option: "Large"},
		{Key: "Cards style", ValueType: "toggle"},
	}
	assertDesignProps(t, "row", r.DesignProperties, wantRowDPs, mdl)

	c := r.Columns[0]
	if c.VerticalAlignment != "Center" || c.Style != "padding: 0" || c.Weight != 6 {
		t.Errorf("column: vertical %q, style %q, weight %d; want Center, padding: 0, 6\n%s",
			c.VerticalAlignment, c.Style, c.Weight, mdl)
	}
	assertDesignProps(t, "column", c.DesignProperties,
		[]pages.DesignPropertyValue{{Key: "Flex container", ValueType: "option", Option: "Vertical (column)"}}, mdl)
}

// A row and column with nothing set still describe as before and build with
// every value at its default — what the writer turns into the old fixed output.
func TestLayoutGridRowColumnAppearanceDefaultsQuiet(t *testing.T) {
	row := map[string]any{
		"$Type":                 "Forms$LayoutGridRow",
		"VerticalAlignment":     "None",
		"HorizontalAlignment":   "None",
		"SpacingBetweenColumns": true,
		"Appearance":            map[string]any{"$Type": "Forms$Appearance", "DesignProperties": []any{int32(3)}},
		"Columns": []any{map[string]any{
			"$Type": "Forms$LayoutGridColumn", "Weight": int64(-1), "VerticalAlignment": "None",
			"Appearance": map[string]any{"$Type": "Forms$Appearance"},
		}},
	}
	rows, mdl := buildDescribedGrid(t, row)
	if !strings.Contains(mdl, "  row {\n") || !strings.Contains(mdl, "column (DesktopWidth: AutoFill) {") {
		t.Errorf("plain row/column no longer describes plainly:\n%s", mdl)
	}
	r, c := rows[0], rows[0].Columns[0]
	if r.VerticalAlignment != "" || r.HorizontalAlignment != "" || r.NoSpacingBetweenColumns ||
		r.Class != "" || r.Style != "" || len(r.DesignProperties) != 0 ||
		c.VerticalAlignment != "" || c.Class != "" || c.Style != "" || len(c.DesignProperties) != 0 {
		t.Errorf("plain row/column built with non-default values: row %+v, column %+v", r, c)
	}
}

// A top-level `row` builds a container around a one-row grid, and the
// appearance written on it is the container's: the inner row must not carry it
// a second time.
func TestTopLevelRowAppearanceStaysOnContainer(t *testing.T) {
	prog, errs := visitor.Build(`create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {
  row r (Class: 'outer', DesignProperties: ['Card style': on]) { column (DesktopWidth: 6) { } }
};`)
	if len(errs) > 0 {
		t.Fatal(errs)
	}
	pb := &pageBuilder{backend: &mock.MockBackend{}, widgetScope: map[string]model.ID{}}
	built, err := pb.buildWidgetV3(prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0])
	if err != nil {
		t.Fatal(err)
	}
	c := built.(*pages.Container)
	if c.Class != "outer" || len(c.DesignProperties) != 1 {
		t.Errorf("container: class %q, %d design properties; want outer, 1", c.Class, len(c.DesignProperties))
	}
	row := c.Widgets[0].(*pages.LayoutGrid).Rows[0]
	if row.Class != "" || len(row.DesignProperties) != 0 {
		t.Errorf("inner row repeats the container's appearance: class %q, %+v", row.Class, row.DesignProperties)
	}
}

func assertDesignProps(t *testing.T, what string, got, want []pages.DesignPropertyValue, mdl string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s design properties: got %+v, want %+v\n%s", what, got, want, mdl)
	}
	for i := range want {
		if got[i].Key != want[i].Key || got[i].ValueType != want[i].ValueType || got[i].Option != want[i].Option {
			t.Errorf("%s design property %d: got %+v, want %+v\n%s", what, i, got[i], want[i], mdl)
		}
	}
}
