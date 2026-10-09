// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"sync"
	"time"
)

// untimedWork tracks the part of a statement's run that the wall-clock guard
// should not count: an implicit catalog rebuild. The guard exists to abort
// runaway loops, and a catalog build is bounded and deterministic — the reason
// REFRESH CATALOG is exempt (#651). A statement that rebuilds the catalog on
// the way to its answer (SHOW REFERENCES, SEARCH, SHOW CALLERS, SHOW IMPACT
// after the project file changed) was not, so on a large app the default 5m
// fired mid-build. The build is only saved once it finishes, so the cache
// stayed stale and every later call paid for the same doomed rebuild
// (mendixlabs/mxcli#1329).
//
// One is created per Execute. A nested statement (EXECUTE SCRIPT) gets its own,
// chained to the enclosing one, so a build inside it is excluded from the outer
// statement's clock as well.
type untimedWork struct {
	parent *untimedWork
	// exclude says whether this statement's clock skips untimed work. False
	// under an explicit MXCLI_EXEC_TIMEOUT: an explicit cap caps everything,
	// REFRESH CATALOG included, so it caps an implicit rebuild too.
	exclude bool

	mu     sync.Mutex
	active int           // builds in progress
	since  time.Time     // when the outermost in-progress build began
	total  time.Duration // finished untimed time
	what   string        // description of the build in progress, for the timeout message
}

type untimedWorkKey struct{}

func newUntimedWork(parent *untimedWork, exclude bool) *untimedWork {
	return &untimedWork{parent: parent, exclude: exclude}
}

// withUntimedWork returns ctx carrying w, so a handler deep in the call chain
// can mark its untimed section without the tracker being threaded through.
func withUntimedWork(ctx context.Context, w *untimedWork) context.Context {
	return context.WithValue(ctx, untimedWorkKey{}, w)
}

// beginUntimed marks the start of an implicit catalog build on the statement
// running under ctx, and returns the function that marks its end. A context
// without a tracker (a direct call from a test or a tool) is a no-op.
func beginUntimed(ctx context.Context, what string) (end func()) {
	if ctx == nil {
		return func() {}
	}
	w, _ := ctx.Value(untimedWorkKey{}).(*untimedWork)
	if w == nil {
		return func() {}
	}
	now := time.Now()
	for l := w; l != nil; l = l.parent {
		l.begin(now, what)
	}
	return func() {
		now := time.Now()
		for l := w; l != nil; l = l.parent {
			l.end(now)
		}
	}
}

func (w *untimedWork) begin(now time.Time, what string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active == 0 {
		w.since = now
		w.what = what
	}
	w.active++
}

func (w *untimedWork) end(now time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active == 0 {
		return
	}
	w.active--
	if w.active == 0 {
		w.total += now.Sub(w.since)
		w.what = ""
	}
}

// excluded is how much of the time up to now the clock must not count.
func (w *untimedWork) excluded(now time.Time) time.Duration {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.exclude {
		return 0
	}
	d := w.total
	if w.active > 0 {
		d += now.Sub(w.since)
	}
	return d
}

// inProgress describes the untimed build running right now, or "" if none.
func (w *untimedWork) inProgress() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.active == 0 {
		return ""
	}
	return w.what
}
