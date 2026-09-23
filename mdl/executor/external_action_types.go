// SPDX-License-Identifier: Apache-2.0

// Classification of the types an external action's parameters and return value
// can have, and the refusal that follows from it.
//
// This is the third bug reported as CE7252 (mendixlabs/mxcli#1020, #1073,
// #1089) and the first whose answer is partly "you cannot". Every one of them
// arrived as "the stored fingerprint is stale, give me a command to refresh
// it". There is no fingerprint: Mendix re-derives the alignment from the
// consumed service's cached contract on every build, so a call it rejects is a
// call that was written wrong — or one the contract makes impossible.
//
// Measured on mxbuild 11.12.0, one action per shape, each written by mxcli and
// checked on its own (the probe contract is reproduced in the test file):
//
//	Edm.String Guid Boolean Byte SByte Int16 Int32 Int64      0 errors
//	Edm.Decimal Double Single Date DateTime DateTimeOffset    0 errors
//	an EnumType, an EntityType, Collection(EntityType)        0 errors
//	Edm.TimeOfDay                                             CE7252 / CE7269
//	Edm.Duration Stream Binary Geography*                     CE7255 (+CE7253)
//	a ComplexType, a TypeDefinition, Collection(Edm.*)        CE7255 (+CE7253)
//
// Two facts drive everything below, and neither is guessable:
//
//  1. Edm.TimeOfDay is SUPPORTED. It is the only type that failed without
//     Mendix also calling it unsupported, which is what identifies it as our
//     gap rather than the platform's. Typing it as DateTime clears both codes.
//
//  2. An ENUM-typed parameter builds with no type written at all. So "mxcli
//     could not name a Mendix type for it" is not on its own grounds to refuse
//     — lumping enums in with complex types would refuse a call that builds.
//
// What Mendix will not take, it states as CE7255 "Action '<x>' of service
// '<svc>' is not supported", with CE7252/CE7269 riding along. No BSON mxcli can
// write changes that, so the statement is refused here instead of executing and
// leaving an unbuildable project with no way back.
package executor

