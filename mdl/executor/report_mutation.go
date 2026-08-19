// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// ReportMutation announces that a document was rewritten — but says "Unchanged"
// instead when the rewrite turned out to be a no-op.
//
// Storage skips a unit whose new content is semantically equal to what is
// already there (ADR-0008), so a handler's `ctx.Backend.UpdateNanoflow(nf)`
// returning nil does not mean anything reached disk. Re-running an
// already-applied script used to print
//
//	Modified javascript action: MxCore.JS_LoadAiAdvisor
//	Replaced nanoflow: HomeScan.ONL_AIAdvisor
//
// against a project whose files were not touched — not even their mtimes. The
// elision was real and correct; only the reporting was wrong, and it is wrong in
// the direction that costs the most: someone diagnosing version-control churn
// from console output concludes mxcli rewrites everything on every run, which is
// what happened in #910.
//
// The verb is downgraded only on positive evidence: unit writes were offered
// since the last report and none of them landed. A mutation that never reaches
// unit storage — a theme file, a backend with no notion of units, a mock — leaves
// no evidence either way and is reported unqualified, exactly as before.
//
// Sampling is per report rather than per statement so a handler that rewrites
// several documents in a loop (constants, module roles) still labels each one on
// its own merits.
func (ctx *ExecContext) ReportMutation(verb, format string, args ...any) {
	if ctx.mutationWasElided() {
		verb = "Unchanged"
	}
	fmt.Fprintf(ctx.Output, "%s %s\n", verb, fmt.Sprintf(format, args...))
}

// mutationWasElided reports whether every unit write since the previous call was
// skipped as a no-op, and advances the watermark.
func (ctx *ExecContext) mutationWasElided() bool {
	now := currentWriteStats(ctx.Backend)
	prev := ctx.lastWriteStats
	ctx.lastWriteStats = now
	// Offered but nothing written is the only case that proves an elision. The
	// first call in a statement compares against the watermark newExecContext
	// took, so work done by earlier statements is never attributed to this one.
	return now.Offered > prev.Offered && now.Written == prev.Written
}

// currentWriteStats reads a backend's write counters, or the zero value from one
// that does not keep them.
func currentWriteStats(b backend.FullBackend) backend.WriteStats {
	reporter, ok := b.(backend.WriteStatsReporter)
	if !ok {
		return backend.WriteStats{}
	}
	return reporter.WriteStats()
}
