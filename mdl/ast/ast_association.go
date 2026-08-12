// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Association Statements
// ============================================================================

// AssociationType represents the type of association.
type AssociationType int

const (
	AssocReference AssociationType = iota
	AssocReferenceSet
)

func (t AssociationType) String() string {
	if t == AssocReference {
		return "Reference"
	}
	return "ReferenceSet"
}

// OwnerType represents the ownership of an association.
type OwnerType int

const (
	OwnerDefault OwnerType = iota
	OwnerBoth
	OwnerParent
	OwnerChild
)

func (o OwnerType) String() string {
	switch o {
	case OwnerDefault:
		return "Default"
	case OwnerBoth:
		return "Both"
	case OwnerParent:
		return "Parent"
	case OwnerChild:
		return "Child"
	default:
		return "Default"
	}
}

// DeleteBehavior represents the delete behavior of an association.
type DeleteBehavior int

const (
	DeleteKeepReferences DeleteBehavior = iota
	DeleteCascade
	DeleteBoth
	DeleteKeepParentDeleteChild
	DeleteKeepChildDeleteParent
	DeleteIfNoReferences
)

func (d DeleteBehavior) String() string {
	switch d {
	case DeleteKeepReferences:
		return "DeleteMeButKeepReferences"
	case DeleteCascade:
		return "DeleteMeAndReferences"
	case DeleteBoth:
		return "DeleteBoth"
	case DeleteKeepParentDeleteChild:
		return "KeepParentDeleteChild"
	case DeleteKeepChildDeleteParent:
		return "KeepChildDeleteParent"
	case DeleteIfNoReferences:
		return "DeleteIfNoReferences"
	default:
		return "DeleteMeButKeepReferences"
	}
}

// StorageType represents how an association is stored in the database.
type StorageType int

const (
	StorageDefault StorageType = iota // Not specified (defaults to Table)
	StorageColumn
	StorageTable
)

func (s StorageType) String() string {
	switch s {
	case StorageColumn:
		return "Column"
	case StorageTable:
		return "Table"
	default:
		return "Table"
	}
}

// CreateAssociationStmt represents: CREATE ASSOCIATION Module.Name FROM ... TO ... TYPE ...
type CreateAssociationStmt struct {
	Name           QualifiedName
	Parent         QualifiedName
	Child          QualifiedName
	Type           AssociationType
	Owner          OwnerType
	Storage        StorageType
	DeleteBehavior DeleteBehavior
	Documentation  string
	Comment        string
	CreateOrModify bool // true for CREATE OR MODIFY / CREATE OR REPLACE

	// Line anchors from `@anchor(from: (x, y), to: (x, y))` — where the
	// connector attaches to the FROM and TO entity boxes, as a PERCENTAGE of the
	// box (0..100). nil means the statement said nothing, which preserves an
	// existing anchor rather than resetting it. Reuses ast.Position because the
	// shape is the same; the unit is not (an entity's @position is canvas
	// pixels). (issue #872)
	FromAnchor *Position
	ToAnchor   *Position
}

func (s *CreateAssociationStmt) isStatement() {}

// DropAssociationStmt represents: DROP ASSOCIATION Module.Name
type DropAssociationStmt struct {
	Name QualifiedName
}

func (s *DropAssociationStmt) isStatement() {}

// AlterAssociationOperation represents the type of ALTER ASSOCIATION operation.
type AlterAssociationOperation int

const (
	AlterAssociationSetDeleteBehavior AlterAssociationOperation = iota
	AlterAssociationSetOwner
	AlterAssociationSetComment
	AlterAssociationSetStorage
	AlterAssociationSetAnchor
)

// AlterAssociationStmt represents: ALTER ASSOCIATION Module.Name SET ...
type AlterAssociationStmt struct {
	Name           QualifiedName
	Operation      AlterAssociationOperation
	DeleteBehavior DeleteBehavior
	Owner          OwnerType
	Storage        StorageType
	Comment        string

	// SET ANCHOR FROM (x, y) TO (x, y) — both ends are always given together,
	// because the pair is one visual decision.
	FromAnchor *Position
	ToAnchor   *Position
}

func (s *AlterAssociationStmt) isStatement() {}
