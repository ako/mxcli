// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Rollup emits the entry point as web/dist/index.js and everything it imports as
// content-hashed files under web/dist/chunks/. A reference looks like
// `./chunks/2j63bV6j.js` or `chunks/2j63bV6j.js` in the importing file.
var chunkRefRe = regexp.MustCompile(`chunks/([A-Za-z0-9._-]+\.js)`)

// maxChunkScanBytes bounds how much of a bundle file is scanned for imports.
// Rollup writes its import statements at the top, and a bundle can be megabytes,
// so reading the whole graph would cost more than the check is worth.
const maxChunkScanBytes = 512 * 1024

// danglingClientChunks returns the chunk files the browser bundle IMPORTS but
// which are not on disk, walking the import graph from index.js.
//
// This is the failure mode ensureClientServed could not see. It probes
// /dist/index.js only, so when the entry point is served but a chunk it imports
// is missing, the probe returns 200, mxcli reports "applied", and the page dies
// in the browser with the runtime's generic error dialog while the log fills
// with
//
//	404 - file not found for file: dist%2Fchunks%2F<hash>.js
//
// The model is clean, `mx check` is clean, and a plain restart fixes it — so it
// reads as an application bug and is debugged as one. Reported after roughly ten
// hot-applies under --watch (ako/mxcli-maintenance-2).
//
// An EMPTY result means "nothing dangling", including for a bundle that has no
// chunks at all. A file that cannot be read is skipped rather than reported: this
// gates a recovery, and a false positive would re-bundle a healthy app.
func danglingClientChunks(deployDir string) []string {
	distDir := filepath.Join(deployDir, "web", "dist")
	entry := filepath.Join(distDir, "index.js")
	if _, err := os.Stat(entry); err != nil {
		return nil // no bundle at all is clientBundlePresent's business, not ours
	}

	missing := map[string]bool{}
	seen := map[string]bool{}
	queue := []string{entry}

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]

		for _, name := range chunkRefsIn(path) {
			if seen[name] {
				continue
			}
			seen[name] = true
			chunk := filepath.Join(distDir, "chunks", name)
			if _, err := os.Stat(chunk); err != nil {
				missing[name] = true
				continue
			}
			// Only follow chunks that exist — an orphaned chunk left behind by an
			// earlier bundle can reference other old hashes, and walking those would
			// report a healthy bundle as broken.
			queue = append(queue, chunk)
		}
	}

	out := make([]string, 0, len(missing))
	for name := range missing {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// chunkRefsIn returns the chunk file names a bundle file imports.
func chunkRefsIn(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, maxChunkScanBytes)
	n, _ := f.Read(buf)
	if n <= 0 {
		return nil
	}
	var out []string
	for _, m := range chunkRefRe.FindAllSubmatch(buf[:n], -1) {
		out = append(out, string(m[1]))
	}
	return out
}
