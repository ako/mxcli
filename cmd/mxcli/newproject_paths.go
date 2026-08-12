// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MxToolset refuses to extract a file whose full destination path exceeds this,
// regardless of what the filesystem allows — it is a Windows-compatibility limit
// carried by the tool, not by the OS. Exceeding it aborts extraction PART WAY
// THROUGH, so the output directory is left holding a few hundred files and no
// .mpr.
//
// Measured on mxbuild 11.13.0 by bisecting the output-directory length: a
// 77-character absolute directory creates the project; 78 fails with
//
//	System.IO.PathTooLongException: The specified file name or path is too long
//
// leaving 259 files and no .mpr — matching what issue #825 reported from 11.12.
// The blank template's longest relative path was 181 characters there
// (…/RNCAsyncStorage.xcodeproj/xcshareddata/xcschemes/RNCAsyncStorage-macOS.xcscheme),
// and 77 + 1 + 181 = 259 exactly.
const mxToolsetMaxPath = 259

// stagedProjectDirs returns a short staging directory to create the project in,
// plus a cleanup func.
//
// mxcli creates the project in the staging directory and moves it to the
// destination afterwards, rather than pointing `mx create-project` at a deep path
// and hoping. Two things fall out of that:
//
//   - A deep destination WORKS. The limit is MxToolset's own, and a created
//     project embeds no absolute paths (verified: zero files in a blank 11.13
//     project mention their own directory), so relocating it is safe.
//   - The destination is never partially populated. Nothing is written there
//     until creation has already succeeded, so a failure leaves the user's
//     directory exactly as it was instead of with 259 orphaned files.
//
// (issue #825)
func stagedProjectDirs() (stage string, cleanup func(), err error) {
	stage, err = os.MkdirTemp("", "mxcli-new-")
	if err != nil {
		return "", func() {}, fmt.Errorf("create staging directory: %w", err)
	}
	return stage, func() { _ = os.RemoveAll(stage) }, nil
}

// longestRelativePath returns the length of the longest path inside root,
// relative to root, and the path itself.
//
// Measured rather than hardcoded: the template changes between Mendix versions
// (181 characters on 11.13.0, 182 reported on 11.12.0), so a constant would drift
// and quietly mis-report the budget.
func longestRelativePath(root string) (int, string) {
	longest, longestPath := 0, ""
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // a partial walk still yields a usable bound
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if len(rel) > longest {
			longest, longestPath = len(rel), rel
		}
		return nil
	})
	return longest, longestPath
}

// warnIfPathTooLongForStudioPro reports when the finished project sits deep
// enough that MxToolset's own limit would be exceeded.
//
// A warning rather than a refusal: the project has already been created and works
// here — the limit is a Windows-compatibility one, and POSIX allows far longer
// paths — but Studio Pro on Windows would not open it, and a `mx` invocation
// against it can fail the same way mxcli's own creation would have. Saying so is
// more useful than either silently shipping an unportable project or refusing to
// create one that is fine on this machine. (issue #825)
func warnIfPathTooLongForStudioPro(w io.Writer, dest string, longest int, longestPath string) bool {
	total := len(dest) + 1 + longest
	if total <= mxToolsetMaxPath {
		return false
	}
	budget := mxToolsetMaxPath - 1 - longest
	fmt.Fprintf(w, "\nWarning: this project's path is %d characters longer than Mendix tooling allows.\n", total-mxToolsetMaxPath)
	fmt.Fprintf(w, "  Output directory:   %s (%d characters)\n", dest, len(dest))
	fmt.Fprintf(w, "  Longest file below: %s (%d)\n", longestPath, longest)
	fmt.Fprintf(w, "  Total %d exceeds the %d-character limit MxToolset enforces.\n", total, mxToolsetMaxPath)
	fmt.Fprintf(w, "  The project was created and works on this machine, but Studio Pro on Windows\n")
	fmt.Fprintf(w, "  may refuse to open it, and `mx` commands against it can fail with\n")
	fmt.Fprintf(w, "  PathTooLongException. Move it to a directory of at most %d characters.\n", budget)
	return true
}

// moveProject moves a created project from the staging directory into dest.
// Falls back to copy-then-remove when the two are on different filesystems,
// which os.Rename cannot cross.
func moveProject(stage, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create parent directory: %w", err)
	}
	// Rename onto an existing empty directory fails, so clear the placeholder the
	// caller may have created. Only ever an empty directory: `mxcli new` refuses a
	// destination that already has content.
	if entries, err := os.ReadDir(dest); err == nil && len(entries) == 0 {
		_ = os.Remove(dest)
	}
	if err := os.Rename(stage, dest); err == nil {
		return nil
	}
	// Cross-device: copy, then drop the staging copy.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	if err := os.CopyFS(dest, os.DirFS(stage)); err != nil {
		return fmt.Errorf("copy project into place: %w", err)
	}
	_ = os.RemoveAll(stage)
	return nil
}

// describeCreateProjectFailure adds the path-length explanation to a
// create-project failure when the staging path was the plausible cause.
//
// Staging makes this nearly unreachable — the staging root is short — but if
// someone's TMPDIR is itself deep, the same failure returns with a far more
// confusing message, so name the cause rather than surfacing a bare exit status.
func describeCreateProjectFailure(stage string, runErr error) error {
	if len(stage) > mxToolsetMaxPath-1-200 {
		return fmt.Errorf("%w\n  The staging directory (%s, %d characters) may be too deep for Mendix tooling,\n"+
			"  which refuses paths over %d characters. Set TMPDIR to a shorter directory and retry",
			runErr, stage, len(stage), mxToolsetMaxPath)
	}
	return runErr
}
