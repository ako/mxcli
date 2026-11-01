// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
)

// #1848: two widgets that derive one MDL name get definitions under different
// names. The catalog looked a definition up by the name derived from the
// widget ID, so both widgets were paired with the same file.
func TestWidgetDefinitionsPairsDefinitionsByWidgetID(t *testing.T) {
	dir := t.TempDir()
	const legacy = "KeyboardShortcut.widget.KeyboardShortcut"
	const pluggable = "itvisors.keyboardshortcut.KeyboardShortcut"
	writeCatalogTestMPK(t, filepath.Join(dir, "widgets", "KeyboardShortcut.mpk"), legacy, false)
	writeCatalogTestMPK(t, filepath.Join(dir, "widgets", "itvisors.KeyboardShortcut.mpk"), pluggable, true)
	defs := filepath.Join(dir, ".mxcli", "widgets")
	writeCatalogTestFile(t, filepath.Join(defs, "keyboardshortcut.def.json"),
		`{"widgetId":"`+pluggable+`","mdlName":"KEYBOARDSHORTCUT","childSlots":[{"propertyKey":"content","mdlContainer":"CONTENT"}]}`)
	writeCatalogTestFile(t, filepath.Join(defs, "customkeyboardshortcut.def.json"),
		`{"widgetId":"`+legacy+`","mdlName":"CUSTOMKEYBOARDSHORTCUT"}`)

	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b := &Builder{
		catalog:  cat,
		reader:   &mock.MockBackend{PathFunc: func() string { return filepath.Join(dir, "App.mpr") }},
		snapshot: &Snapshot{ID: "snap"},
		tx:       wrapTx(tx),
	}
	if err := b.buildWidgetDefinitions(); err != nil {
		t.Fatalf("buildWidgetDefinitions: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	names := queryStrings(t, cat, `SELECT WidgetId, MdlName FROM widget_definitions_data`)
	paths := queryStrings(t, cat, `SELECT WidgetId, DefPath FROM widget_definitions_data`)
	slots := queryStrings(t, cat, `SELECT WidgetId, CAST(ChildSlotCount AS TEXT) FROM widget_definitions_data`)
	for id, want := range map[string][3]string{
		pluggable: {"KEYBOARDSHORTCUT", ".mxcli/widgets/keyboardshortcut.def.json", "1"},
		legacy:    {"CUSTOMKEYBOARDSHORTCUT", ".mxcli/widgets/customkeyboardshortcut.def.json", "0"},
	} {
		if names[id] != want[0] || filepath.ToSlash(paths[id]) != want[1] || slots[id] != want[2] {
			t.Errorf("%s: name %q, def %q, child slots %q; want %v", id, names[id], paths[id], slots[id], want)
		}
	}
}

func writeCatalogTestMPK(t *testing.T, path, widgetID string, pluggable bool) {
	t.Helper()
	plugin := ""
	if pluggable {
		plugin = ` pluginWidget="true"`
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{
		"package.xml": `<package><clientModule name="W" version="1.0.0"><widgetFiles><widgetFile path="W.xml"/></widgetFiles></clientModule></package>`,
		"W.xml":       `<widget id="` + widgetID + `"` + plugin + `><name>W</name><properties><propertyGroup caption="General"><property key="caption" type="string"><caption>Caption</caption><description/></property></propertyGroup></properties></widget>`,
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(w, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeCatalogTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
