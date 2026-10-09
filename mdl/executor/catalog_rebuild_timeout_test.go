// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1329: SHOW REFERENCES, SEARCH, SHOW CALLERS and SHOW IMPACT
// rebuild a stale catalog on the way to their answer, at the cached source
// mode. On a 3,000-document app that build outlasts the default 5m statement
// limit; the guard fired mid-build, the unfinished build was never saved, and
// every later call started the same rebuild over:
//
//	statement timed out after 5m0s
//
// The fixture builds in well under a second, so the test lowers the default
// limit and makes the build outlast it — the same arithmetic as the report.

// slowImplicitRebuilds makes every implicit catalog rebuild take at least d
// longer, and lowers the default statement limit to limit. The returned channel
// receives once per finished rebuild, so a test can wait for one the guard
// abandoned before its fixture is torn down.
func slowImplicitRebuilds(t *testing.T, d, limit time.Duration) <-chan struct{} {
	t.Helper()
	done := make(chan struct{}, 8)
	prevHook, prevLimit := implicitCatalogBuildHook, executeTimeoutDefault
	implicitCatalogBuildHook = func() func() {
		time.Sleep(d)
		return func() { done <- struct{}{} }
	}
	executeTimeoutDefault = limit
	t.Cleanup(func() {
		implicitCatalogBuildHook, executeTimeoutDefault = prevHook, prevLimit
	})
	return done
}

func execStatement(t *testing.T, exec *Executor, mdl string) error {
	t.Helper()
	prog, errs := visitor.Build(mdl)
	if len(errs) > 0 || len(prog.Statements) != 1 {
		t.Fatalf("parsing %q: %v", mdl, errs)
	}
	return exec.Execute(prog.Statements[0])
}

// TestImplicitRebuildIsNotCountedAgainstTheDefaultLimit is the report: a
// statement whose implicit rebuild outlasts the default limit must finish, and
// leave a current cache so the next call does not rebuild again.
func TestImplicitRebuildIsNotCountedAgainstTheDefaultLimit(t *testing.T) {
	exec, cachePath := sourceCachedFixture(t)
	t.Setenv("MXCLI_EXEC_TIMEOUT", "")
	slowImplicitRebuilds(t, 1500*time.Millisecond, 500*time.Millisecond)

	touchProject(t, exec)

	if err := execStatement(t, exec, "SHOW REFERENCES TO MyFirstModule.Ticket"); err != nil {
		t.Fatalf("SHOW REFERENCES after the project changed: %v", err)
	}

	ctx := exec.newExecContext(t.Context())
	if valid, reason := isCacheValid(ctx, cachePath, "source"); !valid {
		t.Errorf("cache still invalid after the implicit rebuild: %s — the next call rebuilds again", reason)
	}
}

// TestDefaultLimitStillCountsTheRestOfTheStatement is the control on the
// exclusion: only the rebuild is skipped. A statement that is itself slow —
// the runaway loop the guard is for — still times out.
func TestDefaultLimitStillCountsTheRestOfTheStatement(t *testing.T) {
	exec := New(&strings.Builder{})
	w := exec.pushUntimed(true)
	defer exec.popUntimed(w)
	begun := time.Now()
	time.Sleep(600 * time.Millisecond)
	if counted := time.Since(begun) - w.excluded(time.Now()); counted < 500*time.Millisecond {
		t.Errorf("counted %v of a statement with no rebuild, want the whole of it", counted)
	}
}

// TestExplicitLimitCapsTheRebuildAndSaysSo: an explicit MXCLI_EXEC_TIMEOUT caps
// everything, REFRESH CATALOG included, so it caps an implicit rebuild too. The
// error then has to say a rebuild was cut off, because the cache was not
// updated and the next call will rebuild again — without that, each retry
// reads as a new failure.
func TestExplicitLimitCapsTheRebuildAndSaysSo(t *testing.T) {
	exec, _ := sourceCachedFixture(t)
	t.Setenv("MXCLI_EXEC_TIMEOUT", "500ms")
	rebuilt := slowImplicitRebuilds(t, 1500*time.Millisecond, 500*time.Millisecond)

	touchProject(t, exec)

	err := execStatement(t, exec, "SHOW REFERENCES TO MyFirstModule.Ticket")
	if err == nil {
		t.Fatal("SHOW REFERENCES finished under an explicit 500ms cap with a 1.5s rebuild")
	}
	msg := err.Error()
	for _, want := range []string{"timed out", "rebuilding the catalog", "source mode", "REFRESH CATALOG FULL SOURCE"} {
		if !strings.Contains(msg, want) {
			t.Errorf("timeout error lacks %q:\n%s", want, msg)
		}
	}
	// Let the abandoned build finish before the fixture's cleanup closes it.
	select {
	case <-rebuilt:
	case <-time.After(time.Minute):
		t.Fatal("the abandoned rebuild did not finish")
	}
}
