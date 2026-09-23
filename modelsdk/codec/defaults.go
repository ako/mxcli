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
	// encoder emits each (empty) as the typed-array marker [3] when not otherwise set.
	MandatoryLists []string
	// MandatoryListMarkers are mandatory PartList keys whose empty form uses a
	// non-default marker (e.g. a SortingsList's "Sortings" empties as [2]). Emitted
	// as bson.A{marker} when not otherwise set.
	MandatoryListMarkers map[string]int32
	// NullFields are keys Studio Pro always serializes as BSON null (e.g. an
	// unset reference like an association's Source); emitted when not otherwise set.
	NullFields []string
	// EmptyStringFields are keys Studio Pro always serializes as the empty string ""
	// (not null) when unset — e.g. a ConditionalVisibility/EditabilitySettings'
	// Attribute (a BY_NAME AttributeIdentifier). Mendix 11.12's reader rejects a
	// null there. Emitted as "" when not otherwise set.
	EmptyStringFields []string
	// ZeroGUIDFields are reference keys Studio Pro serializes as an all-zero GUID
	// binary when the reference is unset (e.g. an IndexedAttribute's
	// AssociationPointer on an attribute-based index segment). Emitted when not
	// otherwise set. Stands in for a gen property the constructor doesn't expose.
	ZeroGUIDFields []string
	// FalseFields are keys Studio Pro always serializes as boolean false when
	// unset. Go's zero value for a bool is false either way, so the property is
	// never marked dirty and the encoder would otherwise omit the key — which
	// is a difference from the stored document even though nothing about the
	// model changed (ako/mxcli#541, Forms$PageVariable.UseAllPages).
	FalseFields []string
	// FreshGUIDFields are keys Studio Pro serializes as a fresh random GUID binary
	// (subtype 0), e.g. a microflow's StableId. Emitted when not otherwise set.
	// Stands in for a gen property mistyped as a string. The value is opaque to
	// Studio Pro (and masked in canonical comparison), so a fresh GUID suffices.
	FreshGUIDFields []string
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

// FreshGUIDRegistrations returns every registered FreshGUIDFields entry, keyed by
// $Type. Exported for the drift guard in the backend package: a property the
// encoder mints fresh on every write is, by construction, a property that makes a
// document differ from itself, so each one has to be a deliberate decision about
// whether it is identity (carry it) or content (churn it). Nothing else should
// need this.
func FreshGUIDRegistrations() map[string][]string {
	out := make(map[string][]string)
	for typeName, d := range registeredDefaults {
		if len(d.FreshGUIDFields) > 0 {
			out[typeName] = append([]string(nil), d.FreshGUIDFields...)
		}
	}
	return out
}

// listMarkers maps a child element $Type to the leading typed-array marker
// Studio Pro writes for a PartList of that element type. Most domain-model
// member lists use 3 (the encoder default); some — notably the IndexedAttribute
// list inside an EntityIndex — use a different version marker. Confirmed against
// real Studio-Pro 11.x BSON (mx-test-projects/test7-app: IdxProbe index uses 2).
var listMarkers = map[string]int32{}

// RegisterListMarker declares the typed-array marker for PartLists whose child
// elements are of childType. Without registration the encoder emits 3.
func RegisterListMarker(childType string, marker int32) {
	listMarkers[childType] = marker
}

func lookupListMarker(childType string) int32 {
	if m, ok := listMarkers[childType]; ok {
		return m
	}
	return 3
}
