// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"go.lsp.dev/protocol"
)

func TestExtractPageParamNames(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "single param",
			text:     "CREATE PAGE Mod.Page (Params: { $Order: Mod.Order })",
			expected: []string{"Order"},
		},
		{
			name:     "multiple params",
			text:     "CREATE PAGE Mod.Page (\n  Params: { $Customer: Mod.Customer, $Helper: Mod.Helper }\n)",
			expected: []string{"Customer", "Helper"},
		},
		{
			name:     "no params",
			text:     "CREATE PAGE Mod.Page (Title: 'Test')",
			expected: nil,
		},
		{
			name:     "skip DECLARE variables",
			text:     "DECLARE $Temp String = '';\n$Order: Mod.Order",
			expected: []string{"Order"},
		},
		{
			name:     "reject body $currentObject reference",
			text:     "CREATE PAGE Mod.Page ($Order: Mod.Order) {\n  -- Context: $currentObject (Mod.Order)\n  DATAVIEW dv1 (DataSource: $Order)\n}",
			expected: []string{"Order"},
		},
		{
			name:     "reject body $var usage without colon",
			text:     "CREATE PAGE Mod.Page ($Item: Mod.Item) {\n  TEXTBOX t1 (Attribute: $Item/Name)\n}",
			expected: []string{"Item"},
		},
		{
			name:     "reject comment lines",
			text:     "-- $FakeParam: NotReal\n$RealParam: Mod.Entity",
			expected: []string{"RealParam"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractPageParamNames(tt.text)
			if len(got) != len(tt.expected) {
				t.Errorf("extractPageParamNames() got %v, want %v", got, tt.expected)
				return
			}
			for i, name := range got {
				if name != tt.expected[i] {
					t.Errorf("extractPageParamNames()[%d] = %q, want %q", i, name, tt.expected[i])
				}
			}
		})
	}
}

func TestVariableCompletionItems(t *testing.T) {
	s := &mdlServer{}
	docText := "CREATE PAGE Mod.Page (\n  Params: { $Customer: Mod.Customer }\n) {\n  DATAVIEW dv1 (DataSource: $Customer) {\n    TEXTBOX t1 (Attribute: $\n"

	// Cursor at last line (line 4, 0-based)
	items := s.variableCompletionItems(docText, "$", 4)
	if len(items) == 0 {
		t.Fatal("expected completion items for $ prefix")
	}

	// Should contain $currentObject
	foundCurrentObj := false
	foundCustomer := false
	for _, item := range items {
		if item.Label == "$currentObject" {
			foundCurrentObj = true
		}
		if item.Label == "$Customer" {
			foundCustomer = true
		}
	}
	if !foundCurrentObj {
		t.Error("expected $currentObject in completion items")
	}
	if !foundCustomer {
		t.Error("expected $Customer in completion items")
	}
}

func TestVariableCompletionItems_DataGridContext(t *testing.T) {
	s := &mdlServer{}
	docText := "CREATE PAGE Mod.Page ($Order: Sales.Order) {\n  DATAGRID dgOrders (DataSource: DATABASE FROM Sales.Order) {\n    COLUMN Name {\n      TEXTBOX t1 (Attribute: $\n"

	// Cursor inside DATAGRID column (line 3)
	items := s.variableCompletionItems(docText, "$", 3)

	var currentObjDetail string
	foundSelection := false
	for _, item := range items {
		if item.Label == "$currentObject" {
			currentObjDetail = item.Detail
		}
		if item.Label == "$dgOrders" {
			foundSelection = true
		}
	}
	if currentObjDetail != "Sales.Order" {
		t.Errorf("expected $currentObject detail = %q, got %q", "Sales.Order", currentObjDetail)
	}
	if !foundSelection {
		t.Error("expected $dgOrders selection variable in completion items")
	}
}

