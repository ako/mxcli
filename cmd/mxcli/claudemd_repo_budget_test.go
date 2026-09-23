// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"testing"
)

// This repo's own CLAUDE.md is re-read into every context started here, exactly
// as the generated one is for a user's project — so its size is a per-session
// tax, and the reasoning in init_claudemd_budget_test.go applies to it too.
//
// It was not applied. The file reached 108,761 B (~27k tokens), 18x the 6,000
// enforced on users' projects, while that budget test sat in the same package
// (ako/mxcli#611). Four slices took it to ~25.5k by moving per-subsystem detail
// into the skill or doc that owns it, and deleting what `mxcli syntax`,
// `mxcli help` and a directory listing answer authoritatively.
//
// The budget is higher than the generated file's because this one legitimately
// carries more: the invariants whose violation is silent and unrecoverable
// (GUID-as-database-identity, conditional writes, storage names) plus the
// evidence bar for a change. It is set just above the current size on purpose —
// enough to edit within, not enough to regrow into. Adding something here means
// taking something out, which is the decision the budget exists to force.
const repoClaudeMDBudgetBytes = 28000

func TestRepoClaudeMDStaysWithinItsContextBudget(t *testing.T) {
	const path = "../../CLAUDE.md"
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("cannot stat %s: %v", path, err)
	}
	if info.Size() > repoClaudeMDBudgetBytes {
		t.Errorf("CLAUDE.md is %d bytes (~%d tokens), over the %d-byte budget.\n"+
			"It is re-read into every context started in this repo. Anything needed only when\n"+
			"touching one subsystem belongs in that subsystem's skill or doc; anything `mxcli\n"+
			"syntax`, `mxcli help` or `ls` answers belongs nowhere. See ako/mxcli#611.",
			info.Size(), info.Size()/4, repoClaudeMDBudgetBytes)
	}
}
