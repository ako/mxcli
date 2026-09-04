// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

func TestWidgetDescribe_UnknownWidget(t *testing.T) {
	reg, err := NewWidgetRegistry()
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
