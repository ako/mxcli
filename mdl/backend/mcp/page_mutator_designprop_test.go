// SPDX-License-Identifier: Apache-2.0

package mcp

import "testing"

// TestPageMutator_DesignProperties covers ALTER STYLING design-property edits
// over MCP: SET (option/toggle), update-in-place across kinds, REMOVE, CLEAR.
func TestPageMutator_DesignProperties(t *testing.T) {
	m := newTestMutator()

	if err := m.SetDesignProperty("t1", "Font Weight", "option", "Bold"); err != nil {
		t.Fatalf("set option: %v", err)
	}
	if err := m.SetDesignProperty("t1", "Cards style", "toggle", ""); err != nil {
		t.Fatalf("set toggle: %v", err)
	}
	_, _, _, w, _ := findWidget(m.content, "t1")
	dp := w["appearance"].(map[string]any)["designProperties"].(map[string]any)
	if dp["option:Font Weight"] != "Bold" {
		t.Errorf("option not set: %v", dp)
	}
	if dp["toggle:Cards style"] != true {
		t.Errorf("toggle not set: %v", dp)
	}

	// Re-SET the same property name with a different kind: single entry, overwritten.
	if err := m.SetDesignProperty("t1", "Font Weight", "toggle", ""); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	if _, has := dp["option:Font Weight"]; has {
		t.Errorf("re-set should drop the prior typed key: %v", dp)
	}
	if dp["toggle:Font Weight"] != true {
		t.Errorf("re-set as toggle: %v", dp)
	}

	// REMOVE.
	if err := m.RemoveDesignProperty("t1", "Cards style"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if _, has := dp["toggle:Cards style"]; has {
		t.Errorf("remove failed: %v", dp)
	}

	// CLEAR.
	if err := m.ClearDesignProperties("t1"); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if after := w["appearance"].(map[string]any)["designProperties"].(map[string]any); len(after) != 0 {
		t.Errorf("clear failed: %v", after)
	}

	// Unknown widget → error, no panic.
	if err := m.SetDesignProperty("nope", "X", "option", "Y"); err == nil {
		t.Error("expected widget-not-found error")
	}
}
