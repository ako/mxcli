// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// TestWidgetDescribe_EmbeddedCombobox runs `widget describe COMBOBOX --format json`
// against mxcli's embedded template (no project) and checks the discovered format.
func TestWidgetDescribe_EmbeddedCombobox(t *testing.T) {
	var out strings.Builder
	cmd := widgetDescribeCmd
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := runWidgetDescribe(cmd, []string{"COMBOBOX"}); err != nil {
		t.Fatalf("describe COMBOBOX: %v", err)
	}
	// Re-run with --format json by setting the flag.
	out.Reset()
	_ = cmd.Flags().Set("format", "json")
	defer cmd.Flags().Set("format", "text")
	if err := runWidgetDescribe(cmd, []string{"COMBOBOX"}); err != nil {
		t.Fatalf("describe COMBOBOX json: %v", err)
	}
	var d widgetDescription
	if err := json.Unmarshal([]byte(out.String()), &d); err != nil {
		t.Fatalf("unmarshal json: %v\n%s", err, out.String())
	}
	if d.WidgetID != "com.mendix.widget.web.combobox.Combobox" {
		t.Errorf("widgetId = %q", d.WidgetID)
	}
	if d.Source != "embedded template" {
		t.Errorf("source = %q, want embedded template", d.Source)
	}
	if len(d.Properties) == 0 {
		t.Fatal("no properties described")
	}
	// The declared order must place the system properties mid-list (after
	// selectAllButtonCaption), not at the end — the ComboBox order fix.
	idx := map[string]int{}
	for i, p := range d.Properties {
		idx[p.Key] = i
	}
	for _, k := range []string{"Label", "Visibility", "Editability", "customEditability"} {
		if _, ok := idx[k]; !ok {
			t.Errorf("expected property %q in described format", k)
		}
	}
	if idx["Label"] < idx["selectAllButtonCaption"] || idx["Label"] > idx["customEditability"] {
		t.Errorf("Label at %d not between selectAllButtonCaption (%d) and customEditability (%d)",
			idx["Label"], idx["selectAllButtonCaption"], idx["customEditability"])
	}
}

// TestWidgetDescribe_UnknownWidget reports a helpful error.
func TestWidgetDescribe_UnknownWidget(t *testing.T) {
	reg, err := executor.NewWidgetRegistry()
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	id, _ := resolveWidgetTarget(reg, "NOPE")
	if id != "" {
		t.Errorf("resolveWidgetTarget(NOPE) = %q, want empty", id)
	}
	// DATAGRID2 resolves via the builtin alias even without a .def.json entry.
	if id, _ := resolveWidgetTarget(reg, "datagrid2"); id != "com.mendix.widget.web.datagrid.Datagrid" {
		t.Errorf("resolveWidgetTarget(datagrid2) = %q", id)
	}
}

// TestConditionText renders the four operators as readable English.
func TestConditionText(t *testing.T) {
	cases := []struct {
		op, val, want string
	}{
		{"eq", "None", `itemSelection = "None"`},
		{"ne", "Multi", `itemSelection ≠ "Multi"`},
		{"truthy", "", "itemSelection is set"},
		{"falsy", "", "itemSelection is not set"},
	}
	for _, c := range cases {
		got := conditionText(&types.WidgetVisibilityCondition{PropertyKey: "itemSelection", Operator: c.op, Value: c.val})
		if got != c.want {
			t.Errorf("op %s: got %q, want %q", c.op, got, c.want)
		}
	}
}
