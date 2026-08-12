// SPDX-License-Identifier: Apache-2.0

// mxcli-chat FINDINGS §14 and §18, both reproduced from the published packages
// themselves before this was written:
//
//   - Atlas_Web_Content 4.3.0 bundles five Data Widgets at 3.4.0 that DataWidgets
//     3.11.3 ships at 3.11.3, so installing the modules in that order rolled the
//     project's widgets back with nothing reported. An older widget is not a
//     `mx check` error, so the app simply ran old widget code.
//   - FeedbackModule 5.0.0 ships SprintrFeedbackWidget twice — as a .mpk and as
//     an unpacked tree of the same version — and installing both left a duplicate
//     nobody asked for.
package marketplace

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// widgetMpk builds a minimal widget package declaring the given version, shaped
// like a real one: the manifest schema version on <package> is 1.0 for every
// widget ever published, and the widget's own version is on <clientModule>.
func widgetMpk(t *testing.T, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("package.xml")
	if err != nil {
		t.Fatal(err)
	}
	xml := `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.mendix.com/package/1.0/" version="1.0">
  <clientModule name="com.mendix.widget.web.Datagrid" version="` + version + `" xmlns="http://www.mendix.com/clientModule/1.0/">
    <widgetFiles><widgetFile path="Datagrid.xml"/></widgetFiles>
  </clientModule>
</package>`
	if _, err := w.Write([]byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// modulePackage builds a module .mpk carrying the given entries.
func modulePackage(t *testing.T, entries map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Module.mpk")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	for name, body := range entries {
		w, cerr := zw.Create(name)
		if cerr != nil {
			t.Fatal(cerr)
		}
		if _, werr := w.Write(body); werr != nil {
			t.Fatal(werr)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// The version on <clientModule> is the widget's; the one on <package> is the
// manifest schema and is 1.0 everywhere. Reading the wrong one makes every
// comparison come out equal, which is as broken as not comparing.
func TestClientModuleVersion_IgnoresTheManifestSchemaVersion(t *testing.T) {
	if got := widgetVersionInMpk(widgetMpk(t, "3.11.3")); got != "3.11.3" {
		t.Errorf("widget version = %q, want 3.11.3 (1.0 means the <package> attribute was read)", got)
	}
}

// The core fix: a module's bundled widget must not replace a newer copy.
func TestInstallPackageFiles_KeepsANewerWidgetTheProjectAlreadyHas(t *testing.T) {
	proj := t.TempDir()
	widget := filepath.Join(proj, "widgets", "com.mendix.widget.web.Datagrid.mpk")
	if err := os.MkdirAll(filepath.Dir(widget), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(widget, widgetMpk(t, "3.11.3"), 0o644); err != nil {
		t.Fatal(err)
	}

	pkg := modulePackage(t, map[string][]byte{
		"widgets/com.mendix.widget.web.Datagrid.mpk": widgetMpk(t, "3.4.0"),
		"themesource/x/web/design-properties.json":   []byte("{}"),
	})

	written, skipped, err := InstallPackageFiles(pkg, proj)
	if err != nil {
		t.Fatalf("InstallPackageFiles: %v", err)
	}
	if got := widgetVersionOnDisk(widget); got != "3.11.3" {
		t.Errorf("widget on disk is %s; the package's older 3.4.0 overwrote it — this is the silent rollback", got)
	}
	if len(skipped) != 1 || skipped[0].Kept != "3.11.3" || skipped[0].Offered != "3.4.0" {
		t.Errorf("skip not reported usefully: %+v — a silent skip is as bad as a silent overwrite", skipped)
	}
	// The negative half: everything else the package ships still lands.
	if len(written) != 1 || written[0] != filepath.Join("themesource", "x", "web", "design-properties.json") {
		t.Errorf("written = %v, want only the themesource file", written)
	}
}

// A guard that skips every widget would also pass the test above. A newer
// bundled widget, and an equal one, must still be installed.
func TestInstallPackageFiles_InstallsNewerAndEqualWidgets(t *testing.T) {
	for _, tc := range []struct{ have, ships string }{
		{"3.4.0", "3.11.3"},
		{"3.11.3", "3.11.3"},
	} {
		proj := t.TempDir()
		widget := filepath.Join(proj, "widgets", "com.mendix.widget.web.Datagrid.mpk")
		if err := os.MkdirAll(filepath.Dir(widget), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(widget, widgetMpk(t, tc.have), 0o644); err != nil {
			t.Fatal(err)
		}
		pkg := modulePackage(t, map[string][]byte{
			"widgets/com.mendix.widget.web.Datagrid.mpk": widgetMpk(t, tc.ships),
		})
		if _, skipped, err := InstallPackageFiles(pkg, proj); err != nil {
			t.Fatal(err)
		} else if len(skipped) != 0 {
			t.Errorf("have %s, package ships %s: skipped %+v, want it installed", tc.have, tc.ships, skipped)
		}
		if got := widgetVersionOnDisk(widget); got != tc.ships {
			t.Errorf("have %s, package ships %s: on disk %s", tc.have, tc.ships, got)
		}
	}
}

// A widget the project does not have yet is always installed — there is nothing
// to compare against, and "cannot compare" must not mean "skip".
func TestInstallPackageFiles_InstallsAWidgetTheProjectLacks(t *testing.T) {
	proj := t.TempDir()
	pkg := modulePackage(t, map[string][]byte{
		"widgets/com.mendix.widget.web.Datagrid.mpk": widgetMpk(t, "3.4.0"),
	})
	written, skipped, err := InstallPackageFiles(pkg, proj)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 0 || len(written) != 1 {
		t.Errorf("written=%v skipped=%+v, want the widget installed", written, skipped)
	}
}

// FINDINGS §18: the same widget shipped twice, once packed and once not.
func TestInstallPackageFiles_SkipsTheUnpackedTwinOfAPackagedWidget(t *testing.T) {
	proj := t.TempDir()
	pkg := modulePackage(t, map[string][]byte{
		"widgets/SprintrFeedbackWidget.mpk":                 widgetMpk(t, "12.0.4"),
		"widgets/SprintrFeedbackWidget/package.xml":         []byte("<package/>"),
		"widgets/SprintrFeedbackWidget/SprintrFeedback.xml": []byte("<widget/>"),
		"widgets/SprintrFeedbackWidget/SprintrFeedback.js":  []byte("//"),
		"widgets/OtherUnpackedWidget/OtherUnpacked.xml":     []byte("<widget/>"),
	})
	written, skipped, err := InstallPackageFiles(pkg, proj)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj, "widgets", "SprintrFeedbackWidget")); !os.IsNotExist(err) {
		t.Errorf("the unpacked twin was installed alongside the .mpk (err=%v)", err)
	}
	if len(skipped) != 3 {
		t.Errorf("skipped %d entries, want the 3 files of the unpacked twin: %+v", len(skipped), skipped)
	}
	// An unpacked widget with NO packaged twin is a different thing and must be
	// installed — skipping it would drop a widget the module needs.
	if _, err := os.Stat(filepath.Join(proj, "widgets", "OtherUnpackedWidget", "OtherUnpacked.xml")); err != nil {
		t.Errorf("an unpacked widget with no .mpk twin was skipped: %v", err)
	}
	if len(written) != 2 {
		t.Errorf("written = %v, want the .mpk and the unrelated unpacked widget", written)
	}
}

func TestVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"3.4.0", "3.11.3", true}, // 4 < 11: string compare would say otherwise
		{"3.11.3", "3.4.0", false},
		{"3.11.3", "3.11.3", false},
		{"1.2", "1.2.1", true},
		{"6.3.0", "6.3.2", true},
		// Unparseable never counts as older: the default must be to install.
		{"1.0.0-beta", "1.0.1", false},
		{"", "1.0.0", false},
	}
	for _, c := range cases {
		if got := versionLess(c.a, c.b); got != c.want {
			t.Errorf("versionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
