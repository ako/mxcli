// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Issue #650 — MDL-WIDGET04 flags a dynamictext whose template references a {N}
// placeholder with no matching parameter binding (orphaned ClientTemplate).
func TestValidateDynamicTextPlaceholders(t *testing.T) {
	dt := func(props map[string]any) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamictext", Name: "txt", Properties: props}
	}
	cases := []struct {
		name    string
		widget  *ast.WidgetV3
		wantBad bool
	}{
		{"orphan {1}", dt(map[string]any{"Content": "{1}"}), true},
		{"orphan {2} with one param", dt(map[string]any{
			"Content":       "Hi {1} {2}",
			"ContentParams": []ast.ParamAssignmentV3{{Value: "Name"}},
		}), true},
		{"bound via Attribute", dt(map[string]any{"Content": "{1}", "Attribute": "Title"}), false},
		{"bound via ContentParams", dt(map[string]any{
			"Content":       "{1}",
			"ContentParams": []ast.ParamAssignmentV3{{Value: "Title"}},
		}), false},
		{"static text, no placeholder", dt(map[string]any{"Content": "Hello"}), false},
		{"empty content (no AST placeholder)", dt(map[string]any{}), false},
		{"not a dynamictext", &ast.WidgetV3{Type: "textbox", Name: "tb", Properties: map[string]any{"Content": "{1}"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := validateDynamicTextPlaceholders(c.widget, "page X")
			if c.wantBad && v == nil {
				t.Errorf("expected MDL-WIDGET04 violation, got none")
			}
			if !c.wantBad && v != nil {
				t.Errorf("unexpected violation: %s", v.Message)
			}
			if v != nil && v.RuleID != "MDL-WIDGET04" {
				t.Errorf("RuleID = %s, want MDL-WIDGET04", v.RuleID)
			}
			if c.wantBad && v != nil && !strings.Contains(v.Message, "orphaned placeholder") {
				t.Errorf("message lacks guidance: %s", v.Message)
			}
		})
	}
}

// TestValidateStaticWidgetUnknownProps covers MDL-WIDGET07: a property a core
// widget doesn't consume is warned (not errored) so the silent drop is visible.
func TestValidateStaticWidgetUnknownProps(t *testing.T) {
	dt := func(props map[string]any) *ast.WidgetV3 {
		return &ast.WidgetV3{Type: "dynamictext", Name: "txt", Properties: props}
	}
	cases := []struct {
		name      string
		widget    *ast.WidgetV3
		wantCount int
		wantHint  string // substring expected in the message, if any
	}{
		{"known props clean", dt(map[string]any{
			"Content": "hi", "Class": "c", "DynamicClasses": "x", "RenderMode": "H1",
		}), 0, ""},
		{"describe vocabulary clean (image units)", &ast.WidgetV3{
			Type: "image", Name: "img",
			Properties: map[string]any{"WidthUnit": "pixels", "Width": 36, "HeightUnit": "pixels", "Height": 36},
		}, 0, ""},
		{"lowercase keyword clean", dt(map[string]any{"content": "hi", "dynamicclasses": "x"}), 0, ""},
		{"unknown property warns", dt(map[string]any{"Content": "hi", "TotallyMadeUp": "x"}), 1, ""},
		{"typo suggests nearest", dt(map[string]any{"Contnet": "hi"}), 1, "did you mean `Content`"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			vs := validateStaticWidgetUnknownProps(c.widget, "page X")
			if len(vs) != c.wantCount {
				t.Fatalf("got %d violations, want %d: %+v", len(vs), c.wantCount, vs)
			}
			for _, v := range vs {
				if v.RuleID != "MDL-WIDGET07" {
					t.Errorf("RuleID = %s, want MDL-WIDGET07", v.RuleID)
				}
				if v.Severity != linter.SeverityWarning {
					t.Errorf("severity = %v, want warning (must not hard-fail check)", v.Severity)
				}
			}
			if c.wantHint != "" && (len(vs) == 0 || !strings.Contains(vs[0].Message, c.wantHint)) {
				t.Errorf("expected hint %q in message, got %+v", c.wantHint, vs)
			}
		})
	}
}

