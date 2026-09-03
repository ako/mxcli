// SPDX-License-Identifier: Apache-2.0

package executor

// carriedDocumentation returns the documentation a rewrite should store.
//
// A statement that carried a `/** … */` comment — even an empty one — states its
// intent and wins. A statement that carried none said nothing about
// documentation, so the stored value is preserved.
//
// Before this, every rewrite path wrote the statement's value unconditionally,
// so a statement with no doc comment overwrote stored prose with "" and reported
// success (mendixlabs/mxcli#1018). It is an empty overwrite, not a drop, which is
// why nothing downstream could see it: the resulting document is valid, and
// `mx check` has no code for "this used to say something".
//
// Preserve-always was not an option. `SET DOCUMENTATION` / `SET COMMENT` exist
// only in the domain-model grammar — there is no ALTER for a microflow, a queue
// or a workflow — so an explicitly empty comment is the only clearing spelling
// most doctypes have.
func carriedDocumentation(set bool, stated, stored string) string {
	if set {
		return stated
	}
	return stored
}
