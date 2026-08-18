// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// validateAttributeTypeRef rejects an attribute type that names neither a
// primitive, a known enumeration, nor a known entity.
//
// The visitor maps every bare qualified name to TypeEnumeration (the
// TypeEnumeration/TypeEntity ambiguity documented in CLAUDE.md), so an
// unrecognised word arrives here looking like an enumeration reference. Writing
// it produces an EnumerationAttributeType whose enumeration does not exist, and
// Mendix cannot load the result at all — `mx check` dies before any validation
// runs:
//
//	System.ArgumentNullException: Value cannot be null. (Parameter 'value')
//	  at EnumerationAttributeType.set_EnumerationId
//
// This lived inline in CREATE ENTITY (added for #552) and was never applied to
// ALTER ENTITY … MODIFY ATTRIBUTE, so the same typo corrupted a project through
// the modify path while the create path rejected it cleanly. It is shared here
// so a third caller cannot repeat that.
func validateAttributeTypeRef(ctx *ExecContext, attrName string, dt ast.DataType) error {
	if dt.Kind != ast.TypeEnumeration || dt.EnumRef == nil {
		return nil
	}
	refModule := dt.EnumRef.Module
	refName := dt.EnumRef.Name
	if findEnumeration(ctx, refModule, refName) != nil {
		return nil
	}
	if _, err := findEntity(ctx, refModule, refName); err == nil {
		return nil
	}
	return mdlerrors.NewValidationf(
		"attribute '%s': unknown type '%s' — not a primitive, enumeration, or entity",
		attrName, dt.EnumRef.String())
}

// validateModifyAttributeTypeRef is validateAttributeTypeRef with the hint that
// only MODIFY ATTRIBUTE needs.
//
// MODIFY ATTRIBUTE requires a type — the grammar has no form without one — and
// its last dataType alternative is a bare qualifiedName. So a user reaching for
// something the syntax does not have writes a statement whose first word lands
// in the type position:
//
//	ALTER ENTITY M.E MODIFY ATTRIBUTE UserID SET DEFAULT NULL
//	                                         ^^^ parsed as the type
//
// The bare "unknown type 'set'" is accurate but unhelpful there, because the
// user was not trying to name a type at all. Naming DROP DEFAULT turns the
// refusal into the answer (mendixlabs/mxcli#910).
func validateModifyAttributeTypeRef(ctx *ExecContext, attrName string, dt ast.DataType) error {
	err := validateAttributeTypeRef(ctx, attrName, dt)
	if err == nil {
		return nil
	}
	return mdlerrors.NewValidationf(
		"%s\n"+
			"  MODIFY ATTRIBUTE always takes a type, so a clause it does not recognise is read as one.\n"+
			"  To clear a default value:      ALTER ENTITY … DROP DEFAULT ON ATTRIBUTE %s;\n"+
			"  To change the type:            ALTER ENTITY … MODIFY ATTRIBUTE %s <Type>;",
		err.Error(), attrName, attrName)
}
