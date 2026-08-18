// SPDX-License-Identifier: Apache-2.0

package exprcheck

// Context carries per-call metadata into the parser.
//
// Two usage modes:
//   - Syntax-only: Catalog and Slots are nil; semantic hint rules (E001, E002,
//     E009, E011, …) are silently skipped. Use NewSyntaxContext.
//   - Semantic: Catalog and Slots are non-nil; all hint rules are active.
//     Use NewSemanticContext.
//
// Scope is always optional: when nil, variable type inference falls back to KindUnknown.
type Context struct {
	SlotPath  string
	Microflow string
	File      string
	Line      int
	Column    int

	Scope    Scope
	Catalog  CatalogReader // nil → semantic checks disabled
	Slots    SlotResolver  // nil → slot-kind checks disabled
	Entities EntityScope   // nil → attribute paths stay KindUnknown
}

// IsSemanticEnabled reports whether Catalog and Slots are both wired so that
// semantic hint rules can run. Callers can gate expensive lookups behind this
// check instead of repeating nil guards.
func (c Context) IsSemanticEnabled() bool {
	return c.Catalog != nil && c.Slots != nil
}

// NewSyntaxContext creates a Context for syntax-only parsing (E003, E007, E011
// structurally wired; semantic rules that need Catalog/Slots are inactive).
func NewSyntaxContext(slotPath, microflow string) Context {
	return Context{SlotPath: slotPath, Microflow: microflow}
}

// NewSemanticContext creates a Context with full semantic checking enabled.
// Both slots and catalog must be non-nil.
func NewSemanticContext(slotPath, microflow string, slots SlotResolver, catalog CatalogReader) Context {
	return Context{
		SlotPath:  slotPath,
		Microflow: microflow,
		Slots:     slots,
		Catalog:   catalog,
	}
}

type Parser interface {
	Parse(source string, ctx Context) (RobustExpr, []Hint)
}

type SlotResolver interface {
	Expect(slotPath string) (SlotConstraint, bool)
}

type CatalogReader interface {
	AttributeKind(entityQN, attrName string) (TypeKind, bool)
	AttributeEnumQN(entityQN, attrName string) (string, bool)
	EnumCases(enumQN string) ([]string, bool)
	MicroflowReturn(qn string) (TypeKind, bool)
	MicroflowParam(qn, paramName string) (TypeKind, bool)
}

type Scope interface {
	Lookup(name string) (TypeKind, bool)
}

// EntityScope resolves the object side of an expression: which entity a
// variable holds, and where an association leads from there.
//
// It is separate from Scope because Scope speaks TypeKind, which can say "$P is
// an Object" but not *which* entity — and without that, `$P/Status` cannot be
// resolved to an attribute at all. Keeping it out of CatalogReader too: the
// variable half is per-microflow rather than per-project, and leaving
// CatalogReader's shape untouched keeps it re-syncable from the upstream fork.
//
// A nil Context.Entities is the pre-existing behaviour: attribute paths infer
// KindUnknown and every rule that depends on them stays quiet.
type EntityScope interface {
	// VariableEntity returns the qualified entity name a variable holds. The
	// name is passed without the leading '$'.
	VariableEntity(name string) (string, bool)
	// AssociationTarget returns the entity at the other end of association
	// assocQN traversed from fromEntityQN, and whether assocQN is an
	// association at all. Both directions resolve — a Mendix expression can
	// follow an association from either end.
	AssociationTarget(assocQN, fromEntityQN string) (string, bool)
}

type SlotConstraint struct {
	Kind      TypeKind
	ResolveBy string
	Frequency int
	Samples   []string
}

type TypeKind int

const (
	KindUnknown TypeKind = iota
	KindAny
	KindBoolean
	KindString
	KindInteger
	KindLong
	KindDecimal
	KindDateTime
	KindBinary
	KindObject
	KindList
	KindEnumeration
	KindEmpty
)

// Hint is now defined as a type alias to hints.Hint in hint.go (P1.6).
// RobustExpr lives in ast.go (P1.2).
