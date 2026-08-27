// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/offlinepaths"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// maxReportedOfflinePaths caps the listing. A project that trips this has a
// systemic problem, not a list of things to fix one by one, and a wall of
// output is how a real warning gets scrolled past.
const maxReportedOfflinePaths = 10

// warnOfflineIncompatiblePages reports the pages that CREATING an offline
// profile has just invalidated.
//
// This is the finding from ako/mxcli-maintenance, and it is a warning about
// documents the statement never named. An offline navigation profile restricts
// every page it can reach: an attribute may be bound across at most ONE
// association hop, and Mendix rejects a longer path with CE6206. The pages were
// valid before the statement ran. Adding the profile is what broke them.
//
// Without this, `create or replace navigation TabletOffline` succeeds, prints
// one cheerful line, and the next build fails in pages the author has no reason
// to connect to what they just did. Studio Pro would not have let them get here
// without saying so; mxcli was silent.
//
// It is a WARNING and not a refusal, deliberately. Whether a page is REACHABLE
// from the new profile takes the whole page graph — home pages, menu items, and
// every page those open — which mxcli does not walk here. Refusing a statement
// on a reachability that has not been established would block work that is
// perfectly valid; reporting it names the risk and leaves the judgement where
// the evidence is. Studio Pro decides.
func warnOfflineIncompatiblePages(ctx *ExecContext, kind string) {
	if !types.IsOfflineProfileKind(kind) {
		return
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return
	}
	src, ok := ctx.Backend.(offlinepaths.Source)
	if !ok {
		return
	}
	qualify := func(containerID model.ID, name string) string {
		if h == nil {
			return name
		}
		if mod := h.GetModuleName(h.FindModuleID(containerID)); mod != "" {
			return mod + "." + name
		}
		return name
	}

	findings, err := offlinepaths.Scan(src, qualify)
	if err != nil || len(findings) == 0 {
		return
	}

	docs := 0
	var last string
	for _, f := range findings {
		if f.Document != last {
			docs++
			last = f.Document
		}
	}

	fmt.Fprintf(ctx.Output,
		"\nWarning: %s is an offline profile, and %d %s in this project %s an attribute across more than one association.\n",
		kind, docs, plural(docs, "document", "documents"), plural(docs, "binds", "bind"))
	fmt.Fprintln(ctx.Output,
		"Mendix rejects a multi-step attribute path with CE6206 on any page reachable from an offline profile — one hop is allowed, two are not.")

	shown := findings
	if len(shown) > maxReportedOfflinePaths {
		shown = shown[:maxReportedOfflinePaths]
	}
	for _, f := range shown {
		fmt.Fprintf(ctx.Output, "  %s %s: %s (%d steps)\n", f.Kind, f.Document, f.Path, f.Steps)
	}
	if len(findings) > len(shown) {
		fmt.Fprintf(ctx.Output, "  ... and %d more\n", len(findings)-len(shown))
	}
	fmt.Fprintln(ctx.Output,
		"These are errors only for pages the profile can actually reach, which mxcli does not compute — run `mxcli docker check -p <project>` or open the app in Studio Pro to see which ones apply.")
}
