// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A fresh clone: Fieldset's .mpk is installed in widgets/ and .mxcli/ — which
// is gitignored — does not exist.
//
// The LSP loaded its widget registry with LoadUserDefinitions alone, which reads
// only .mxcli/widgets/*.def.json, so completions offered the nine embedded
// widgets and the diagnostics built on the same registry knew nothing else.
// DESCRIBE WIDGET, `widget list` and check had the same defect
// (ako/mxcli#663, mendixlabs/mxcli#1135).
func TestWidgetRegistryCompletions_InstalledWidgetNeedsNoWidgetInit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	const mpk = "com.mendix.widget.web.Fieldset.mpk"
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "expr-checker", "widgets", mpk))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widgets", mpk), data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := &mdlServer{mprPath: filepath.Join(dir, "App.mpr")}
	var labels []string
	for _, item := range s.widgetRegistryCompletions() {
		if item.Label == "fieldset" {
			return
		}
		labels = append(labels, item.Label)
	}
	t.Fatalf("installed widget `fieldset` missing from completions; got %v", labels)
}

// Control: with no project the embedded widgets are still offered.
func TestWidgetRegistryCompletions_NoProjectStillOffersEmbeddedWidgets(t *testing.T) {
	s := &mdlServer{}
	for _, item := range s.widgetRegistryCompletions() {
		if item.Label == "combobox" {
			return
		}
	}
	t.Fatal("embedded widget `combobox` missing from completions with no project")
}
