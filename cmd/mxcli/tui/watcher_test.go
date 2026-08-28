package tui

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// mockSender captures MprChangedMsg sends for testing.
type mockSender struct {
	count atomic.Int32
}

func (m *mockSender) Send(msg tea.Msg) {
	if _, ok := msg.(MprChangedMsg); ok {
		m.count.Add(1)
	}
}

func TestWatcherDebounce(t *testing.T) {
	// A burst of writes must collapse into ONE message.
	//
	// The old version wrote five files, slept 700ms and asserted the count was
	// exactly 1. It failed on CI (2026-08-28) with **got 2**: the gaps between
	// writes outran the production 500ms window, so an intermediate timer fired
	// and the burst was debounced as two. The old comment named that hazard
	// ("keep the burst tighter than the debounce window so slow CI machines do
	// not accidentally let an intermediate timer fire") without doing anything
	// about it — the burst's speed was assumed, never checked.
	//
	// Three changes, each aimed at that failure:
	//
	//  1. a window far longer than the production one, so an ordinary burst has
	//     orders of magnitude of headroom rather than a factor of two;
	//  2. the premise is MEASURED — if the machine was too slow to emit a burst
	//     inside the window, this test's precondition did not hold and it says
	//     so instead of reporting a product defect;
	//  3. the message is polled for, not slept for, so event-delivery latency
	//     cannot decide the outcome either.
	const debounce = 2 * time.Second

	dir := t.TempDir()
	unitFile := filepath.Join(dir, "test.mxunit")
	if err := os.WriteFile(unitFile, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}
	w, err := newWatcherDebounced("", dir, sender, debounce)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	burstStart := time.Now()
	for i := range 5 {
		_ = os.WriteFile(unitFile, []byte{byte('a' + i)}, 0644)
	}
	burst := time.Since(burstStart)

	// The precondition. A burst that does not fit inside the window cannot be
	// debounced into one message by any implementation, so a failure here would
	// say nothing about the watcher.
	if burst >= debounce {
		t.Skipf("machine too slow to emit a burst inside the debounce window "+
			"(burst took %v, window %v) — the premise of this test did not hold", burst, debounce)
	}

	// Wait for the debounced message rather than assuming when it arrives.
	deadline := time.Now().Add(debounce + 10*time.Second)
	for sender.count.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sender.count.Load() == 0 {
		t.Fatalf("no debounced message within %v", debounce+10*time.Second)
	}

	// Still one a full window later. The grace has to be a whole window: were
	// the burst ever split, the poll above would return on the intermediate
	// message and the final timer would not fire until one window after the last
	// write, so a shorter wait would stop looking before the evidence arrived.
	time.Sleep(debounce)
	if got := sender.count.Load(); got != 1 {
		t.Errorf("expected 1 debounced message, got %d (burst took %v, window %v)",
			got, burst, debounce)
	}
}

func TestWatcherSuppress(t *testing.T) {
	dir := t.TempDir()
	unitFile := filepath.Join(dir, "test.mxunit")
	if err := os.WriteFile(unitFile, []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}
	w, err := newWatcher("", dir, sender)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Suppress for 2 seconds
	w.Suppress(2 * time.Second)

	// Write during suppress window
	_ = os.WriteFile(unitFile, []byte("b"), 0644)
	time.Sleep(700 * time.Millisecond)

	got := sender.count.Load()
	if got != 0 {
		t.Errorf("expected 0 messages during suppress, got %d", got)
	}
}

func TestWatcherCloseIdempotent(t *testing.T) {
	dir := t.TempDir()
	unitFile := filepath.Join(dir, "test.mxunit")
	_ = os.WriteFile(unitFile, []byte("a"), 0644)

	sender := &mockSender{}
	w, err := newWatcher("", dir, sender)
	if err != nil {
		t.Fatal(err)
	}

	// Double close should not panic
	w.Close()
	w.Close()
}

func TestWatcherIgnoresNonMxunitFiles(t *testing.T) {
	dir := t.TempDir()
	unitFile := filepath.Join(dir, "test.mxunit")
	_ = os.WriteFile(unitFile, []byte("a"), 0644)

	sender := &mockSender{}
	w, err := newWatcher("", dir, sender)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	// Write a .tmp file — should be ignored
	tmpFile := filepath.Join(dir, "test.tmp")
	_ = os.WriteFile(tmpFile, []byte("b"), 0644)
	time.Sleep(700 * time.Millisecond)

	got := sender.count.Load()
	if got != 0 {
		t.Errorf("expected 0 messages for .tmp file, got %d", got)
	}
}
