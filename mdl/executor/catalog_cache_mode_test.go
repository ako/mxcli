// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The catalog cache records the mode it was built in, and the modes nest:
// source ⊃ full ⊃ fast. A consumer that only needs fast-mode answers — `mxcli
// check`, `show structure`, `describe` — must not be able to replace a richer
// cache with its own narrow one. That is mendixlabs/mxcli#1081: after the
// project file changes, a `check` rebuilt fast and saved over a source-mode
// cache, emptying source, refs, permissions, strings and xpath_expressions in
// one go. The commands that read those tables then report "requires refresh
// catalog full source" instead of an answer.
//
// A real project rather than a mock, for the same reason typecheck_test gives:
// the thing under test is a file on disk written from a model on disk.

// sourceCachedFixture connects an executor to a copy of the shared fixture and
// builds a source-mode catalog, returning the executor and the cache path.
func sourceCachedFixture(t *testing.T) (*Executor, string) {
	t.Helper()
	exec := typeCheckFixture(t)
	run(t, exec, "REFRESH CATALOG FULL SOURCE FORCE")

	ctx := exec.newExecContext(context.Background())
	cachePath := getCachePath(ctx)
	if cachePath == "" {
		t.Fatal("no cache path — the fixture is not connected to a project on disk")
	}
	if mode := cachedMode(t, cachePath); mode != "source" {
		t.Fatalf("fixture setup: cache mode = %q, want %q", mode, "source")
	}
	if n := cachedRowCount(t, cachePath, "source"); n == 0 {
		t.Fatal("fixture setup: source table is empty, so its loss would prove nothing")
	}
	return exec, cachePath
}

// cachedMode reports the build mode recorded in the cache file.
func cachedMode(t *testing.T, cachePath string) string {
	t.Helper()
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		t.Fatalf("open cache %s: %v", cachePath, err)
	}
	defer cat.Close()
	info, err := cat.GetCacheInfo()
	if err != nil {
		t.Fatalf("read cache info: %v", err)
	}
	return info.BuildMode
}

// cachedRowCount counts the rows of one table inside the cache file.
func cachedRowCount(t *testing.T, cachePath, table string) int {
	t.Helper()
	cat, err := catalog.NewFromFile(cachePath)
	if err != nil {
		t.Fatalf("open cache %s: %v", cachePath, err)
	}
	defer cat.Close()
	res, err := cat.Query("select count(*) from " + table)
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	if len(res.Rows) == 0 {
		return 0
	}
	var n int
	fmt.Sscanf(fmt.Sprintf("%v", res.Rows[0][0]), "%d", &n)
	return n
}

// touchProject moves the project file's mtime forward, which is what makes the
// cache invalid — the everyday trigger being a save in Studio Pro, an `mxcli
// exec`, or a branch switch between two mxcli commands.
func touchProject(t *testing.T, exec *Executor) {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(ctx.MprPath, future, future); err != nil {
		t.Fatalf("touch project: %v", err)
	}
}

