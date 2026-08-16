// SPDX-License-Identifier: Apache-2.0

package skillpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const nsManifest = `name: ns-pack
version: 1.0.0
rewrite:
  files:
    - widget/package.json
    - widget/src/VegaChart.xml
`

func nsFS() fstest.MapFS {
	return fstest.MapFS{
		"ns-pack/pack.yaml": {Data: []byte(nsManifest)},
		"ns-pack/SKILL.md":  {Data: []byte("# ns\n")},
		"ns-pack/widget/package.json": {Data: []byte(
			`{"packagePath":"{{NAMESPACE}}.widget.web","config":{"projectPath":"{{PROJECT_PATH}}"}}`)},
		"ns-pack/widget/src/VegaChart.xml": {Data: []byte(
			`<widget id="{{NAMESPACE}}.widget.web.vegachart.VegaChart" />`)},
		// Not listed under rewrite.files: a spec that happens to contain brace
		// syntax must come through byte-for-byte.
		"ns-pack/specs/bar.json": {Data: []byte(`{"mark":"bar","text":"{{NAMESPACE}} is not a token here"}`)},
	}
}

// TestInstallSubstitutesNamespace is the headline behaviour: the widget id a
// project ends up with must be the project's own, in every file that carries it.
func TestInstallSubstitutesNamespace(t *testing.T) {
	dest := t.TempDir()
	_, err := InstallWith(nsFS(), "ns-pack", dest, Options{Vars: Vars("acme", "../../..")})
	if err != nil {
		t.Fatalf("InstallWith: %v", err)
	}

	pkg := readFile(t, dest, "ns-pack/widget/package.json")
	if !strings.Contains(pkg, `"packagePath":"acme.widget.web"`) {
		t.Errorf("packagePath not substituted: %s", pkg)
	}
	if !strings.Contains(pkg, `"projectPath":"../../.."`) {
		t.Errorf("projectPath not substituted: %s", pkg)
	}
	xml := readFile(t, dest, "ns-pack/widget/src/VegaChart.xml")
	if !strings.Contains(xml, `id="acme.widget.web.vegachart.VegaChart"`) {
		t.Errorf("widget id not substituted: %s", xml)
	}
}

// TestRewriteTouchesOnlyDeclaredFiles — substitution is a whitelist, not a scan.
// A pack ships megabytes of built JS and spec JSON, and a blind replace across
// all of it would rewrite content that merely looks like a token.
func TestRewriteTouchesOnlyDeclaredFiles(t *testing.T) {
	dest := t.TempDir()
	if _, err := InstallWith(nsFS(), "ns-pack", dest, Options{Vars: Vars("acme", ".")}); err != nil {
		t.Fatalf("InstallWith: %v", err)
	}
	spec := readFile(t, dest, "ns-pack/specs/bar.json")
	if !strings.Contains(spec, "{{NAMESPACE}} is not a token here") {
		t.Errorf("an undeclared file was rewritten: %s", spec)
	}
}

// TestInstallRefusesWithoutVars — a pack whose id must carry the destination's
// namespace cannot be installed without one. Silently shipping the placeholder
// (or the pack author's own namespace) is the failure this design exists to make
// impossible.
func TestInstallRefusesWithoutVars(t *testing.T) {
	dest := t.TempDir()
	if _, err := Install(nsFS(), "ns-pack", dest); err == nil {
		t.Error("a pack needing a namespace installed without one")
	}
}

// TestInstallRefusesUnknownToken — a token the caller has no value for must stop
// the install, not go out unsubstituted.
func TestInstallRefusesUnknownToken(t *testing.T) {
	fsys := nsFS()
	fsys["ns-pack/widget/src/VegaChart.xml"] = &fstest.MapFile{
		Data: []byte(`<widget id="{{NAMESPACE}}.{{UNDECLARED}}.VegaChart" />`)}
	dest := t.TempDir()
	_, err := InstallWith(fsys, "ns-pack", dest, Options{Vars: Vars("acme", ".")})
	if err == nil || !strings.Contains(err.Error(), "UNDECLARED") {
		t.Errorf("expected a refusal naming UNDECLARED, got %v", err)
	}
}

