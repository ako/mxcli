// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
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
	var d executor.WidgetDescription
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
