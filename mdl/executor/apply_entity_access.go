// SPDX-License-Identifier: Apache-2.0

package executor

// carriedApplyEntityAccess decides a rewrite's "apply entity access" setting.
//
// An ABSENT annotation preserves what is stored; `@applyentityaccess` and
// `@applyentityaccess(false)` set it. That asymmetry is deliberate and it is the
// same rule `@excluded` and the doc comment follow (#914, #1018): the setting is
// model state, not script state, so a statement that does not mention it must
// not decide it.
//
// It matters more here than for those two because this is a SECURITY setting and
// it only ever narrows. Defaulting an unstated rewrite to false turned "apply
// entity access" OFF — widening what the microflow may read and write — and
// nothing reported it: `mxcli check` is quiet, mxbuild is quiet, and the model
// is valid either way. Measured across 342 microflows in 4 projects: every
// microflow storing true came back false.
//
// On a CREATE there is nothing stored, so stored is false and a fresh microflow
// gets Studio Pro's default (326 of 342 in that corpus are false).
func carriedApplyEntityAccess(stated *bool, stored bool) bool {
	if stated != nil {
		return *stated
	}
	return stored
}