// TestStaticWidgetKnownPropsCoverDescribe guards against describe-vocabulary
// drift: every property describe page can emit must be recognized, otherwise the
// describe→create roundtrip would self-warn. Representative sample of the
// describe emit set (native widgets + datagrid columns).
func TestStaticWidgetKnownPropsCoverDescribe(t *testing.T) {
	describeVocabulary := []string{
		"Action", "Alignment", "AlternativeText", "Attribute", "Attributes", "ButtonStyle",
		"Caption", "CaptionAttribute", "CaptionParams", "Class", "Collapsible", "ColumnWidth",
		"Content", "ContentParams", "DataSource", "DesignProperties", "DesktopColumns",
		"DisplayAs", "DynamicCellClass", "DynamicClasses", "Editable", "FilterType", "HeaderMode",
		"Height", "HeightUnit", "Hidable", "ImageType", "ImageUrl", "Label", "LabelWidth",
		"OnClick", "PageSize", "Pagination", "PagingPosition", "PhoneColumns", "PhoneWidth",
		"ReadOnlyStyle", "RenderMode", "Responsive", "Selection", "ShowContentAs",
		"ShowPagingButtons", "Size", "Snippet", "Sortable", "Style", "TabletColumns",
		"TabletWidth", "Tooltip", "Visible", "Width", "WidthUnit",
	}
	for _, p := range describeVocabulary {
		if !isKnownStaticWidgetProp(p) {
			t.Errorf("describe emits %q but it is not in the MDL-WIDGET07 allow-list — the describe→create roundtrip would false-warn", p)
		}
	}
}

// Bug 4 follow-up — MDL-WIDGET08: a DataView cannot use an association data
// source (Studio Pro rejects it). List widgets may.
func TestValidateStaticWidget_DataViewAssociationSource(t *testing.T) {
	assocDS := &ast.DataSourceV3{Type: "association", Reference: "M.Order_Customer", ContextVariable: ""}
	paramDS := &ast.DataSourceV3{Type: "parameter", Reference: "$Order"}

	cases := []struct {
		name   string
		widget *ast.WidgetV3
		want   bool // expect an MDL-WIDGET08 violation
	}{
		{"dataview + association → rejected", &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: map[string]any{"DataSource": assocDS}}, true},
		{"dataview + parameter → ok", &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: map[string]any{"DataSource": paramDS}}, false},
		{"listview + association → ok", &ast.WidgetV3{Type: "listview", Name: "lv", Properties: map[string]any{"DataSource": assocDS}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := false
			for _, v := range validateStaticWidget(c.widget, "page X") {
				if v.RuleID == "MDL-WIDGET08" {
					got = true
				}
			}
			if got != c.want {
				t.Errorf("MDL-WIDGET08 present = %v, want %v", got, c.want)
			}
		})
	}
}

// Bug 3 — MDL-WIDGET09: a DataView cannot use a database data source either
// (a data view shows one object; database sources belong to list widgets).
// mxbuild rejects the legacy fallback with CE7007.
func TestValidateStaticWidget_DataViewDatabaseSource(t *testing.T) {
	dbDS := &ast.DataSourceV3{Type: "database", Reference: "M.Expense"}
	mfDS := &ast.DataSourceV3{Type: "microflow", Reference: "M.GetExpense"}

	cases := []struct {
		name   string
		widget *ast.WidgetV3
		want   bool // expect an MDL-WIDGET09 violation
	}{
		{"dataview + database → rejected", &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: map[string]any{"DataSource": dbDS}}, true},
		{"dataview + microflow → ok", &ast.WidgetV3{Type: "dataview", Name: "dv", Properties: map[string]any{"DataSource": mfDS}}, false},
		{"listview + database → ok", &ast.WidgetV3{Type: "listview", Name: "lv", Properties: map[string]any{"DataSource": dbDS}}, false},
		{"datagrid + database → ok", &ast.WidgetV3{Type: "datagrid", Name: "dg", Properties: map[string]any{"DataSource": dbDS}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := false
			for _, v := range validateStaticWidget(c.widget, "page X") {
				if v.RuleID == "MDL-WIDGET09" {
					got = true
				}
			}
			if got != c.want {
				t.Errorf("MDL-WIDGET09 present = %v, want %v", got, c.want)
			}
		})
	}
}