// TestCheckDoesNotDowngradeASourceCache is #1081 itself, entered through the
// tier `mxcli check -p` actually reaches. TypeCheckProgram asks for a fast
// catalog; with the cache stale it rebuilds, and the rebuild must not become
// the new cache.
func TestCheckDoesNotDowngradeASourceCache(t *testing.T) {
	exec, cachePath := sourceCachedFixture(t)
	before := cachedRowCount(t, cachePath, "source")

	touchProject(t, exec)

	prog, errs := visitor.Build(`CREATE MICROFLOW MyFirstModule.Probe() RETURNS BOOLEAN BEGIN RETURN true; END`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	exec.TypeCheckProgram(prog)

	if mode := cachedMode(t, cachePath); mode != "source" {
		t.Errorf("after check, cache mode = %q, want %q — a fast rebuild overwrote the source cache", mode, "source")
	}
	if n := cachedRowCount(t, cachePath, "source"); n != before {
		t.Errorf("after check, source rows = %d, want %d — the source index was destroyed", n, before)
	}
}

// TestFastConsumersDoNotDowngradeASourceCache covers the rest of the fast-mode
// callers through their shared seam. `check` is where #1081 was reported, but
// `show structure`, `describe` and `show catalog tables` all call ensureCatalog
// the same way, so fixing this at the check command would have left the bug in
// place for them.
func TestFastConsumersDoNotDowngradeASourceCache(t *testing.T) {
	exec, cachePath := sourceCachedFixture(t)
	before := cachedRowCount(t, cachePath, "source")

	touchProject(t, exec)

	ctx := exec.newExecContext(context.Background())
	if err := ensureCatalog(ctx, false); err != nil {
		t.Fatalf("ensureCatalog(fast): %v", err)
	}

	if mode := cachedMode(t, cachePath); mode != "source" {
		t.Errorf("after a fast rebuild, cache mode = %q, want %q", mode, "source")
	}
	if n := cachedRowCount(t, cachePath, "source"); n != before {
		t.Errorf("after a fast rebuild, source rows = %d, want %d", n, before)
	}
}

// TestFullRebuildRestoresTheCachedSourceMode is the other half, and the reason
// "leave the cache alone" is not the whole fix. A consumer that needs full mode
// (search, show references, lint's graph rules) has to rebuild anyway; it
// rebuilds at the level the project is set up for, so the cache comes back
// current at source rather than thrashing on every invocation.
func TestFullRebuildRestoresTheCachedSourceMode(t *testing.T) {
	exec, cachePath := sourceCachedFixture(t)
	before := cachedRowCount(t, cachePath, "source")

	touchProject(t, exec)

	ctx := exec.newExecContext(context.Background())
	if err := ensureCatalog(ctx, true); err != nil {
		t.Fatalf("ensureCatalog(full): %v", err)
	}

	if mode := cachedMode(t, cachePath); mode != "source" {
		t.Errorf("after a full rebuild, cache mode = %q, want %q — the source level was not restored", mode, "source")
	}
	if n := cachedRowCount(t, cachePath, "source"); n != before {
		t.Errorf("after a full rebuild, source rows = %d, want %d", n, before)
	}
	// And the cache must be current again, or the next consumer rebuilds too.
	if valid, reason := isCacheValid(ctx, cachePath, "source"); !valid {
		t.Errorf("cache still invalid after a full rebuild: %s", reason)
	}
}

// TestPlainRefreshCatalogKeepsTheSourceMode covers the explicit statement.
// REFRESH CATALOG means "bring what this project has up to date", not "change
// its level" — there is no syntax for lowering the level, so treating a bare
// REFRESH CATALOG as a request to drop the source index is a silent loss.
func TestPlainRefreshCatalogKeepsTheSourceMode(t *testing.T) {
	exec, cachePath := sourceCachedFixture(t)
	before := cachedRowCount(t, cachePath, "source")

	touchProject(t, exec)
	run(t, exec, "REFRESH CATALOG")

	if mode := cachedMode(t, cachePath); mode != "source" {
		t.Errorf("after REFRESH CATALOG, cache mode = %q, want %q", mode, "source")
	}
	if n := cachedRowCount(t, cachePath, "source"); n != before {
		t.Errorf("after REFRESH CATALOG, source rows = %d, want %d", n, before)
	}
	// And it has to leave a *current* cache. The never-narrow guard alone would
	// keep the level by declining to save at all, which turns an explicit
	// refresh into a no-op on disk — the cache stays source-mode and stale.
	ctx := exec.newExecContext(context.Background())
	if valid, reason := isCacheValid(ctx, cachePath, "source"); !valid {
		t.Errorf("cache still invalid after REFRESH CATALOG: %s", reason)
	}
}

// TestFirstBuildStillWritesItsOwnMode is the control on the guard: with no
// cache to protect, a fast build must still be cached as fast. A guard that
// simply stopped persisting fast catalogs would pass every test above and make
// every fast consumer rebuild from scratch forever.
func TestFirstBuildStillWritesItsOwnMode(t *testing.T) {
	exec := typeCheckFixture(t)
	ctx := exec.newExecContext(context.Background())
	cachePath := getCachePath(ctx)
	if err := os.RemoveAll(filepath.Dir(cachePath)); err != nil {
		t.Fatalf("clear cache dir: %v", err)
	}

	if err := ensureCatalog(ctx, false); err != nil {
		t.Fatalf("ensureCatalog(fast): %v", err)
	}
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("no cache written on a first fast build: %v", err)
	}
	if mode := cachedMode(t, cachePath); mode != "fast" {
		t.Errorf("first build cache mode = %q, want %q", mode, "fast")
	}
}
