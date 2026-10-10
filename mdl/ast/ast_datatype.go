// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Data Types
// ============================================================================

// DataType represents an MDL attribute data type.
type DataType struct {
	Kind            DataTypeKind
	Length          int            // For String(length), -1 for unlimited
	Precision       int            // For Decimal(p,s)
	Scale           int            // For Decimal(p,s)
	EnumRef         *QualifiedName // For Enumeration(Module.EnumName)
	EntityRef       *QualifiedName // For Entity or List of Entity types
	TemplateContext string         // For StringTemplate(Sql), stores "Sql", "OQL", etc.
	TypeParamName   string         // For TypeEntityTypeParam: the declared name (e.g., "pEntity")
	// ExplicitEnum is true when the type was written with the unambiguous
	// `ENUM Module.Name` / `Enumeration(Module.Name)` syntax (vs. a bare
	// `Module.Name`, which the parser cannot tell apart from an entity and also
	// records as TypeEnumeration). Consumers that must serialize entity vs.
	// enumeration distinctly (e.g. Java/JavaScript action parameters, #680) use
	// this as the authoritative signal that the name is an enumeration.
	ExplicitEnum bool
	// Localize is a DateTime attribute's `localized` / `not localized`
	// constraint (DateTimeAttributeType.LocalizeDate, #1373). It is tri-state
	// because "not stated" is a third answer: a rewrite that does not state it
	// keeps the stored value (#743), while a stated one wins.
	Localize LocalizeSpec
}

// LocalizeSpec is what an attribute definition says about a DateTime's
// LocalizeDate.
type LocalizeSpec int

const (
	LocalizeUnstated     LocalizeSpec = iota // no clause: new attribute true, rewrite keeps stored
	LocalizeLocalized                        // `localized`: LocalizeDate = true
	LocalizeNotLocalized                     // `not localized`: LocalizeDate = false
)

// LocalizeDate resolves the spec for a newly built DateTime attribute: true
// unless `not localized` was stated. Mendix's default is true.
func (s LocalizeSpec) LocalizeDate() bool {
	return s != LocalizeNotLocalized
}

// DataTypeKind represents the kind of data type.
type DataTypeKind int

const (
	TypeUnknown DataTypeKind = iota // Unknown or unresolvable type
	TypeString
	TypeInteger
	TypeLong
	TypeDecimal
	TypeBoolean
	TypeDateTime
	TypeDate
	TypeAutoNumber
	TypeAutoOwner       // System.owner association (auto-set on create)
	TypeAutoChangedBy   // System.changedBy association (auto-set on commit)
	TypeAutoCreatedDate // CreatedDate DateTime (auto-set on create)
	TypeAutoChangedDate // ChangedDate DateTime (auto-set on commit)
	TypeBinary
	TypeEnumeration
	TypeEntity          // Entity reference (for microflow parameters)
	TypeListOf          // List of entity (for microflow parameters)
	TypeVoid            // Void return type (for microflows)
	TypeStringTemplate  // StringTemplate(Sql) etc. for Java actions
	TypeEntityTypeParam // ENTITY <pEntity> type parameter declaration for Java actions
	// TypeHashedString is DomainModels$HashedStringAttributeType: an entity
	// attribute type only. No microflow, constant or Java action type exists
	// for it, so the visitor refuses it everywhere but an attribute definition.
	// Appended last so no existing kind's value moves.
	TypeHashedString
)

func (k DataTypeKind) String() string {
	switch k {
	case TypeString:
		return "String"
	case TypeInteger:
		return "Integer"
	case TypeLong:
		return "Long"
	case TypeDecimal:
		return "Decimal"
	case TypeBoolean:
		return "Boolean"
	case TypeDateTime:
		return "DateTime"
	case TypeDate:
		return "Date"
	case TypeAutoNumber:
		return "AutoNumber"
	case TypeAutoOwner:
		return "AutoOwner"
	case TypeAutoChangedBy:
		return "AutoChangedBy"
	case TypeAutoCreatedDate:
		return "AutoCreatedDate"
	case TypeAutoChangedDate:
		return "AutoChangedDate"
	case TypeBinary:
		return "Binary"
	case TypeEnumeration:
		return "Enumeration"
	case TypeEntity:
		return "Entity"
	case TypeListOf:
		return "List"
	case TypeVoid:
		return "Void"
	case TypeStringTemplate:
		return "StringTemplate"
	case TypeEntityTypeParam:
		return "EntityTypeParam"
	case TypeHashedString:
		return "HashedString"
	default:
		return "Unknown"
	}
}
