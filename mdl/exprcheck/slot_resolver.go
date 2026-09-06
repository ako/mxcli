// SPDX-License-Identifier: Apache-2.0

package exprcheck

// staticExpectations maps MDL slot paths to their expected expression type.
// Add a new entry whenever a new MDL statement slot is added to the executor.
// Slot paths mirror the AST node + field name, e.g. "IfStmt.Condition".
var staticExpectations = map[string]SlotConstraint{
	"IfStmt.Condition":        {Kind: KindBoolean},
	"WhileStmt.Condition":     {Kind: KindBoolean},
	"RetrieveStmt.LimitExpr":  {Kind: KindInteger},
	"RetrieveStmt.OffsetExpr": {Kind: KindInteger},
	"ChangeItem.Value":        {Kind: KindUnknown, ResolveBy: "AttributeOf:Parent"},
	"CreateItem.Value":        {Kind: KindUnknown, ResolveBy: "AttributeOf:Parent"},
	"ReturnStmt.Value":        {Kind: KindUnknown, ResolveBy: "MicroflowReturn"},
	"CallArgument.Value":      {Kind: KindUnknown, ResolveBy: "TargetParameter"},
	"LogStmt.Message":         {Kind: KindString},
	// A log message's template parameter must be a String — Mendix does not
	// coerce here, and a non-String one is CE0117 "Error(s) in expression" on
	// the activity. Measured on 11.13.0: an Integer attribute, an integer
	// literal and an object each fail; a String attribute is clean; and
	// toString(...) around any of them is clean (mendixlabs/mxcli#1043).
	"LogStmt.TemplateParam":    {Kind: KindString},
	"MfSetStmt.Value":          {Kind: KindUnknown, ResolveBy: "TargetVariable"},
	"DeclareStmt.InitialValue": {Kind: KindUnknown, ResolveBy: "DeclareType"},
}

type defaultSlotResolver struct{}

func DefaultSlotResolver() SlotResolver { return &defaultSlotResolver{} }

func (r *defaultSlotResolver) Expect(path string) (SlotConstraint, bool) {
	sc, ok := staticExpectations[path]
	return sc, ok
}
