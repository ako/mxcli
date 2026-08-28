// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance-2: after roughly ten hot-applies under --watch, a page
// failed with the runtime's generic error dialog and the log filled with
//
//	404 - file not found for file: dist%2Fchunks%2F<hash>.js
//
// The model was clean, `mx check` was clean, and a plain restart fixed it — so
// several minutes went into debugging the wrong layer.
package docker

import (
	"os"
	"path/filepath"
	"testing"
)

// bundle writes a web/dist tree: entry is index.js's content, chunks are the
// chunk files that exist on disk (name -> content).
func bundle(t *testing.T, entry string, chunks map[string]string) string {
	t.Helper()
	deploy := t.TempDir()
	dist := filepath.Join(deploy, "web", "dist", "chunks")
	if err := os.MkdirAll(dist, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(deploy, "web", "dist", "index.js"), []byte(entry), 0o600); err != nil {
		t.Fatalf("write index.js: %v", err)
	}
	for name, content := range chunks {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return deploy
}

func TestDanglingChunkIsDetected(t *testing.T) {
	// The reported failure: index.js is present and served, and imports a chunk
	// that is not there. The old index.js probe returns 200 and reports success.
	deploy := bundle(t,
		`import{a}from"./chunks/AAAA1111.js";import{b}from"./chunks/BBBB2222.js";`,
		map[string]string{"AAAA1111.js": "export const a=1;"})

	got := danglingClientChunks(deploy)
	if len(got) != 1 || got[0] != "BBBB2222.js" {
		t.Fatalf("got %v, want [BBBB2222.js]", got)
	}
}

func TestHealthyBundleIsNotReported(t *testing.T) {
	// The control that decides whether this is safe to act on: it gates a
	// re-bundle, so a false positive rebuilds a working app on every apply.
	deploy := bundle(t,
		`import{a}from"./chunks/AAAA1111.js";`,
		map[string]string{
			"AAAA1111.js": `import{c}from"./chunks/CCCC3333.js";export const a=1;`,
			"CCCC3333.js": "export const c=3;",
		})
	if got := danglingClientChunks(deploy); len(got) != 0 {
		t.Errorf("healthy bundle reported as dangling: %v", got)
	}
}

func TestTransitiveChunkIsFollowed(t *testing.T) {
	// index.js imports one chunk directly and the rest of the graph hangs off it —
	// which is what a real rollup bundle looks like, so a one-level check would
	// miss almost every real breakage.
	deploy := bundle(t,
		`import{a}from"./chunks/AAAA1111.js";`,
		map[string]string{"AAAA1111.js": `import{d}from"./chunks/DDDD4444.js";`})
	got := danglingClientChunks(deploy)
	if len(got) != 1 || got[0] != "DDDD4444.js" {
		t.Fatalf("got %v, want [DDDD4444.js] via the transitive import", got)
	}
}

func TestOrphanedChunksAreNotFollowed(t *testing.T) {
	// A chunk left behind by an earlier bundle can reference other old hashes.
	// Walking from index.js only — and following just the chunks that exist —
	// keeps those out, or every re-bundle would look broken.
	deploy := bundle(t,
		`import{a}from"./chunks/AAAA1111.js";`,
		map[string]string{
			"AAAA1111.js": "export const a=1;",
			"OLD9999.js":  `import{z}from"./chunks/GONE0000.js";`, // orphan from a previous build
		})
	if got := danglingClientChunks(deploy); len(got) != 0 {
		t.Errorf("orphaned chunk dragged in: %v", got)
	}
}

func TestNoBundleIsNotADanglingChunk(t *testing.T) {
	// "No bundle at all" is clientBundlePresent's job. Reporting it here would
	// send a missing-bundle case down the wrong recovery message.
	if got := danglingClientChunks(t.TempDir()); len(got) != 0 {
		t.Errorf("missing bundle reported as dangling: %v", got)
	}
}