// TestInstallRefusesStaleManifest covers both directions of drift: a declared
// file that carries no token (the file changed under the manifest), and a
// declared file the pack does not ship at all. Either one means a file somebody
// intended to rewrite went out untouched.
func TestInstallRefusesStaleManifest(t *testing.T) {
	t.Run("declared file has no token", func(t *testing.T) {
		fsys := nsFS()
		fsys["ns-pack/widget/src/VegaChart.xml"] = &fstest.MapFile{Data: []byte(`<widget id="hardcoded" />`)}
		if _, err := InstallWith(fsys, "ns-pack", t.TempDir(), Options{Vars: Vars("acme", ".")}); err == nil {
			t.Error("a declared file with no token was accepted")
		}
	})
	t.Run("declared file is not shipped", func(t *testing.T) {
		fsys := nsFS()
		delete(fsys, "ns-pack/widget/src/VegaChart.xml")
		if _, err := InstallWith(fsys, "ns-pack", t.TempDir(), Options{Vars: Vars("acme", ".")}); err == nil {
			t.Error("a manifest naming a file the pack does not ship was accepted")
		}
	})
}

// TestUpgradeKeepsTheInstalledNamespace — re-deriving on upgrade would change a
// widget id when the project is renamed, and every page referencing it would be
// pointing at a widget that no longer exists under that name.
func TestUpgradeKeepsTheInstalledNamespace(t *testing.T) {
	dest := t.TempDir()
	if _, err := InstallWith(nsFS(), "ns-pack", dest, Options{Vars: Vars("acme", ".")}); err != nil {
		t.Fatalf("first install: %v", err)
	}
	// An upgrade supplies no vars — it must recover them from the lock.
	if _, err := Install(nsFS(), "ns-pack", dest); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	xml := readFile(t, dest, "ns-pack/widget/src/VegaChart.xml")
	if !strings.Contains(xml, `id="acme.`) {
		t.Errorf("upgrade lost the installed namespace: %s", xml)
	}
}

// TestLockSurvivesPrune — the lock is written by the install, not shipped by the
// pack, so the prune that removes everything unshipped must not take it.
func TestLockSurvivesPrune(t *testing.T) {
	dest := t.TempDir()
	if _, err := InstallWith(nsFS(), "ns-pack", dest, Options{Vars: Vars("acme", ".")}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "ns-pack", LockName)); err != nil {
		t.Fatalf("lock missing after install: %v", err)
	}
	if _, err := Install(nsFS(), "ns-pack", dest); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "ns-pack", LockName)); err != nil {
		t.Errorf("prune removed the lock: %v", err)
	}
}

func TestNormalizeNamespace(t *testing.T) {
	ok := map[string]string{
		"acme":        "acme",
		"Acme":        "acme",
		"My App":      "myapp",
		"my-app_1112": "myapp1112",
		"App1112":     "app1112",
	}
	for in, want := range ok {
		got, err := NormalizeNamespace(in)
		if err != nil {
			t.Errorf("NormalizeNamespace(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeNamespace(%q) = %q, want %q", in, got, want)
		}
	}
	// A leading digit is rejected rather than silently prefixed: a namespace the
	// user did not choose is as wrong as one that does not fit, and they would
	// find out with the id already baked into pages.
	for _, bad := range []string{"", "   ", "123", "1app", "---"} {
		if got, err := NormalizeNamespace(bad); err == nil {
			t.Errorf("NormalizeNamespace(%q) = %q, want an error", bad, got)
		}
	}
}

func TestNamespaceFromProject(t *testing.T) {
	cases := map[string]string{
		"App1112.mpr":           "app1112",
		"/tmp/wd/My-App.mpr":    "myapp",
		"./projects/Ledger.mpr": "ledger",
	}
	for in, want := range cases {
		got, err := NamespaceFromProject(in)
		if err != nil {
			t.Errorf("NamespaceFromProject(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NamespaceFromProject(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWidgetID(t *testing.T) {
	if got := WidgetID("acme", "vegachart.VegaChart"); got != "acme.widget.web.vegachart.VegaChart" {
		t.Errorf("WidgetID = %q", got)
	}
}

func readFile(t *testing.T, dest, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(b)
}
