// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"testing"
)

// mxcli-formula1 FINDINGS §62, re-fixed within what mxbuild actually allows.
//
// A headless boot's Gradle packaging pass deletes deployment/web/dist, so a
// `mxcli test --local` run between a `mxcli run --local` and a browser left the
// running app serving Mendix's SPA shell over a 404 for /dist/index.js: HTTP
// 200, blank page, nothing reported at either end.
//
// The first attempt gave the test boot its own deployment tree. That cannot
// work — mxbuild writes the deployment to `<app dir>/deployment` and has no flag
// to move it (see localapp_deploydir_test.go) — so the runtime booted against an
// empty directory and `mxcli test --local` stopped working entirely
// (mxcli-ledger §150).
//
// The bundle is a few MB of files that the boot destroys and does not need. So
// it is copied aside before the boot and put back after, which costs a file copy
// instead of the ~30s re-bundle that made warning-instead-of-fixing the earlier
// choice. The dev loop gets back the exact bundle it built.

// writeBundle is in webclient_bundle_test.go — the same fixture, since this is
// the same bundle by the same route.

func TestPreserveWebClientBundle_PutsItBack(t *testing.T) {
	deploy := t.TempDir()
	writeBundle(t, deploy, "console.log('the dev loop built this')")

	restore := preserveWebClientBundle(deploy)

	// What the boot's packaging step does.
	if err := os.RemoveAll(filepath.Join(deploy, "web")); err != nil {
		t.Fatal(err)
	}
	if WebClientBundled(deploy) {
		t.Fatal("the fixture did not reproduce the loss")
	}

	restore()

	if !WebClientBundled(deploy) {
		t.Fatal("the bundle was not restored — the running app still serves a blank page")
	}
	got, err := os.ReadFile(filepath.Join(deploy, "web", "dist", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	// Byte-identical, not merely present: a placeholder would render the same
	// blank page while passing an existence check.
	if string(got) != "console.log('the dev loop built this')" {
		t.Errorf("restored content = %q, want the bundle the dev loop built", got)
	}
}

// Sibling files matter as much as index.js — the bundle is a directory of
// chunks, and a restore that brought back only the entry point would serve a
// 404 for everything it imports.
func TestPreserveWebClientBundle_RestoresTheWholeDirectory(t *testing.T) {
	deploy := t.TempDir()
	writeBundle(t, deploy, "entry")
	dist := filepath.Join(deploy, "web", "dist")
	if err := os.MkdirAll(filepath.Join(dist, "chunks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "chunks", "a.js"), []byte("chunk"), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := preserveWebClientBundle(deploy)
	if err := os.RemoveAll(filepath.Join(deploy, "web")); err != nil {
		t.Fatal(err)
	}
	restore()

	if got, err := os.ReadFile(filepath.Join(dist, "chunks", "a.js")); err != nil || string(got) != "chunk" {
		t.Errorf("nested bundle file not restored: %v %q", err, got)
	}
}

// CONTROL 1: a boot that did NOT destroy the bundle must be left exactly as it
// is. Restoring a stale copy over a newer bundle would be its own bug — the
// warm loop re-bundles on a page edit, and this must not undo that.
func TestPreserveWebClientBundle_LeavesASurvivingBundleAlone(t *testing.T) {
	deploy := t.TempDir()
	writeBundle(t, deploy, "old")

	restore := preserveWebClientBundle(deploy)
	writeBundle(t, deploy, "newer, built during the boot")
	restore()

	got, err := os.ReadFile(filepath.Join(deploy, "web", "dist", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "newer, built during the boot" {
		t.Errorf("a surviving bundle was overwritten with the snapshot: %q", got)
	}
}

// CONTROL 2: with no bundle to begin with — a fresh project, or a tree only ever
// used headlessly — there is nothing to snapshot and nothing to restore. The
// restore must not create an empty web/dist that then reads as "bundled".
func TestPreserveWebClientBundle_NoBundleIsANoOp(t *testing.T) {
	deploy := t.TempDir()

	restore := preserveWebClientBundle(deploy)
	restore()

	if _, err := os.Stat(filepath.Join(deploy, "web")); !os.IsNotExist(err) {
		t.Errorf("restore invented a web/ directory: %v", err)
	}
	if WebClientBundled(deploy) {
		t.Error("an empty deployment now reports a bundle")
	}
}
