// SPDX-License-Identifier: Apache-2.0

package ast

// DropGuard is embedded in every document-level DROP statement. IfExists
// downgrades "not found" to a skip, so a script that drops something — a stub
// page that broke a cycle, a renamed document's old name — can be re-run.
//
// The guard is honoured once, in the executor's dispatch, rather than in each
// handler: there are some thirty drop handlers, and a guard that has to be
// remembered per handler is the one the next doctype forgets (#531).
type DropGuard struct {
	IfExists bool
}

// DropIfExists reports whether the statement was written DROP … IF EXISTS.
func (g *DropGuard) DropIfExists() bool { return g.IfExists }

// SetDropIfExists records the guard; the visitor calls it once for whichever
// drop statement it built.
func (g *DropGuard) SetDropIfExists(v bool) { g.IfExists = v }

// IfExistsDrop is implemented by every statement that embeds DropGuard.
type IfExistsDrop interface {
	Statement
	DropIfExists() bool
	SetDropIfExists(bool)
}
