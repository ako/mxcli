// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
)

// preserveWebClientBundle copies the deployment's browser bundle aside and
// returns a function that puts it back if the boot destroyed it.
//
// The boot's Gradle packaging pass repopulates deployment/web and takes dist/
// with it. A headless boot does not need the bundle and does not re-create one,
// so a `mxcli test --local` run between a `mxcli run --local` and a browser left
// the running app serving Mendix's SPA shell over a 404 for /dist/index.js:
// HTTP 200, blank page, nothing reported at either end (mxcli-formula1 §62).
//
// Giving the test boot its own deployment tree would have made the collision
// impossible, and is not available: mxbuild writes the deployment to
// `<app dir>/deployment` and has no flag to move it, so the runtime booted
// against an empty directory instead (mxcli-ledger §150). The deployment
// directory is shared because mxbuild shares it.
//
// So the bundle is carried across the boot rather than protected from it. That
// costs a copy of a few MB — against the ~30s re-bundle that made warning the
// earlier choice — and returns the exact bundle the dev loop built, not a
// rebuilt approximation of it.
//
// The restore is deliberately conservative: it writes only when the bundle is
// gone. A boot that left one alone, or built a newer one, keeps what is there —
// restoring a snapshot over a fresher bundle would be the same defect pointing
// the other way.
func preserveWebClientBundle(deployDir string) func() {
	if !WebClientBundled(deployDir) {
		return func() {} // nothing to lose
	}
	dist := filepath.Join(deployDir, "web", "dist")
	snapshot, err := os.MkdirTemp("", "mxcli-webclient-")
	if err != nil {
		return func() {}
	}
	saved := filepath.Join(snapshot, "dist")
	if err := copyTree(dist, saved); err != nil {
		os.RemoveAll(snapshot)
		return func() {}
	}
	return func() {
		defer os.RemoveAll(snapshot)
		if WebClientBundled(deployDir) {
			return // the boot did not take it
		}
		if err := os.MkdirAll(filepath.Dir(dist), 0o755); err != nil {
			return
		}
		_ = copyTree(saved, dist)
	}
}

// copyTree copies a directory recursively. Symlinks are not followed — a
// deployment's bundle is plain files, and following one would copy out of the
// tree being snapshotted.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.IsDir():
			return os.MkdirAll(target, 0o755)
		case !info.Mode().IsRegular():
			return nil
		default:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return copyFile(path, target) // check.go — preserves the source mode
		}
	})
}
