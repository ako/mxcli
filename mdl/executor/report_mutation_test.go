// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
)

// countingBackend is a mock that also reports write statistics, so a test can
// drive the elision path without a project on disk.
type countingBackend struct {
	*mock.MockBackend
	stats backend.WriteStats
}

func (b *countingBackend) WriteStats() backend.WriteStats { return b.stats }

// offer records n unit writes of which written actually landed.
func (b *countingBackend) offer(offered, written int) {
	b.stats.Offered += offered
	b.stats.Written += written
}

func reportCtx(t *testing.T) (*ExecContext, *countingBackend, *bytes.Buffer) {
	t.Helper()
	mb := &countingBackend{MockBackend: &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
	}}
	out := &bytes.Buffer{}
	return &ExecContext{Backend: mb, Output: out}, mb, out
}

// TestReportMutationSaysUnchangedWhenNothingWasWritten is the reporting half of
// the ticket: re-running an already-applied script printed "Modified …" and
// "Replaced …" for documents whose files were not touched, not even their
// mtimes. The elision was right; the console was not, and someone diagnosing
// version-control churn from it concludes mxcli rewrites on every run.
func TestReportMutationSaysUnchangedWhenNothingWasWritten(t *testing.T) {
	ctx, mb, out := reportCtx(t)

	mb.offer(1, 0) // one unit write offered to storage, elided as a no-op
	ctx.ReportMutation("Replaced", "nanoflow: %s", "HomeScan.ONL_AIAdvisor")

	if got := out.String(); got != "Unchanged nanoflow: HomeScan.ONL_AIAdvisor\n" {
		t.Errorf("reported %q, want the write to be described as unchanged", got)
	}
}

// TestReportMutationKeepsTheVerbWhenTheWriteLanded is the control. Without it
// the test above would pass against a helper that said "Unchanged" always.
func TestReportMutationKeepsTheVerbWhenTheWriteLanded(t *testing.T) {
	ctx, mb, out := reportCtx(t)

	mb.offer(1, 1)
	ctx.ReportMutation("Replaced", "nanoflow: %s", "HomeScan.ONL_AIAdvisor")

	if got := out.String(); got != "Replaced nanoflow: HomeScan.ONL_AIAdvisor\n" {
		t.Errorf("reported %q, want the original verb", got)
	}
}

// TestReportMutationJudgesEachReportSeparately pins the granularity. A handler
// that rewrites several documents in one statement — constants, module roles —
// must label each on its own merits rather than tarring them all with the
// statement's overall outcome.
func TestReportMutationJudgesEachReportSeparately(t *testing.T) {
	ctx, mb, out := reportCtx(t)

	mb.offer(1, 1)
	ctx.ReportMutation("Modified", "constant: %s", "App.First")
	mb.offer(1, 0)
	ctx.ReportMutation("Modified", "constant: %s", "App.Second")
	mb.offer(1, 1)
	ctx.ReportMutation("Modified", "constant: %s", "App.Third")

	want := strings.Join([]string{
		"Modified constant: App.First",
		"Unchanged constant: App.Second",
		"Modified constant: App.Third",
		"",
	}, "\n")
	if got := out.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestReportMutationNeedsPositiveEvidence pins the failure direction. A mutation
// that never reaches unit storage — a theme file, a backend with no notion of
// units — leaves no evidence either way, and must be reported as it always was
// rather than guessed at.
func TestReportMutationNeedsPositiveEvidence(t *testing.T) {
	ctx, _, out := reportCtx(t)
	ctx.ReportMutation("Modified", "theme: %s", "signal")
	if got := out.String(); got != "Modified theme: signal\n" {
		t.Errorf("with no unit write offered, reported %q", got)
	}

	// A backend that does not implement WriteStatsReporter at all is the same
	// case, and is what every mock-driven executor test goes through.
	plain := &ExecContext{
		Backend: &mock.MockBackend{IsConnectedFunc: func() bool { return true }},
		Output:  &bytes.Buffer{},
	}
	plain.ReportMutation("Replaced", "microflow: %s", "App.Flow")
	if got := plain.Output.(*bytes.Buffer).String(); got != "Replaced microflow: App.Flow\n" {
		t.Errorf("a backend with no write stats reported %q", got)
	}
}

// TestReportMutationIgnoresEarlierStatements pins that the watermark starts at
// the statement boundary. newExecContext samples the backend's counters when it
// builds the context, so a write by an earlier statement cannot make this one
// look like it landed.
func TestReportMutationIgnoresEarlierStatements(t *testing.T) {
	ctx, mb, out := reportCtx(t)
	mb.offer(5, 5) // five earlier statements, all of which really wrote
	ctx.lastWriteStats = mb.WriteStats()

	mb.offer(1, 0)
	ctx.ReportMutation("Modified", "entity: %s", "App.Thing")

	if got := out.String(); got != "Unchanged entity: App.Thing\n" {
		t.Errorf("reported %q — earlier statements' writes were counted against this one", got)
	}
}
