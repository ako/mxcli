//go:build integration

// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// shortTemp returns a work dir with a short path. mx create-project extracts its
// template with .NET path handling and dies with PathTooLongException under a
// deep directory, so t.TempDir() nested inside a long module path is not safe
// here — this was found the hard way.
func shortTemp(t *testing.T) string {
	d, err := os.MkdirTemp("", "mxref")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// mxVersionAvailable reports whether the exact mxbuild for want is cached.
// ResolveMxForVersion falls back to any cached version, so its success is not
// evidence — the cache path is checked directly.
func mxVersionAvailable(want string) bool {
	p := docker.CachedMxPath(want)
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

// TestPackageProject_RefusesAVersionMismatch is the guard that matters most here,
// and it is the one case this environment can always exercise.
//
// ResolveMxForVersion silently falls back to any cached mxbuild: asking for a
// version that is not installed returns a different one, and mx create-project
// stamps the reference project with whatever binary ran it. Building the
// baseline one Mendix version away from the project under comparison would make
// the platform's own conversions look like user edits — false findings that are
// indistinguishable from real ones. PackageProject must refuse, not warn.
func TestPackageProject_RefusesAVersionMismatch(t *testing.T) {
	const absent = "9.0.0" // old enough that no cache in CI or dev holds it
	if mxVersionAvailable(absent) {
		t.Skipf("Mendix %s is cached here, so the mismatch path cannot be exercised", absent)
	}
	if docker.AnyCachedMxPath() == "" {
		t.Skip("no mxbuild cached; nothing to fall back to")
	}

	_, err := PackageProject(context.Background(), "", absent, t.TempDir(), testBackend)
	if err == nil {
		t.Fatal("PackageProject must refuse when the reference project cannot be built at the requested version")
	}
	// The message has to name both versions and the fix, or the user cannot act.
	for _, want := range []string{absent, "setup mxbuild"} {
		if !contains(err.Error(), want) {
			t.Errorf("error should mention %q so the user can act on it; got:\n%s", want, err)
		}
	}
}

// TestPackageProject_BuildsAReferenceProject exercises the happy path end to end
// against a real .mpk and a real mx, and then snapshots the imported module —
// proving slice 2 hands slice 1 something it can actually read.
//
// It needs a package to import. MXCLI_TEST_MPK points at one; without it the
// test skips rather than reaching for the network, so the suite stays hermetic
// by default.
func TestPackageProject_BuildsAReferenceProject(t *testing.T) {
	mpk := os.Getenv("MXCLI_TEST_MPK")
	if mpk == "" {
		t.Skip("set MXCLI_TEST_MPK to a downloaded .mpk to run this")
	}
	version := os.Getenv("MXCLI_TEST_MPK_MENDIX")
	if version == "" {
		t.Skip("set MXCLI_TEST_MPK_MENDIX to the Mendix version the .mpk should be imported at")
	}
	if !mxVersionAvailable(version) {
		t.Skipf("mxbuild %s is not cached; run 'mxcli setup mxbuild --version %s'", version, version)
	}
	if _, err := os.Stat(mpk); err != nil {
		t.Skipf("MXCLI_TEST_MPK does not exist: %v", err)
	}

	mprPath, err := PackageProject(context.Background(), mpk, version, shortTemp(t), testBackend)
	if err != nil {
		t.Fatalf("PackageProject: %v", err)
	}
	if _, err := os.Stat(mprPath); err != nil {
		t.Fatalf("reference project not on disk: %v", err)
	}
	t.Logf("reference project: %s", filepath.Base(mprPath))

	module := os.Getenv("MXCLI_TEST_MPK_MODULE")
	if module == "" {
		return
	}
	snap, err := SnapshotModule(mprPath, module, testBackend)
	if err != nil {
		t.Fatalf("snapshot the imported module: %v", err)
	}
	if len(snap.Elements) == 0 {
		t.Fatalf("imported module %q described no elements — the import produced nothing to compare", module)
	}
	t.Logf("%s in the reference project: %d elements", module, len(snap.Elements))
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) &&
		(haystack == needle || indexOf(haystack, needle) >= 0))
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// TestPackageProject_ReferenceIsReproducibleAndDiffable is the end-to-end proof
// of the whole Phase 1 read path against real marketplace content.
//
// Two reference projects are built independently from the *same* .mpk. They must
// compare clean: if building the baseline were not reproducible, every diff
// against it would carry noise the user cannot distinguish from their own edits.
// Then one side is edited, and the differ must report exactly that element.
//
// Measured with Administration 4.3.2 (content 23513) at Mendix 11.12.1: 21
// elements captured, 21 unchanged between the two references, and a single added
// attribute reported as `ENTITY Account` modified.
func TestPackageProject_ReferenceIsReproducibleAndDiffable(t *testing.T) {
	mpk := os.Getenv("MXCLI_TEST_MPK")
	version := os.Getenv("MXCLI_TEST_MPK_MENDIX")
	module := os.Getenv("MXCLI_TEST_MPK_MODULE")
	if mpk == "" || version == "" || module == "" {
		t.Skip("set MXCLI_TEST_MPK, MXCLI_TEST_MPK_MENDIX and MXCLI_TEST_MPK_MODULE to run this")
	}
	if !mxVersionAvailable(version) {
		t.Skipf("mxbuild %s is not cached; run 'mxcli setup mxbuild --version %s'", version, version)
	}

	ctx := context.Background()
	refA, err := PackageProject(ctx, mpk, version, shortTemp(t), testBackend)
	if err != nil {
		t.Fatalf("reference A: %v", err)
	}
	refB, err := PackageProject(ctx, mpk, version, shortTemp(t), testBackend)
	if err != nil {
		t.Fatalf("reference B: %v", err)
	}

	snapA, err := SnapshotModule(refA, module, testBackend)
	if err != nil {
		t.Fatalf("snapshot A: %v", err)
	}
	snapB, err := SnapshotModule(refB, module, testBackend)
	if err != nil {
		t.Fatalf("snapshot B: %v", err)
	}
	if len(snapA.Elements) == 0 {
		t.Fatalf("the imported module described no elements — nothing to compare")
	}

	if rep := Compare(snapA, snapB); !rep.Clean() {
		for _, f := range rep.Findings {
			if f.Verdict != Unchanged {
				t.Errorf("%s: %s %s", f.Key, f.Verdict, f.Reason)
			}
		}
		t.Fatal("two references built from the same package must compare clean")
	}
	t.Logf("%s: %d elements, reproducible", module, len(snapA.Elements))

	// A real local edit must be reported, and nothing else with it.
	execMDL(t, refA, "alter entity "+module+".Account add attribute DiffProbe: String(50);")
	edited, err := SnapshotModule(refA, module, testBackend)
	if err != nil {
		t.Fatalf("snapshot after edit: %v", err)
	}

	var changed []string
	for _, f := range Compare(edited, snapB).Findings {
		if f.Verdict != Unchanged {
			changed = append(changed, f.Key.String()+"="+string(f.Verdict))
		}
	}
	if len(changed) != 1 || changed[0] != "ENTITY Account=modified" {
		t.Errorf("expected exactly ENTITY Account=modified, got %v", changed)
	}
}
