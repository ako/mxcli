// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func columnWidget(name, attr, caption string) *ast.WidgetV3 {
	w := &ast.WidgetV3{Type: "COLUMN", Name: name, Properties: map[string]any{}}
	if attr != "" {
		w.Properties["Attribute"] = attr
	}
	if caption != "" {
		w.Properties["Caption"] = caption
	}
	return w
}

// gridWith wraps columns in the DataGrid the rule is reported against.
func gridWith(cols ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "DATAGRID", Name: "dg1", Children: cols}
}

// TestColumnNameWarningNamesTheAddressableName. DataGrid 2 stores no column
// name, so the one written in MDL is dropped and the column is addressed by a
// derived name. An author who wrote `colLabel` otherwise finds out only when
// `ALTER PAGE … ON dg1.colLabel` fails on a column they just named.
func TestColumnNameWarningNamesTheAddressableName(t *testing.T) {
	v := validateDataGrid2ColumnNames(gridWith(columnWidget("colLabel", "Label", "The Label")), "page X")
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if v[0].RuleID != "MDL-WIDGET16" {
		t.Errorf("RuleID = %q", v[0].RuleID)
	}
	if !strings.Contains(v[0].Message, "colLabel → Label") {
		t.Errorf("message does not name the mapping:\n%s", v[0].Message)
	}
}

// TestColumnNameWarningIsQuietWhenTheNamesAgree — writing the name the column
// will actually answer to is not a mistake and must not be nagged about.
func TestColumnNameWarningIsQuietWhenTheNamesAgree(t *testing.T) {
	for _, name := range []string{"Label", "label"} {
		if v := validateDataGrid2ColumnNames(gridWith(columnWidget(name, "Label", "")), "page X"); len(v) != 0 {
			t.Errorf("%s warned unnecessarily: %s", name, v[0].Message)
		}
	}
}

// TestColumnNameWarningUsesTheCaptionWhenThereIsNoAttribute — a custom-content
// column keys on its caption, sanitized the same way the writer does.
func TestColumnNameWarningUsesTheCaptionWhenThereIsNoAttribute(t *testing.T) {
	v := validateDataGrid2ColumnNames(gridWith(columnWidget("colActions", "", "Row actions")), "page X")
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if !strings.Contains(v[0].Message, "colActions → Row_actions") {
		t.Errorf("message should name the sanitized caption:\n%s", v[0].Message)
	}
}

// TestColumnNameWarningStaysSilentWhenItCannotTell. With neither attribute nor
// caption the addressable name is colN, which depends on the column's position —
// naming a wrong one would be worse than saying nothing.
func TestColumnNameWarningStaysSilentWhenItCannotTell(t *testing.T) {
	if v := validateDataGrid2ColumnNames(gridWith(columnWidget("colMystery", "", "")), "page X"); len(v) != 0 {
		t.Errorf("guessed a name it cannot know: %s", v[0].Message)
	}
}

// TestColumnNameWarningIgnoresNonColumns — the rule is scoped to DataGrid 2
// columns, not to every object-list item.
func TestColumnNameWarningIgnoresNonColumns(t *testing.T) {
	w := columnWidget("someItem", "Label", "")
	w.Type = "TEXTBOX"
	if v := validateDataGrid2ColumnNames(gridWith(w), "page X"); len(v) != 0 {
		t.Errorf("warned on a non-column: %s", v[0].Message)
	}
}

// TestColumnNameWarningIsOnePerGrid pins the aggregation. Emitting one info per
// column produced 44 of them on a real project, all saying the same thing about
// the same grid — the fact belongs to the grid, not to each column.
func TestColumnNameWarningIsOnePerGrid(t *testing.T) {
	grid := gridWith(
		columnWidget("colA", "Alpha", ""),
		columnWidget("colB", "Beta", ""),
		columnWidget("colC", "Gamma", ""),
	)
	v := validateDataGrid2ColumnNames(grid, "page X")
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1 for a three-column grid", len(v))
	}
	for _, want := range []string{"colA → Alpha", "colB → Beta", "colC → Gamma", "dg1"} {
		if !strings.Contains(v[0].Message, want) {
			t.Errorf("message does not mention %s:\n%s", want, v[0].Message)
		}
	}
}
