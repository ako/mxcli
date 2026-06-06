// SPDX-License-Identifier: Apache-2.0

package codec

// TypeDefaults captures the serialization defaults Studio Pro applies to a
// freshly-created element of a given $Type but that the gen New<Type>()
// constructors do not yet set — the "applyDefaults" gap (engalar Fix 4 TODO).
//
// Confirmed against real Studio-Pro BSON (mx-test-projects/test7-app): a created
// entity always carries a GUID (= its own $ID, as binary) and its member
// collections (Attributes/AccessRules/…), which Studio Pro emits even when empty
// as the typed-array marker [3]. The encoder consults this registry only for
// new elements (raw == nil), keyed by BSON $Type.
//
// This is a hand-maintained registry standing in for codegen-emitted defaults
// from reflection-data; it is consulted only for types explicitly registered, so
// it cannot affect serialization of any other element.
type TypeDefaults struct {
	// EmitGUID adds {GUID: <$ID as binary subtype 0>} (domain-model elements).
	EmitGUID bool
	// MandatoryLists are PartList BSON keys Studio Pro always serializes; the
	// encoder emits each (empty) as the typed-array marker when not otherwise set.
	MandatoryLists []string
}

var registeredDefaults = map[string]TypeDefaults{}

// RegisterTypeDefaults declares the applyDefaults for a $Type. Call from an
// init() in the layer that constructs these elements.
func RegisterTypeDefaults(typeName string, d TypeDefaults) {
	registeredDefaults[typeName] = d
}

func lookupTypeDefaults(typeName string) (TypeDefaults, bool) {
	d, ok := registeredDefaults[typeName]
	return d, ok
}