func TestScanEnclosingDataContainer(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		cursorLine     int
		wantEntity     string
		wantWidgetName string
	}{
		{
			name:           "inside DATAVIEW",
			text:           "DATAVIEW dv1 (DataSource: DATABASE FROM Mod.Order) {\n  TEXTBOX t1\n}",
			cursorLine:     1,
			wantEntity:     "Mod.Order",
			wantWidgetName: "",
		},
		{
			name:           "inside DATAGRID",
			text:           "DATAGRID dg1 (DataSource: DATABASE FROM Shop.Product) {\n  COLUMN Name {\n    TEXTBOX t1\n  }\n}",
			cursorLine:     2,
			wantEntity:     "Shop.Product",
			wantWidgetName: "dg1",
		},
		{
			name:           "no container",
			text:           "CREATE PAGE Mod.Page ($P: Mod.E) {\n  TEXTBOX t1\n}",
			cursorLine:     1,
			wantEntity:     "",
			wantWidgetName: "",
		},
		{
			name:           "nested DataView inside DataGrid",
			text:           "DATAGRID dg1 (DataSource: DATABASE FROM Sales.Order) {\n  COLUMN col1 {\n    DATAVIEW dv1 (DataSource: DATABASE FROM Sales.Line) {\n      TEXTBOX t1\n    }\n  }\n}",
			cursorLine:     3,
			wantEntity:     "Sales.Line",
			wantWidgetName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, widgetName := scanEnclosingDataContainer(tt.text, tt.cursorLine)
			if entity != tt.wantEntity {
				t.Errorf("scanEnclosingDataContainer() entity = %q, want %q", entity, tt.wantEntity)
			}
			if widgetName != tt.wantWidgetName {
				t.Errorf("scanEnclosingDataContainer() widgetName = %q, want %q", widgetName, tt.wantWidgetName)
			}
		})
	}
}

