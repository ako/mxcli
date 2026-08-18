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

// TestColumnNameWarningNamesTheAddressableName. DataGrid 2 stores no column
// name, so the one written in MDL is dropped and the column is addressed by a
// derived name. An author who wrote `colLabel` otherwise finds out only when
// `ALTER PAGE … ON dg1.colLabel` fails on a column they just named.
func TestColumnNameWarningNamesTheAddressableName(t *testing.T) {
	mapping := &ObjectListMapping{}
	v := validateDataGrid2ColumnName(columnWidget("colLabel", "Label", "The Label"), mapping, "page X")
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if v[0].RuleID != "MDL-WIDGET16" {
		t.Errorf("RuleID = %q", v[0].RuleID)
	}
	for _, want := range []string{`"colLabel"`, `"Label"`, "ON <grid>.Label"} {
		if !strings.Contains(v[0].Message, want) {
			t.Errorf("message does not contain %s:\n%s", want, v[0].Message)
		}
	}
}

// TestColumnNameWarningIsQuietWhenTheNamesAgree — writing the name the column
// will actually answer to is not a mistake and must not be nagged about.
func TestColumnNameWarningIsQuietWhenTheNamesAgree(t *testing.T) {
	mapping := &ObjectListMapping{}
	for _, name := range []string{"Label", "label"} {
		if v := validateDataGrid2ColumnName(columnWidget(name, "Label", ""), mapping, "page X"); len(v) != 0 {
			t.Errorf("%s warned unnecessarily: %s", name, v[0].Message)
		}
	}
}

// TestColumnNameWarningUsesTheCaptionWhenThereIsNoAttribute — a custom-content
// column keys on its caption, sanitized the same way the writer does.
func TestColumnNameWarningUsesTheCaptionWhenThereIsNoAttribute(t *testing.T) {
	mapping := &ObjectListMapping{}
	v := validateDataGrid2ColumnName(columnWidget("colActions", "", "Row actions"), mapping, "page X")
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if !strings.Contains(v[0].Message, `"Row_actions"`) {
		t.Errorf("message should name the sanitized caption:\n%s", v[0].Message)
	}
}

// TestColumnNameWarningStaysSilentWhenItCannotTell. With neither attribute nor
// caption the addressable name is colN, which depends on the column's position —
// naming a wrong one would be worse than saying nothing.
func TestColumnNameWarningStaysSilentWhenItCannotTell(t *testing.T) {
	mapping := &ObjectListMapping{}
	if v := validateDataGrid2ColumnName(columnWidget("colMystery", "", ""), mapping, "page X"); len(v) != 0 {
		t.Errorf("guessed a name it cannot know: %s", v[0].Message)
	}
}

// TestColumnNameWarningIgnoresNonColumns — the rule is scoped to DataGrid 2
// columns, not to every object-list item.
func TestColumnNameWarningIgnoresNonColumns(t *testing.T) {
	mapping := &ObjectListMapping{}
	w := columnWidget("someItem", "Label", "")
	w.Type = "TEXTBOX"
	if v := validateDataGrid2ColumnName(w, mapping, "page X"); len(v) != 0 {
		t.Errorf("warned on a non-column: %s", v[0].Message)
	}
}
