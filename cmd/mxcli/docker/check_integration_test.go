// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// TestCheck_PreservesMPRv2StorageFormat is the end-to-end guard for
// mendixlabs/mxcli#763. `mx update-widgets`, which Check runs before `mx check`,
// rewrites an MPRv2 project into the self-contained MPRv1 format (inlining every
// unit into the .mpr and deleting mprcontents/). Check must leave the on-disk
// storage format exactly as it found it. Both mx 11.9.0 (the version the CI
// integration job installs) and 11.12.1 were observed to perform this conversion,
// so on a v2 project this test genuinely exercises the bug: without the snapshot/
// restore fix the project would be MPRv1 after Check.
//
// Requires a resolvable mx (provided by the CI integration job's
// `mxcli setup mxbuild`); skips otherwise. Uses `mx create-project`, whose output
// is MPRv2, as the fixture — the same scaffolding the executor roundtrip suite uses.
//
// Since ako/mxcli#646 update-widgets runs before the check only with
// --update-widgets (UpdateWidgetsFirst), which is what this test now passes so
// that it still runs the converting tool.
func TestCheck_PreservesMPRv2StorageFormat(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}

	// Scaffold a fresh project. `mx create-project` (no template arg) writes App.mpr
	// into the working directory and produces MPRv2 storage.
	dir := t.TempDir()
	scaffold := exec.Command(mxPath, "create-project")
	scaffold.Dir = dir
	if out, err := scaffold.CombinedOutput(); err != nil {
		t.Skipf("mx create-project failed, cannot scaffold fixture: %v\n%s", err, out)
	}
	mprPath := filepath.Join(dir, "App.mpr")
	if _, err := os.Stat(mprPath); err != nil {
		t.Skipf("mx create-project did not produce App.mpr: %v", err)
	}

	// Precondition: the fixture must be MPRv2, or the test proves nothing.
	if v := mprStorageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Skipf("scaffolded project is %v, not MPRv2 — nothing to protect", v)
	}

	var stdout, stderr bytes.Buffer
	if err := Check(CheckOptions{
		ProjectPath:        mprPath,
		UpdateWidgetsFirst: true,
		Stdout:             &stdout,
		Stderr:             &stderr,
	}); err != nil {
		t.Fatalf("Check failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	// Postcondition: still MPRv2. Without the fix, update-widgets would have left
	// it MPRv1.
	if v := mprStorageVersion(t, mprPath); v != types.MPRVersionV2 {
		t.Errorf("Check converted the project to %v; the MPRv2 storage format must be preserved (#763)", v)
	}
	if _, err := os.Stat(filepath.Join(dir, "mprcontents")); err != nil {
		t.Errorf("mprcontents/ missing after Check, storage format was not preserved: %v", err)
	}
}

// mprStorageVersion opens the .mpr and returns its detected storage-format version.
func mprStorageVersion(t *testing.T, mprPath string) types.MPRVersion {
	t.Helper()
	reader, err := openReadOnly(mprPath)
	if err != nil {
		t.Fatalf("openReadOnly(%s): %v", mprPath, err)
	}
	defer reader.Disconnect()
	return reader.Version()
}

// TestCheck_LeavesProjectUntouched is the end-to-end guard for ako/mxcli#951:
// with real mx, a default `docker check`, a --no-update-widgets one and an
// --update-widgets one leave every file of an MPRv1 and an MPRv2 project
// byte-identical with the same mtime. Before the fix, update-widgets rewrote a
// v1 .mpr permanently, and mx check wrote theme-cache/ and deployment/sass/ in
// both formats. (The default ran update-widgets until #646; --update-widgets
// is the mode that runs it on a clean project now.)
func TestCheck_LeavesProjectUntouched(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}
	scaffold := func(t *testing.T) string {
		// Not t.TempDir(): the subtest names make it long enough for
		// create-project's template extraction to hit PathTooLongException.
		dir, err := os.MkdirTemp("", "chk")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.RemoveAll(dir) })
		cmd := exec.Command(mxPath, "create-project")
		cmd.Dir = dir
		PrepareMxCommand(cmd)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Skipf("mx create-project failed: %v\n%s", err, out)
		}
		return filepath.Join(dir, "App.mpr")
	}

	modes := []struct {
		name string
		opts CheckOptions
	}{
		{"default", CheckOptions{}},
		{"no-update-widgets", CheckOptions{SkipUpdateWidgets: true}},
		{"update-widgets", CheckOptions{UpdateWidgetsFirst: true}},
	}
	for _, format := range []string{"v2", "v1"} {
		for _, mode := range modes {
			t.Run(fmt.Sprintf("%s/%s", format, mode.name), func(t *testing.T) {
				mprPath := scaffold(t)
				if format == "v1" {
					// update-widgets converts a v2 project to v1 — the fixture we want.
					if err := updateWidgetsCmd(mxPath, mprPath, io.Discard, io.Discard); err != nil {
						t.Skipf("could not produce a v1 fixture: %v", err)
					}
					if v := mprStorageVersion(t, mprPath); v != types.MPRVersionV1 {
						t.Skipf("fixture is %v, not v1", v)
					}
				}
				dir := filepath.Dir(mprPath)
				before := treeState(t, dir)
				var stdout bytes.Buffer
				opts := mode.opts
				opts.ProjectPath, opts.Stdout, opts.Stderr = mprPath, &stdout, &stdout
				if err := Check(opts); err != nil {
					t.Fatalf("Check: %v\n%s", err, stdout.String())
				}
				if d := diffStates(before, treeState(t, dir)); len(d) > 0 {
					t.Errorf("docker check modified the project:\n  %v", d)
				}
			})
		}
	}
}

// TestMxCheckOnCopy_LeavesProjectUntouched: the plain `mx check` the TUI
// checker and the eval runner run (ako/mxcli#961) leaves an MPRv1 and an MPRv2
// project byte-identical with the same mtimes, and still reports what mx found.
// Measured before the fix with mx 11.14: theme-cache/web/ and deployment/sass/
// were added to the project in both formats.
func TestMxCheckOnCopy_LeavesProjectUntouched(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}
	for _, format := range []string{"v2", "v1"} {
		t.Run(format, func(t *testing.T) {
			dir, err := os.MkdirTemp("", "mxc")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(dir) })
			cmd := exec.Command(mxPath, "create-project")
			cmd.Dir = dir
			PrepareMxCommand(cmd)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Skipf("mx create-project failed: %v\n%s", err, out)
			}
			mprPath := filepath.Join(dir, "App.mpr")
			if format == "v1" {
				if err := updateWidgetsCmd(mxPath, mprPath, io.Discard, io.Discard); err != nil {
					t.Skipf("could not produce a v1 fixture: %v", err)
				}
			}
			jsonPath := filepath.Join(t.TempDir(), "check.json")
			before := treeState(t, dir)
			var out bytes.Buffer
			if err := MxCheckOnCopy(mxPath, mprPath, []string{"-j", jsonPath}, &out, &out); err != nil {
				t.Fatalf("MxCheckOnCopy: %v\n%s", err, out.String())
			}
			if d := diffStates(before, treeState(t, dir)); len(d) > 0 {
				t.Errorf("mx check modified the project:\n  %v", d)
			}
			if _, err := os.Stat(jsonPath); err != nil {
				t.Errorf("mx check wrote no JSON result: %v", err)
			}
		})
	}
}
