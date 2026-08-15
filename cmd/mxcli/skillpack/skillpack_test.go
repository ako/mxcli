// SPDX-License-Identifier: Apache-2.0

package skillpack

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

const manifest = "name: demo-pack\nversion: 1.0.0\ndescription: a demo\n"

func demoFS() fstest.MapFS {
	return fstest.MapFS{
		"demo-pack/pack.yaml":             {Data: []byte(manifest)},
		"demo-pack/SKILL.md":              {Data: []byte("---\nname: demo-pack\n---\n# Demo\n")},
		"demo-pack/references/install.md": {Data: []byte("REFERENCES install\n")},
		"demo-pack/specs/install.md":      {Data: []byte("SPECS install\n")},
		"demo-pack/specs/bar.json":        {Data: []byte(`{"mark":"bar"}`)},
		"demo-pack/scripts/check.mjs":     {Data: []byte("process.exit(0)\n")},
		"demo-pack/mdl/actions.mdl":       {Data: []byte("-- actions\n")},
		"demo-pack/scripts/_helper.mjs":   {Data: []byte("// underscore-prefixed\n")},
	}
}

// TestInstallPreservesStructure is the flattening hazard, and the two
// `install.md` files are the whole point: the old skill writer joins with
// d.Name(), so under it references/install.md and specs/install.md become one
// file and the loser vanishes. That does not error — it produces a plausible
// directory with a file silently missing.
func TestInstallPreservesStructure(t *testing.T) {
	dest := t.TempDir()
	res, err := Install(demoFS(), "demo-pack", dest)
	if err != nil {
		t.Fatalf("Install: %v", err)
	}

	want := map[string]string{
		"pack.yaml":             manifest,
		"SKILL.md":              "---\nname: demo-pack\n---\n# Demo\n",
		"references/install.md": "REFERENCES install\n",
		"specs/install.md":      "SPECS install\n",
		"specs/bar.json":        `{"mark":"bar"}`,
		"scripts/check.mjs":     "process.exit(0)\n",
		"scripts/_helper.mjs":   "// underscore-prefixed\n",
		"mdl/actions.mdl":       "-- actions\n",
	}
	for rel, content := range want {
		got, err := os.ReadFile(filepath.Join(dest, "demo-pack", filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			continue
		}
		if string(got) != content {
			t.Errorf("%s = %q, want %q", rel, got, content)
		}
	}
	if len(res.Written) != len(want) {
		t.Errorf("wrote %d files (%v), want %d", len(res.Written), res.Written, len(want))
	}
}

// TestInstallIsIdempotent — a second install with nothing changed must write
// nothing. `mxcli init --sync-skills` runs on every session start, so churn here
// shows up as a dirty working tree on every boot.
func TestInstallIsIdempotent(t *testing.T) {
	dest := t.TempDir()
	if _, err := Install(demoFS(), "demo-pack", dest); err != nil {
		t.Fatalf("first Install: %v", err)
	}
	res, err := Install(demoFS(), "demo-pack", dest)
	if err != nil {
		t.Fatalf("second Install: %v", err)
	}
	if res.Changed() {
		t.Errorf("second install changed things: written=%v pruned=%v", res.Written, res.Pruned)
	}
}

// TestInstallPrunesDroppedFiles is the stale-asset hazard. A pack that drops a
// spec in v2 must not leave v1's behind — a stale spec template is worse than a
// missing one, because it still looks current.
func TestInstallPrunesDroppedFiles(t *testing.T) {
	dest := t.TempDir()
	if _, err := Install(demoFS(), "demo-pack", dest); err != nil {
		t.Fatalf("v1 Install: %v", err)
	}

	v2 := demoFS()
	delete(v2, "demo-pack/specs/bar.json")
	delete(v2, "demo-pack/scripts/check.mjs")
	delete(v2, "demo-pack/scripts/_helper.mjs") // empties scripts/ entirely

	res, err := Install(v2, "demo-pack", dest)
	if err != nil {
		t.Fatalf("v2 Install: %v", err)
	}

	for _, gone := range []string{"specs/bar.json", "scripts/check.mjs"} {
		if _, err := os.Stat(filepath.Join(dest, "demo-pack", filepath.FromSlash(gone))); err == nil {
			t.Errorf("%s survived a pack that no longer ships it", gone)
		}
	}
	if len(res.Pruned) != 3 {
		t.Errorf("pruned %v, want 3 files", res.Pruned)
	}
	// A directory emptied by the prune goes too, but the pack root stays.
	if _, err := os.Stat(filepath.Join(dest, "demo-pack", "scripts")); err == nil {
		t.Error("scripts/ was left behind empty")
	}
	if _, err := os.Stat(filepath.Join(dest, "demo-pack", "SKILL.md")); err != nil {
		t.Errorf("still-shipped file was pruned: %v", err)
	}
}

// TestPruneLeavesUnrelatedPacksAlone — installing one pack must not reach into
// another's directory.
func TestPruneLeavesUnrelatedPacksAlone(t *testing.T) {
	dest := t.TempDir()
	other := filepath.Join(dest, "other-pack")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(other, "SKILL.md")
	if err := os.WriteFile(keep, []byte("other"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(demoFS(), "demo-pack", dest); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("installing demo-pack disturbed other-pack: %v", err)
	}
}

// TestLoadRejectsNameMismatch — the directory is what the user types and the
// manifest name is what everything else keys on. If they drift, `skill remove
// <name>` cannot find what `skill add <name>` wrote.
func TestLoadRejectsNameMismatch(t *testing.T) {
	fsys := fstest.MapFS{
		"demo-pack/pack.yaml": {Data: []byte("name: something-else\n")},
	}
	if _, err := Load(fsys, "demo-pack"); err == nil {
		t.Error("a manifest naming a different pack was accepted")
	}
}

// TestListRequiresAManifest — a directory without one is an error, not a skip.
// A pack that silently fails to appear is indistinguishable from one that was
// never vendored, which is a bad half-hour for whoever added it.
func TestListRequiresAManifest(t *testing.T) {
	fsys := fstest.MapFS{
		"good/pack.yaml": {Data: []byte("name: good\n")},
		"bad/SKILL.md":   {Data: []byte("# no manifest\n")},
	}
	if _, err := List(fsys); err == nil {
		t.Error("a directory with no pack.yaml was silently skipped")
	}
}

// TestRemoveRejectsTraversal — the pack name reaches the filesystem, so
// `skill remove ../../something` must not escape the skills directory.
func TestRemoveRejectsTraversal(t *testing.T) {
	dest := t.TempDir()
	for _, bad := range []string{"..", "../evil", "a/b", ""} {
		if _, err := Remove(dest, bad); err == nil {
			t.Errorf("Remove(%q) was accepted", bad)
		}
	}
}

// TestWritesToModelFlagsMDLInstalls — copying documentation must never be
// confused with writing Java actions into the .mpr.
func TestWritesToModelFlagsMDLInstalls(t *testing.T) {
	docsOnly := Pack{Manifest: Manifest{Name: "a"}}
	if docsOnly.WritesToModel() {
		t.Error("a docs-only pack claims it writes to the model")
	}
	withMDL := Pack{Manifest: Manifest{Name: "b", Installs: Installs{MDL: []string{"mdl/x.mdl"}}}}
	if !withMDL.WritesToModel() {
		t.Error("a pack with installs.mdl does not report writing to the model")
	}
}
