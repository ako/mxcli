// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

// `brain capture` is append-only and was measured concurrency-safe: three
// simultaneous captures land as three well-formed lines. That measurement is
// right and it is also the whole of the safety, which is easy to over-read.
//
// `promote` and `drop` are NOT appends. Both do Load -> rebuild the file in
// memory -> write it back whole, so a capture that lands between the load and
// the write is silently overwritten. Nothing reports it: the queue stays
// well-formed, one line is simply gone.
//
// That matters far more under the shape the brain is being pointed at. In one
// long session a lost capture is a lost note the author could still recall.
// With one sub-agent per slice, the brain is the ONLY channel between slices —
// slice 8's agent has no memory of slice 7 — so capture is the agent's return
// value and a lost one is a lost decision. The orchestrator promoting slice 7's
// entries while slice 8's agent captures is exactly the interleaving here.
//
// The test asserts conservation: every entry captured is present afterwards,
// and every entry dropped is gone. It runs the interleaving repeatedly because
// the window is small — measured against the unlocked implementation it loses
// entries on every run, and the failure is reported below with the count.
func TestQueueDoesNotLoseACaptureRacingAPromote(t *testing.T) {
	const rounds, capturesPerRound = 12, 6

	for round := range rounds {
		dir := t.TempDir()
		q := NewQueue(dir)

		// Seed an entry for the "promote" side to remove. A promote is a Get
		// followed by a Drop, and Drop is the half that rewrites the file.
		seed, err := NewEntry(fmt.Sprintf("seed decision %d", round), nil, day)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Append(seed); err != nil {
			t.Fatal(err)
		}

		// The captures a slice agent is making while the dispatcher promotes.
		captured := make([]Entry, capturesPerRound)
		for i := range captured {
			e, err := NewEntry(fmt.Sprintf("captured decision %d-%d", round, i), nil, day)
			if err != nil {
				t.Fatal(err)
			}
			captured[i] = e
		}

		var wg sync.WaitGroup
		start := make(chan struct{})
		wg.Add(len(captured) + 1)

		go func() {
			defer wg.Done()
			<-start
			if _, err := NewQueue(dir).Drop(seed.ID); err != nil {
				t.Errorf("drop: %v", err)
			}
		}()
		for _, e := range captured {
			go func() {
				defer wg.Done()
				<-start
				if _, err := NewQueue(dir).Append(e); err != nil {
					t.Errorf("append: %v", err)
				}
			}()
		}
		close(start)
		wg.Wait()

		final, err := q.Load()
		if err != nil {
			t.Fatalf("round %d: queue is unreadable after concurrent use: %v", round, err)
		}
		present := map[string]bool{}
		for _, e := range final {
			present[e.ID] = true
		}

		var lost []string
		for _, e := range captured {
			if !present[e.ID] {
				lost = append(lost, e.Title)
			}
		}
		if len(lost) > 0 {
			t.Fatalf("round %d: %d of %d captures were silently lost by a concurrent promote: %v\n"+
				"Under one sub-agent per slice the brain is the only channel between slices, "+
				"so a dropped capture is a dropped decision and nothing reports it.",
				round, len(lost), len(captured), lost)
		}
		if present[seed.ID] {
			t.Fatalf("round %d: the dropped entry came back — a concurrent capture rewrote the queue "+
				"from a snapshot taken before the drop", round)
		}
	}
}

// A crash or a full disk part-way through a rewrite must not leave a truncated
// queue behind: Load parses JSON per line and would report the survivor as a
// corrupt file, which reads like a bug in capture rather than an interrupted
// promote. Writing through a temp file and renaming makes the swap atomic, so
// a reader sees either the old queue or the new one.
func TestQueueRewriteIsAtomic(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir)

	var ids []string
	for i := range 40 {
		e, err := NewEntry(fmt.Sprintf("decision number %d with enough text to span bytes", i), nil, day)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := q.Append(e); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}

	// Read the queue continuously while it is rewritten underneath. Every read
	// must parse; a partially written file would not.
	stop := make(chan struct{})
	var readErr error
	var readWG sync.WaitGroup
	readWG.Add(1)
	go func() {
		defer readWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := NewQueue(dir).Load(); err != nil {
				readErr = err
				return
			}
		}
	}()

	for _, id := range ids[:20] {
		if _, err := q.Drop(id); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	readWG.Wait()

	if readErr != nil {
		t.Fatalf("a read during a rewrite saw a torn file: %v", readErr)
	}
}

// A process that dies holding the lock must not wedge the queue for every
// later capture. The lock is taken over once it is clearly abandoned, so the
// worst case is a delay rather than a project whose agents can no longer
// record anything.
func TestQueueLockIsTakenOverWhenAbandoned(t *testing.T) {
	dir := t.TempDir()
	q := NewQueue(dir)

	release, err := acquireQueueLock(q.Path)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate the holder dying: the lock file stays, nothing releases it.
	// Age it past the staleness threshold rather than waiting for it.
	ageQueueLock(t, q.Path, staleQueueLock+time.Second)

	e, err := NewEntry("a capture after the holder died", nil, day)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Append(e); err != nil {
		t.Fatalf("capture blocked forever on an abandoned lock: %v", err)
	}
	release()

	got, err := q.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != e.ID {
		t.Fatalf("entry did not land after taking over the abandoned lock: %v", got)
	}
}

// ageQueueLock backdates the lock file so the staleness path can be exercised
// without the test waiting out staleQueueLock.
func ageQueueLock(t *testing.T, queuePath string, by time.Duration) {
	t.Helper()
	lock := queueLockPath(queuePath)
	old := time.Now().Add(-by)
	if err := os.Chtimes(lock, old, old); err != nil {
		t.Fatalf("could not backdate %s: %v", lock, err)
	}
}
