// SPDX-License-Identifier: Apache-2.0

package executor

// Excluded documents ("Exclude from project" in Studio Pro) make a document
// name non-unique. Mendix allows two documents in one module to share a name as
// long as at most one of them is active: mxbuild reports CE0122 "Duplicate
// document name" only for the active ones. Measured on 11.13.0 — two microflows
// named MyFirstModule.Calc with one Excluded build at 0 errors; the same pair
// with both active is 1 error, CE0122.
//
// Two consequences for every by-name lookup and every rewrite:
//
//  1. A name is not a unique key, so picking the first match is order-dependent.
//     It can resolve to a document the app does not contain — DESCRIBE then
//     shows the wrong body, and CREATE OR REPLACE edits the wrong document
//     while the live one silently keeps its old behaviour.
//  2. Excluded belongs to the model, not to the MDL script. A rebuild that does
//     not carry it forward turns an excluded document active, which makes a
//     valid project fail CE0122 (guard-don't-drop, ADR-0005) — a build broken by
//     a statement that never mentioned exclusion.
//
// pickLive addresses (1). preserveExcluded (each call site, one line next to the
// ID/roles it already preserves) addresses (2).
//
// Issue #914.

// pickLive returns the document the running app uses: the active (non-excluded)
// match when there is one, otherwise the first match so that a lookup against an
// all-excluded set still resolves rather than reporting "not found".
//
// matches selects the candidates (typically name + module); excluded reports a
// candidate's Excluded flag. The bool result is false only when nothing matched.
func pickLive[T any](items []T, matches func(T) bool, excluded func(T) bool) (T, bool) {
	var first T
	found := false
	for _, it := range items {
		if !matches(it) {
			continue
		}
		if !excluded(it) {
			return it, true
		}
		if !found {
			first, found = it, true
		}
	}
	return first, found
}

// pickLiveIndex is pickLive for call sites that need the position rather than
// the element (slice mutation in place, or a parallel slice).
func pickLiveIndex[T any](items []T, matches func(T) bool, excluded func(T) bool) int {
	first := -1
	for i, it := range items {
		if !matches(it) {
			continue
		}
		if !excluded(it) {
			return i
		}
		if first < 0 {
			first = i
		}
	}
	return first
}
