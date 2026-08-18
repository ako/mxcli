// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// projectSnapshot is the project file exactly as it was before anything was
// injected, plus a digest of the document tree beside it.
//
// A test run injects a module, builds, runs, and takes the injection back out.
// The taking-out is very nearly perfect: `mprcontents/` comes back
// byte-identical. The `.mpr` does not, and it is not the model's fault — every
// unit write stamps a fresh UUID into the `_Transaction` bookkeeping row, and
// the insert/delete cycle leaves SQLite's pages laid out differently even once
// the logical content matches again.
//
// Restoring the row alone therefore is not enough: measured over three
// consecutive runs it makes the id stable and leaves the file hash different
// every time. Version control compares bytes, so a run that changed nothing
// still shows as a modification — which breaks any "run the tests, then assert
// the tree is clean" CI step, puts a meaningless diff in a pull request, and
// costs whoever sees it an afternoon, because a `.mpr` diff is opaque and there
// is no cheap way to tell a bookkeeping GUID from a real model edit.
//
// So the whole file is put back. That is byte-exact by construction rather than
// by enumerating what might have changed.
type projectSnapshot struct {
	path     string
	contents []byte
	// treeDigest covers the mprcontents/ document tree (empty for MPR v1, which
	// keeps everything in the .mpr). It is the safety interlock: the .mpr indexes
	// those documents, so putting an old .mpr back over a tree that has moved on
	// would leave the index pointing at documents that are no longer there.
	treeDigest string
}

// takeProjectSnapshot reads the project file and digests the document tree.
//
// A failure is not fatal to the run — the caller keeps the zero value and simply
// does not restore, which is the behaviour before this existed. Refusing to run
// tests over a cosmetic diff would be the wrong trade.
func takeProjectSnapshot(projectPath string) projectSnapshot {
	data, err := os.ReadFile(projectPath)
	if err != nil {
		return projectSnapshot{}
	}
	digest, err := documentTreeDigest(projectPath)
	if err != nil {
		return projectSnapshot{}
	}
	return projectSnapshot{path: projectPath, contents: data, treeDigest: digest}
}

// restore puts the project file back, and reports what it did for the caller to
// log.
//
// It refuses in two cases, both of which mean the project is not in the state
// the snapshot describes: cleanup failed, or the document tree has changed.
// Writing the old .mpr in either case would replace a visible, harmless
// discrepancy with an invisible, misleading one — the tree would read as clean
// while the model was not.
func (s projectSnapshot) restore(cleanupOK bool) error {
	if s.path == "" || !cleanupOK {
		return nil
	}
	digest, err := documentTreeDigest(s.path)
	if err != nil {
		return fmt.Errorf("re-reading the document tree: %w", err)
	}
	if digest != s.treeDigest {
		return fmt.Errorf("the document tree changed during the run, so the original %s was left in place",
			filepath.Base(s.path))
	}

	current, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("re-reading %s: %w", filepath.Base(s.path), err)
	}
	if string(current) == string(s.contents) {
		return nil
	}
	// Write through a temp file in the same directory so an interrupted restore
	// cannot leave a truncated .mpr — that would be a far worse outcome than the
	// cosmetic diff this is fixing.
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".mxcli-restore-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(s.contents); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if info, err := os.Stat(s.path); err == nil {
		_ = os.Chmod(tmpName, info.Mode())
	}
	return os.Rename(tmpName, s.path)
}

// documentTreeDigest hashes the mprcontents/ tree — every file's path and
// content — so a change anywhere in it is detectable. MPR v1 has no such
// directory and digests to a constant.
func documentTreeDigest(projectPath string) (string, error) {
	dir := filepath.Join(filepath.Dir(projectPath), "mprcontents")
	if _, err := os.Stat(dir); err != nil {
		return "", nil
	}

	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)

	sum := sha256.New()
	for _, p := range paths {
		rel, _ := filepath.Rel(dir, p)
		fmt.Fprintf(sum, "%s\n", filepath.ToSlash(rel))
		f, err := os.Open(p)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(sum, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
