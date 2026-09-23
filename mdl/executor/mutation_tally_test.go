// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A settled script reports one "Unchanged …" line per statement, and those lines
// are the single most repeated thing mxcli prints. MEASURED on a 40-statement
// script against a project it had already been applied to: 40 lines, ~1.5 KB,
// every one of them saying nothing happened.
//
// That is cheap in a terminal and expensive in an agent session, where a tool
// result is written into the conversation once and RE-READ by every later model
// call — so 40 no-op lines early in a long session are not 40 lines of cost.
// See docs/11-proposals/PROPOSAL_agent_loop_efficiency.md.
//
// The collapse is deliberately narrow: only "Unchanged" is suppressed, because
// it is the one verb that by construction reports that nothing happened. Every
// verb that describes a real write is still printed, individually and in full —
// an agent (or a person) never loses a line they would have acted on.

func TestUnchangedLinesCollapseIntoOneSummary(t *testing.T) {
	ctx, mb, out := reportCtx(t)
	tally := &mutationTally{active: true}
	ctx.tally = tally

	for i := 0; i < 40; i++ {
		mb.offer(1, 0) // offered and elided: this is an "Unchanged"
		ctx.ReportMutation("Created", "entity: MyFirstModule.Od%02d", i)
	}
	tally.flush(ctx.Output)

	got := out.String()
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("40 unchanged statements printed %d lines, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, "40") {
		t.Errorf("the summary does not say how many were unchanged: %q", got)
	}
}

// THE CONTROL. Without it the test above passes against an implementation that
// swallows everything — which would be a far worse bug than the noise it fixes,
// because the run would look idempotent while rewriting the project.
func TestEveryRealWriteIsStillPrintedInFull(t *testing.T) {
	ctx, mb, out := reportCtx(t)
	tally := &mutationTally{active: true}
	ctx.tally = tally

	mb.offer(1, 1) // landed
	ctx.ReportMutation("Created", "entity: %s", "A")
	mb.offer(1, 0) // elided
	ctx.ReportMutation("Created", "entity: %s", "B")
	mb.offer(1, 1) // landed
	ctx.ReportMutation("Replaced", "nanoflow: %s", "C")
	mb.offer(1, 0) // elided — two of them, so they collapse
	ctx.ReportMutation("Created", "entity: %s", "D")
	tally.flush(ctx.Output)

	got := out.String()
	for _, want := range []string{"Created entity: A", "Replaced nanoflow: C"} {
		if !strings.Contains(got, want) {
			t.Errorf("a write that landed was not reported: want %q in\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"entity: B", "entity: D"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("an elided write was named individually; it should only be counted:\n%s", got)
		}
	}
	if !strings.Contains(got, "2 documents already in sync") {
		t.Errorf("the summary does not report the 2 unchanged documents: %q", got)
	}
}

// Nothing is suppressed outside a program run.
func TestNothingIsCollapsedWhenTheTallyIsInactive(t *testing.T) {
	ctx, mb, out := reportCtx(t)
	ctx.tally = &mutationTally{active: false}

	mb.offer(1, 0)
	ctx.ReportMutation("Created", "entity: %s", "A")

	if got := out.String(); got != "Unchanged entity: A\n" {
		t.Errorf("reported %q, want the per-statement line kept when inactive", got)
	}
}

// A SINGLE elided mutation is printed in full, even inside a program run.
// "1 document already in sync" is strictly worse than the line it replaces, and
// this is the case an entry-point-based rule got wrong: a `-c` one-liner reaches
// ExecuteProgram too, because main.go prepends a CONNECT statement, so gating on
// "is this a script?" collapsed exactly the case worth protecting. Measured
// before the fix: `mxcli -p app.mpr -c 'create or modify entity …'` printed
// "1 document already in sync (unchanged, not listed)" and named nothing.
func TestALoneUnchangedLineIsPrintedVerbatim(t *testing.T) {
	ctx, mb, out := reportCtx(t)
	tally := &mutationTally{active: true}
	ctx.tally = tally

	mb.offer(1, 0)
	ctx.ReportMutation("Created", "entity: %s", "MyFirstModule.Od01")
	tally.flush(ctx.Output)

	if got := out.String(); got != "Unchanged entity: MyFirstModule.Od01\n" {
		t.Errorf("reported %q, want the single elided line in full", got)
	}
}

// A nil tally is the zero-configuration path every existing caller takes.
func TestNilTallyBehavesExactlyAsBefore(t *testing.T) {
	ctx, mb, out := reportCtx(t)

	mb.offer(1, 0)
	ctx.ReportMutation("Created", "entity: %s", "A")

	if got := out.String(); got != "Unchanged entity: A\n" {
		t.Errorf("reported %q, want the unmodified behaviour on a nil tally", got)
	}
}

// A run in which nothing was elided must print no summary at all: a trailing
// "0 unchanged" on every clean first run is noise of exactly the kind this is
// meant to remove.
func TestNoSummaryWhenNothingWasUnchanged(t *testing.T) {
	ctx, mb, out := reportCtx(t)
	tally := &mutationTally{active: true}
	ctx.tally = tally

	mb.offer(1, 1)
	ctx.ReportMutation("Created", "entity: %s", "A")
	tally.flush(ctx.Output)

	if got := out.String(); got != "Created entity: A\n" {
		t.Errorf("reported %q, want no summary line when nothing was unchanged", got)
	}
}
