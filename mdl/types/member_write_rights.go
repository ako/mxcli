// SPDX-License-Identifier: Apache-2.0

package types

// WriteRightsForbidden reports whether Mendix refuses write access on an entity
// member with these properties, which the build reports as CE6592.
//
// There are two causes and they are easy to mistake for one. A CALCULATED
// attribute has its value produced by a microflow on read; an AUTONUMBER has its
// value produced by the database on insert. Neither is a value a user may set,
// so Mendix rejects an access rule granting write on either — but they are
// unrelated in the model: an autonumber carries no DomainModels$CalculatedValue,
// so a predicate written as "is calculated" covers exactly half the rule.
//
// That half-rule is what mendixlabs/mxcli#524 was: `grant write *` on an entity
// with an autonumber wrote ReadWrite and failed the build, and the user had to
// narrow the grant with a REVOKE by hand.
//
// The rule lives here, in one currency-free place, because the two callers speak
// different ones — the executor sees sdk/domainmodel types and the codec backend
// sees modelsdk/gen types, and neither may import the other. Each side detects
// its own two booleans and asks this function what they mean. Two copies of the
// predicate in two currencies is how a resolver drifts, which is the mistake
// CLAUDE.md records against the layout placeholder check.
func WriteRightsForbidden(isCalculated, isAutoNumber bool) bool {
	return isCalculated || isAutoNumber
}
