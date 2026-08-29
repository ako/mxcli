// SPDX-License-Identifier: Apache-2.0

package executor

import "github.com/mendixlabs/mxcli/mdl/linter"

// firstBlockingViolation returns the first ERROR-severity violation, or nil when
// there is none.
//
// It exists because the obvious spelling is wrong in a way that is invisible
// until someone adds a warning. An exec-time guard written as
//
//	if vs := ValidateX(s); len(vs) > 0 { return error(vs[0]) }
//
// treats every violation as fatal, so the day a WARNING is added to ValidateX,
// `mxcli check` reports "0 errors, 1 warnings" and `mxcli exec` refuses the same
// script. Measured on MDL022 (the AutoX rename warning), which had this
// behaviour: check passed it as a warning and exec failed it as an error.
//
// That divergence is the specific thing ValidateProgram's contract exists to
// prevent — `check` and `exec` must agree on what is fatal — so the severity has
// to be consulted rather than the count.
func firstBlockingViolation(vs []linter.Violation) *linter.Violation {
	for i := range vs {
		if vs[i].Severity == linter.SeverityError {
			return &vs[i]
		}
	}
	return nil
}
