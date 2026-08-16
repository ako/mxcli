// SPDX-License-Identifier: Apache-2.0

package skillpack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

const javaManifest = `name: jv-pack
version: 1.0.0
rewrite:
  files:
    - java/Helper.java
    - java/actions/Do.java
installs:
  java:
    - java
`

func javaFS() fstest.MapFS {
	return fstest.MapFS{
		"jv-pack/pack.yaml": {Data: []byte(javaManifest)},
		"jv-pack/SKILL.md":  {Data: []byte("# jv\n")},
		"jv-pack/java/Helper.java": {Data: []byte(
			"package {{MODULE_PATH}};\npublic class Helper { static String m = \"{{MODULE}}\"; }\n")},
		"jv-pack/java/sub/Deep.java": {Data: []byte("package x.sub;\n")},
		"jv-pack/java/actions/Do.java": {Data: []byte(
			"package {{MODULE_PATH}}.actions;\n// generated from the MDL\n")},
	}
}

// TestInstallJavaPlacesIntoJavasource is the headline: a helper class only
// compiles where the module expects it, which is outside the pack's own
// directory — the one thing no other install target does.
func TestInstallJavaPlacesIntoJavasource(t *testing.T) {
	proj := t.TempDir()
	vars, err := ModuleVars("ODataPushdown")
	if err != nil {
		t.Fatal(err)
	}
	res, err := InstallJava(javaFS(), "jv-pack", proj, Options{Vars: vars})
	if err != nil {
		t.Fatalf("InstallJava: %v", err)
	}

	body := readFile(t, proj, "javasource/odatapushdown/Helper.java")
	if !strings.Contains(body, "package odatapushdown;") {
		t.Errorf("package not substituted: %s", body)
	}
	if !strings.Contains(body, `"ODataPushdown"`) {
		t.Errorf("{{MODULE}} not substituted: %s", body)
	}
	// Subdirectories are preserved — a flattened package does not compile.
	if _, err := os.Stat(filepath.Join(proj, "javasource/odatapushdown/sub/Deep.java")); err != nil {
		t.Errorf("subdirectory not preserved: %v", err)
	}
	if len(res.Written) != 2 {
		t.Errorf("wrote %v, want the two non-action files", res.Written)
	}
}

// TestInstallJavaExcludesGeneratedActions — mxcli writes the action classes
// from the MDL, so placing the pack's copies too means two sources of truth for
// the same files and applying the MDL immediately overwrites them.
func TestInstallJavaExcludesGeneratedActions(t *testing.T) {
	proj := t.TempDir()
	vars, _ := ModuleVars("ODataPushdown")
	res, err := InstallJava(javaFS(), "jv-pack", proj, Options{Vars: vars})
	if err != nil {
		t.Fatalf("InstallJava: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "javasource/odatapushdown/actions/Do.java")); err == nil {
		t.Error("an action class was placed; the MDL owns those")
	}
	if len(res.Excluded) != 1 {
		t.Errorf("Excluded = %v, want the one action class reported", res.Excluded)
	}
}

// TestInstallJavaRefusesToClobber is the guard-don't-drop rule (ADR-0005). From
// the outside a locally fixed helper and a stale copy look identical, so the
// choice stays with whoever knows which side is right.
func TestInstallJavaRefusesToClobber(t *testing.T) {
	proj := t.TempDir()
	vars, _ := ModuleVars("ODataPushdown")
	if _, err := InstallJava(javaFS(), "jv-pack", proj, Options{Vars: vars}); err != nil {
		t.Fatalf("first install: %v", err)
	}

	edited := filepath.Join(proj, "javasource/odatapushdown/Helper.java")
	if err := os.WriteFile(edited, []byte("package odatapushdown;\n// my fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := InstallJava(javaFS(), "jv-pack", proj, Options{Vars: vars})
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if len(res.Refused) != 1 || !strings.Contains(res.Refused[0], "Helper.java") {
		t.Errorf("Refused = %v, want the edited file named", res.Refused)
	}
	body := readFile(t, proj, "javasource/odatapushdown/Helper.java")
	if !strings.Contains(body, "// my fix") {
		t.Error("the local edit was overwritten")
	}
}

// TestInstallJavaIsIdempotent — an unchanged re-install must not churn files,
// same as every other write path in this repo (ADR-0008).
func TestInstallJavaIsIdempotent(t *testing.T) {
	proj := t.TempDir()
	vars, _ := ModuleVars("ODataPushdown")
	if _, err := InstallJava(javaFS(), "jv-pack", proj, Options{Vars: vars}); err != nil {
		t.Fatalf("first: %v", err)
	}
	res, err := InstallJava(javaFS(), "jv-pack", proj, Options{Vars: vars})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if res.Changed() || len(res.Refused) > 0 {
		t.Errorf("re-install churned: written=%v refused=%v", res.Written, res.Refused)
	}
	if len(res.Skipped) != 2 {
		t.Errorf("Skipped = %v, want both files", res.Skipped)
	}
}

// TestInstallJavaNeedsAModule — placing a class without knowing the module
// would write a package line that cannot be right.
func TestInstallJavaNeedsAModule(t *testing.T) {
	if _, err := InstallJava(javaFS(), "jv-pack", t.TempDir(), Options{}); err == nil {
		t.Error("Java was placed with no module")
	}
}

// TestInstallJavaRefusesUndeclaredDirectory — a manifest naming a directory the
// pack does not ship placed nothing while reporting success.
func TestInstallJavaRefusesUndeclaredDirectory(t *testing.T) {
	fsys := javaFS()
	fsys["jv-pack/pack.yaml"] = &fstest.MapFile{Data: []byte(
		"name: jv-pack\nversion: 1.0.0\ninstalls:\n  java:\n    - nosuchdir\n")}
	vars, _ := ModuleVars("M")
	if _, err := InstallJava(fsys, "jv-pack", t.TempDir(), Options{Vars: vars}); err == nil {
		t.Error("installs.java naming a missing directory was accepted")
	}
}

func TestNormalizeModule(t *testing.T) {
	cases := map[string][2]string{
		"ODataPushdown":  {"ODataPushdown", "odatapushdown"},
		"My Module":      {"MyModule", "mymodule"},
		"Data_Warehouse": {"Data_Warehouse", "data_warehouse"},
	}
	for in, want := range cases {
		name, pkg, err := NormalizeModule(in)
		if err != nil {
			t.Errorf("NormalizeModule(%q): %v", in, err)
			continue
		}
		if name != want[0] || pkg != want[1] {
			t.Errorf("NormalizeModule(%q) = %q/%q, want %q/%q", in, name, pkg, want[0], want[1])
		}
	}
	for _, bad := range []string{"", "   ", "9Module", "---"} {
		if _, _, err := NormalizeModule(bad); err == nil {
			t.Errorf("NormalizeModule(%q) was accepted", bad)
		}
	}
}