// TestWidgetPropertyCompletion verifies that LSP completion inside a
// pluggable widget's (...) block suggests the widget's known property keys
// rather than the generic MDL keyword list.
func TestWidgetPropertyCompletion(t *testing.T) {
	s := &mdlServer{}

	tests := []struct {
		name        string
		text        string
		cursorLine  int
		linePrefix  string
		wantPresent []string // labels that must appear
		wantAbsent  []string // labels that must NOT appear (e.g. typos, unrelated)
	}{
		{
			name: "combobox property completion (built-in widget)",
			text: "create page X (Title: 'T') {\n" +
				"  combobox cb1 (\n" +
				"    \n" +
				"  )\n" +
				"}",
			cursorLine:  2,
			linePrefix:  "    ",
			wantPresent: []string{"Class", "Style", "Visible"},
			wantAbsent:  []string{"ACCORDION", "CREATE", "ENTITY"},
		},
		{
			name: "filtering by partial prefix",
			text: "create page X (Title: 'T') {\n" +
				"  combobox cb1 (\n" +
				"    Vis\n" +
				"  )\n" +
				"}",
			cursorLine:  2,
			linePrefix:  "    Vis",
			wantPresent: []string{"Visible"},
			wantAbsent:  []string{"Class", "Style"}, // filtered out
		},
		{
			name: "outside widget block → no widget completions",
			text: "create page X (Title: 'T') {\n" +
				"  \n" +
				"}",
			cursorLine: 1,
			linePrefix: "  ",
			wantAbsent: []string{"Visible", "Class"},
		},
		{
			name: "pluggablewidget by ID form",
			text: "create page X (Title: 'T') {\n" +
				"  pluggablewidget 'com.mendix.widget.web.combobox.Combobox' cb1 (\n" +
				"    \n" +
				"  )\n" +
				"}",
			cursorLine:  2,
			linePrefix:  "    ",
			wantPresent: []string{"Class", "Visible"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := s.widgetPropertyCompletionItems(tt.text, tt.linePrefix, tt.cursorLine)
			labels := make(map[string]bool, len(items))
			for _, it := range items {
				labels[it.Label] = true
			}
			for _, w := range tt.wantPresent {
				if !labels[w] {
					t.Errorf("expected label %q in completion items, got: %v", w, mapKeys(labels))
				}
			}
			for _, w := range tt.wantAbsent {
				if labels[w] {
					t.Errorf("expected label %q NOT in completion items, but found it", w)
				}
			}
		})
	}
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestWidgetPropertyDiagnostics verifies that unknown property keys on a
// pluggable widget surface as LSP diagnostics through runSemanticValidation
// (the path that fires on every keystroke).
func TestWidgetPropertyDiagnostics(t *testing.T) {
	s := &mdlServer{}

	tests := []struct {
		name        string
		text        string
		wantRule    string   // RuleID expected in at least one diagnostic
		wantInMsg   []string // substrings expected somewhere in the diagnostics
		wantNoMatch bool     // when no widget diagnostic should fire
	}{
		{
			name: "typo on combobox property surfaces MDL-WIDGET01",
			text: "create page Mod.X (Title: 'T') {\n" +
				"  combobox cb1 (\n" +
				"    optionsSourcType: 'enumeration'\n" +
				"  )\n" +
				"}",
			wantRule:  "MDL-WIDGET01",
			wantInMsg: []string{"optionsSourcType", "combobox"},
		},
		{
			name: "valid property — no widget diagnostic",
			text: "create page Mod.X (Title: 'T') {\n" +
				"  combobox cb1 (\n" +
				"    optionsSourceType: 'enumeration'\n" +
				"  )\n" +
				"}",
			wantNoMatch: true,
		},
		{
			name: "alter page insert — typo flagged",
			text: "ALTER PAGE Mod.X {\n" +
				"  INSERT AFTER target1 {\n" +
				"    combobox cb1 (badprop: 'x')\n" +
				"  }\n" +
				"};",
			wantRule:  "MDL-WIDGET01",
			wantInMsg: []string{"badprop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diags := s.runSemanticValidation(tt.text)
			var widgetDiags []protocol.Diagnostic
			for _, d := range diags {
				if code, ok := d.Code.(string); ok && code == "MDL-WIDGET01" {
					widgetDiags = append(widgetDiags, d)
				}
			}
			if tt.wantNoMatch {
				if len(widgetDiags) != 0 {
					t.Errorf("expected no widget diagnostics, got: %v", widgetDiags)
				}
				return
			}
			if len(widgetDiags) == 0 {
				t.Fatalf("expected at least one MDL-WIDGET01 diagnostic, got none (all diags: %v)", diags)
			}
			for _, sub := range tt.wantInMsg {
				found := false
				for _, d := range widgetDiags {
					if strings.Contains(d.Message, sub) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected substring %q in a widget diagnostic; messages: %v", sub, widgetDiags)
				}
			}
		})
	}
}

// TestWidgetPropertyHover verifies LSP hover surfaces widget property
// descriptions / type / default from the .def.json content.
func TestWidgetPropertyHover(t *testing.T) {
	s := &mdlServer{}

	tests := []struct {
		name    string
		text    string
		line    uint32
		col     uint32
		wantSub []string // substrings expected in the rendered hover Value
		wantNil bool     // when the hover should not fire
	}{
		{
			name: "cursor on combobox property key shows hover",
			text: "create page X (Title: 'T') {\n" +
				"  combobox cb1 (\n" +
				"    optionsSourceType: 'enumeration'\n" +
				"  )\n" +
				"}",
			line:    2,
			col:     12, // mid-word in "optionsSourceType"
			wantSub: []string{"optionsSourceType", "combobox"},
		},
		{
			name: "cursor on universal Class property",
			text: "create page X (Title: 'T') {\n" +
				"  combobox cb1 (\n" +
				"    Class: 'foo'\n" +
				"  )\n" +
				"}",
			line:    2,
			col:     6,
			wantNil: true, // Class isn't a registered property; falls through
		},
		{
			name: "cursor outside any widget block",
			text: "create page X (Title: 'T') {\n" +
				"  \n" +
				"}",
			line:    1,
			col:     2,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := s.widgetPropertyHover(tt.text, protocol.Position{Line: tt.line, Character: tt.col})
			if tt.wantNil {
				if h != nil {
					t.Errorf("expected nil hover; got %v", h.Contents.Value)
				}
				return
			}
			if h == nil {
				t.Fatalf("expected hover, got nil")
			}
			content := h.Contents.Value
			for _, sub := range tt.wantSub {
				if !strings.Contains(content, sub) {
					t.Errorf("hover missing %q. Full content:\n%s", sub, content)
				}
			}
		})
	}
}
