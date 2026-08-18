// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeUnits simulates what an `mxcli exec` does to the model on disk: many
// files rewritten over a stretch of time, not one atomic change.
func writeUnits(t *testing.T, dir string, n int, gap time.Duration) {
	t.Helper()
	for i := 0; i < n; i++ {
		path := filepath.Join(dir, "unit"+string(rune('a'+i))+".mxunit")
		if err := os.WriteFile(path, []byte{byte(i)}, 0o644); err != nil {
			t.Error(err)
			return
		}
		time.Sleep(gap)
	}
}

// TestSettleSourceWaitsForAMultiFileWrite is the regression test for the stale
// build. The watcher used to rebuild on the first mtime bump, so a multi-second
// exec was deployed half-applied — and once exec became byte-idempotent,
// re-running it wrote nothing and there was no way to re-trigger a build.
func TestSettleSourceWaitsForAMultiFileWrite(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	contents := filepath.Join(dir, "mprcontents")
	if err := os.MkdirAll(contents, 0o755); err != nil {
		t.Fatal(err)
	}

	// The gap between unit writes is well inside one poll, which is what an exec
	// actually does — it rewrites units as fast as the disk takes them, over a
	// span of seconds. A writer that paused longer than the settle window between
	// files would be indistinguishable from a finished one, and no timeout-based
	// watcher can tell those apart.
	const poll = 20 * time.Millisecond
	var done atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeUnits(t, contents, 12, poll/4)
		done.Store(true)
	}()

	settled := settleSource(mpr, sourceMTime(mpr), poll, nil)
	// Read the writer's state at the moment settleSource returned — after
	// wg.Wait() every unit is on disk regardless, so that assertion would hold
	// against a settleSource that returned immediately.
	finishedFirst := done.Load()
	wg.Wait()

	if settled.IsZero() {
		t.Fatal("settleSource reported an interrupt that never happened")
	}
	if !finishedFirst {
		t.Error("settleSource returned while the model was still being written — the build would be half-applied")
	}
	if final := sourceMTime(mpr); settled.Before(final) {
		t.Errorf("settled at %v but the source is newer (%v)", settled, final)
	}
}

// TestSettleSourceReturnsPromptlyForOneChange guards the other direction: a
// single editor save must not pay a long wait.
func TestSettleSourceReturnsPromptlyForOneChange(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	const poll = 20 * time.Millisecond
	start := time.Now()
	settled := settleSource(mpr, sourceMTime(mpr), poll, nil)
	elapsed := time.Since(start)

	if settled.IsZero() {
		t.Fatal("settleSource reported an interrupt that never happened")
	}
	if max := poll * (sourceSettleWindow + 3); elapsed > max {
		t.Errorf("a quiet source took %v to settle, want under %v", elapsed, max)
	}
}

// TestSettleSourceHonoursInterrupt — Ctrl-C during a long exec must still stop
// the loop rather than wait the write out.
func TestSettleSourceHonoursInterrupt(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sigCh := make(chan os.Signal, 1)
	sigCh <- os.Interrupt

	if settled := settleSource(mpr, sourceMTime(mpr), 20*time.Millisecond, sigCh); !settled.IsZero() {
		t.Errorf("settleSource ignored the interrupt, returned %v", settled)
	}
}
