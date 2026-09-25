// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Issue #643: a datasource-typed combo property supplied as a named value
// (optionsSourceAssociationDataSource: Module.Entity) passed `check` but was
// silently dropped at exec (CE0642). It must now be flagged (MDL-WIDGET05), and
// a real-but-unmapped property (optionsSourceAssociationCaptionType) must be a
// warning (MDL-WIDGET06), not a false "unknown property" error (MDL-WIDGET01).
func combo(props map[string]any) *ast.WidgetV3 {
	p := map[string]any{"WidgetType": "com.mendix.widget.web.combobox.Combobox"}
	for k, v := range props {
		p[k] = v
	}
	return &ast.WidgetV3{Name: "cb", Type: "pluggablewidget", Properties: p}
}

func ruleIDs(vs []linter.Violation) map[string]string {
	out := map[string]string{}
	for _, v := range vs {
		out[v.RuleID] = v.Message
	}
	return out
}

func TestIssue643_DatasourceByName_Rejected(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}

	// Datasource-typed property by name → MDL-WIDGET05 error.
	w := combo(map[string]any{
		"optionsSourceType":                   "association",
		"optionsSourceAssociationDataSource":  "Administration.Account",
		"optionsSourceAssociationCaptionType": "attribute",
	})
	got := ruleIDs(validatePluggableWidgetProperties(w, reg, "page P"))
	if _, ok := got["MDL-WIDGET05"]; !ok {
		t.Errorf("expected MDL-WIDGET05 for datasource-by-name, got rules: %v", keysOf(got))
	}
	// CaptionType is an enumeration the explicit-property pass WRITES — measured
	// (#664): `optionsSourceAssociationCaptionType: expression` persists and the
	// page builds clean — so a "not persisted" warning would be false.
	if msg, ok := got["MDL-WIDGET06"]; ok {
		t.Errorf("CaptionType is persisted by the explicit-property pass; MDL-WIDGET06 is false here: %q", msg)
	}
	if _, ok := got["MDL-WIDGET01"]; ok {
		t.Errorf("CaptionType must NOT be a false 'unknown property' (MDL-WIDGET01)")
	}
}

func TestIssue643_DatasourceClause_NotFlagged(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	// The workaround: datasource provided via the widget DataSource clause (the
	// builtin "DataSource" key), not by name → no MDL-WIDGET05.
	w := combo(map[string]any{
		"optionsSourceType": "association",
		"DataSource":        &ast.DataSourceV3{Type: "database", Reference: "Administration.Account"},
	})
	for _, v := range validatePluggableWidgetProperties(w, reg, "page P") {
		if v.RuleID == "MDL-WIDGET05" {
			t.Errorf("datasource clause must not trigger MDL-WIDGET05: %s", v.Message)
		}
	}
}

func keysOf(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// ako/mxcli#664: MDL-WIDGET06 ("recognized but not yet persisted") must fire only
// for keys the explicit-property pass cannot write. The expression caption's two
// keys ARE written (measured: exec persists both and mx check is clean), and
// DESCRIBE now emits them, so a warning would tell the reader their own
// round-tripped caption is dropped. A widgets-typed slot is still not written.
func TestWidget06_OnlyForKeysTheExplicitPassCannotWrite(t *testing.T) {
	reg := LoadWidgetRegistry("")
	if reg == nil {
		t.Fatal("built-in widget registry not available")
	}
	w := combo(map[string]any{
		"optionsSourceAssociationCaptionType":       "expression",
		"optionsSourceAssociationCaptionExpression": "$currentObject/Description",
		"optionsSourceAssociationCustomContent":     "x",
	})
	var w06 []string
	for _, v := range validatePluggableWidgetProperties(w, reg, "page P") {
		if v.RuleID == "MDL-WIDGET06" {
			w06 = append(w06, v.Message)
		}
		if v.RuleID == "MDL-WIDGET01" {
			t.Errorf("unexpected MDL-WIDGET01: %s", v.Message)
		}
	}
	joined := strings.Join(w06, "\n")
	for _, k := range []string{"optionsSourceAssociationCaptionType", "optionsSourceAssociationCaptionExpression"} {
		if strings.Contains(joined, "`"+k+"`") {
			t.Errorf("%s is written by the explicit pass; MDL-WIDGET06 is false for it", k)
		}
	}
	if !strings.Contains(joined, "optionsSourceAssociationCustomContent") {
		t.Errorf("a widgets-typed slot is not written; MDL-WIDGET06 must still fire for it (got %q)", joined)
	}
}
