// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"io"
)

// mutationTally collapses the "Unchanged …" lines of a program run into one
// summary, and counts nothing else.
//
// Re-running a settled script printed one line per statement saying that nothing
// had happened — measured at 40 lines and ~1.5 KB for a 40-statement script. In
// a terminal that is merely noisy. In an agent session it is charged repeatedly:
// a tool result is written into the conversation once and RE-READ by every later
// model call, so the cost of a no-op line is its length times the number of calls
// that follow it (docs/11-proposals/PROPOSAL_agent_loop_efficiency.md).
//
// Only "Unchanged" is suppressed, and that narrowness is the whole safety
// argument. It is the one verb that by construction reports an absence: storage
// was offered a write and skipped it (ADR-0008). Every verb naming a real write
// is still printed individually and in full, so nothing a reader would act on is
// replaced by a number. Suppressing on volume instead — "collapse after N lines"
// — would have hidden real writes in exactly the runs where they matter most.
type mutationTally struct {
	// active is set for the duration of a program run.
	//
	// It is NOT what decides whether anything collapses. A single elided
	// mutation is held and printed verbatim at flush, because "1 document
	// already in sync" is strictly worse than the line it replaces — and that
	// has to be decided by how many arrive, not by which entry point ran. A
	// `-c` one-liner reaches ExecuteProgram too (main.go prepends CONNECT), so
	// gating on the entry point collapsed exactly the case the rule exists to
	// protect.
	active    bool
	unchanged int
	// first holds the one elided line seen so far, so it can still be printed
	// in full if no second one arrives.
	first string
}

// begin activates the tally and reports whether THIS call owns it. A nested run
// (EXECUTE SCRIPT inside a script) finds it already active and returns false, so
// only the outermost program flushes and a nested script does not emit a second
// summary mid-run.
func (t *mutationTally) begin() bool {
	if t == nil || t.active {
		return false
	}
	t.active = true
	t.unchanged = 0
	return true
}

// end deactivates the tally after the owning run has flushed it.
func (t *mutationTally) end() {
	if t != nil {
		t.active = false
	}
}

// countUnchanged records an elided mutation and reports whether the caller
// should stay quiet about it for now. The line is passed in because a run with
// exactly one elision prints it verbatim at flush rather than a count of one.
func (t *mutationTally) countUnchanged(line string) bool {
	if t == nil || !t.active {
		return false
	}
	t.unchanged++
	if t.unchanged == 1 {
		t.first = line
	}
	return true
}

// flush writes the one-line summary, if there is anything to summarise. A run
// with nothing elided prints nothing: a trailing "0 unchanged" on every clean
// first run is the same noise from the other side.
func (t *mutationTally) flush(w io.Writer) {
	if t == nil || !t.active || t.unchanged == 0 {
		return
	}
	if t.unchanged == 1 {
		// Nothing was gained by holding it: print the line as it always was.
		fmt.Fprint(w, t.first)
	} else {
		fmt.Fprintf(w, "%d documents already in sync (unchanged, not listed)\n", t.unchanged)
	}
	t.unchanged = 0
	t.first = ""
}