import (
	"fmt"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// externalActionTypeClass is what mxcli can do with one parameter or return type.
type externalActionTypeClass int

const (
	// extTypeRepresentable: the call can be written. kind names the Mendix type
	// for a primitive, and is empty for an enum, which needs none.
	extTypeRepresentable externalActionTypeClass = iota
	// extTypeEntity: an entity type. Whether the call can be written depends on
	// the PROJECT — the external entity must have been imported — so this class
	// is resolved by the caller, not here.
	extTypeEntity
	// extTypeUnsupported: Mendix refuses the action itself (CE7255). Nothing
	// mxcli writes can change that.
	extTypeUnsupported
)

// externalActionType is one classified parameter or return type.
type externalActionType struct {
	class  externalActionTypeClass
	kind   string // Mendix kind ("String", "DateTime", …); empty for enum/entity
	entity string // bare entity type name, for extTypeEntity
	isList bool   // the contract said Collection(...)
}

// classifyExternalActionType decides what an action's parameter or return type
// is, against the contract it was declared in.
//
// The contract is needed because the EDM name alone does not say which of the
// three a `Probe.Thing` is: an entity type (representable), an enum
// (representable, and needs no type written) or a complex type / type
// definition (which Mendix refuses outright). A name the contract does not
// declare at all is refused for the same reason it is not resolvable.
func classifyExternalActionType(doc *types.EdmxDocument, edmType string) externalActionType {
	if kind := edmReturnTypeToKind(edmType); kind != "" {
		return externalActionType{class: extTypeRepresentable, kind: kind}
	}

	bare, isList := edmBareTypeName(edmType)
	if bare == "" {
		// An Edm.* primitive that the table above does not map, or a collection
		// of one. Both are CE7255; Collection(Edm.String) is measured.
		return externalActionType{class: extTypeUnsupported}
	}
	if doc != nil {
		if doc.FindEntityType(bare) != nil {
			return externalActionType{class: extTypeEntity, entity: bare, isList: isList}
		}
		if doc.FindEnumType(bare) != nil && !isList {
			// Collection(enum) is deliberately NOT included: every collection of
			// a non-entity type measured as CE7255, and nothing here was
			// measured for the enum case, so it is refused rather than assumed.
			return externalActionType{class: extTypeRepresentable}
		}
	}
	return externalActionType{class: extTypeUnsupported}
}

// flowPrefix names the flow the call sits in, when the caller knows it. The
// writer does not — its errors are already reported under the statement — so
// the same message serves both callers rather than a second copy in the
// builder's currency. Two copies of a rule in two currencies is how a resolver
// drifts, which is the reason this is one function and not two.
func (c externalCall) flowPrefix() string {
	if c.flow == "" {
		return ""
	}
	return c.flow + ": "
}

// checkExternalActionTypes refuses a call whose parameter or return types the
// contract makes unwritable, and says which of the two kinds of unwritable it is.
//
// importedEntity maps a remote type name onto the qualified name of the external
// entity imported for it, or "" when none has been. It is a function so the rule
// can be tested without a project.
//
// A BOUND action's first parameter is its binding parameter, supplied by Mendix
// from the object the action is called on. It is skipped for the same reason
// checkExternalActionParameters skips it: it is not the statement's to supply,
// so it is not the statement's to be refused over.
func checkExternalActionTypes(
	c externalCall,
	action *types.EdmAction,
	doc *types.EdmxDocument,
	svcQN string,
	importedEntity func(remoteName string) string,
) error {
	for i, p := range action.Parameters {
		if action.IsBound && i == 0 {
			continue
		}
		t := classifyExternalActionType(doc, p.Type)
		switch t.class {
		case extTypeUnsupported:
			return unsupportedExternalActionType(c, action, svcQN,
				fmt.Sprintf("parameter %q is %s", p.Name, p.Type),
				"One of the parameters is not supported")
		case extTypeEntity:
			if importedEntity(t.entity) != "" {
				continue
			}
			return mdlerrors.NewValidation(fmt.Sprintf(
				"%sexternal action %q takes parameter %q of %s, but no external entity has been "+
					"imported for that type, so the argument cannot be typed.\n"+
					"  Mendix reports this as CE7252 \"The parameters for remote action '%s' have changed\".\n"+
					"  Import it first: create or modify external entities from %s entities (%s)",
				c.flowPrefix(), action.Name, p.Name, p.Type, action.Name, svcQN, t.entity))
		}
	}

	if t := classifyExternalActionType(doc, action.ReturnType); t.class == extTypeUnsupported {
		return unsupportedExternalActionType(c, action, svcQN,
			fmt.Sprintf("it returns %s", action.ReturnType),
			"This action's return type is not supported")
	}
	// An entity-typed return with no imported entity is reported by
	// checkExternalActionReturn, which already names the import statement.
	return nil
}

// unsupportedExternalActionType is the message for the case with no remedy. It
// says so outright: the report this exists to answer spent its effort looking
// for the mxcli command that would clear the error, and the useful answer is
// that the action is not callable from a microflow at all.
func unsupportedExternalActionType(c externalCall, action *types.EdmAction, svcQN, detail, mendixReason string) error {
	return mdlerrors.NewValidation(fmt.Sprintf(
		"%sexternal action %q cannot be called from a microflow — %s, which Mendix does not support "+
			"on a call external action.\n"+
			"  Mendix reports this as CE7255 \"Action '%s' of service '%s' is not supported. %s\" "+
			"(with CE7253 on the parameter, and CE7252/CE7269 on the call itself).\n"+
			"  No MDL clears it: the limitation is the service contract's, not the call's. Use a "+
			"different action, or reach the operation over a REST call instead.",
		c.flowPrefix(), action.Name, detail, action.Name, svcQN, mendixReason))
}
