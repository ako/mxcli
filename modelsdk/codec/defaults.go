// SPDX-License-Identifier: Apache-2.0

package codec

import "github.com/mendixlabs/mxcli/modelsdk/canon"

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

// RegisterListMarker declares the typed-array marker for PartLists whose child
// elements are of childType. Without registration the encoder emits 3. Most
// domain-model member lists use 3; some — notably the IndexedAttribute list
// inside an EntityIndex — use a different version marker. Confirmed against
// real Studio-Pro 11.x BSON (mx-test-projects/test7-app: IdxProbe index uses 2).
//
// The table lives in canon (canon.RegisterChildListMarker), beside the
// owner/key table every write applies.
func RegisterListMarker(childType string, marker int32) {
	canon.RegisterChildListMarker(childType, marker)
}

// RegisterPropertyListMarker declares the typed-array marker for the PartList
// stored under key on elements of ownerType. It takes precedence over a
// child-type RegisterListMarker and also covers the list when it is empty. It
// exists for lists whose marker depends on WHERE the list sits rather than on
// what it holds: a Forms$DesignPropertyValue list is marker 3 as a
// Forms$Appearance's DesignProperties and marker 2 as a
// Forms$CompoundDesignPropertyValue's Properties (measured: 1821/1821 and
// 373/373 across Studio Pro-authored documents in a Mendix 11.13.0 app), so a
// RegisterListMarker on the child type cannot express it.
//
// Register only a measured marker: every write applies it to the stored lists
// the encoder does not rebuild too (canon.NormalizeListMarkers,
// ako/mxcli#1640), where a wrong one would rewrite what Studio Pro stored.
func RegisterPropertyListMarker(ownerType, key string, marker int32) {
	canon.RegisterPropertyListMarker(ownerType, key, marker)
}

// mandatoryPropertyLists marks the owner/key pairs Studio Pro serializes on
// every element of the owner type, empty or not. RegisterMandatoryPropertyList
// fills it; the encoder writes such a list as bson.A{marker} on a new element
// whose list is empty, where it would otherwise omit the key.
var mandatoryPropertyLists = map[string]map[string]bool{}

// RegisterMandatoryPropertyList is RegisterPropertyListMarker for a PartList
// Studio Pro always writes: the marker applies as there, and a new element of
// ownerType with nothing in the list still writes the key as the bare marker.
// It is keyed on the owner, so it never adds the key to another type, and it
// merges with — rather than replaces — the owner's RegisterTypeDefaults.
func RegisterMandatoryPropertyList(ownerType, key string, marker int32) {
	RegisterPropertyListMarker(ownerType, key, marker)
	if mandatoryPropertyLists[ownerType] == nil {
		mandatoryPropertyLists[ownerType] = map[string]bool{}
	}
	mandatoryPropertyLists[ownerType][key] = true
}

func isMandatoryPropertyList(ownerType, key string) bool {
	return mandatoryPropertyLists[ownerType][key]
}

func lookupPropertyListMarker(ownerType, key string) (int32, bool) {
	return canon.PropertyListMarker(ownerType, key)
}

func lookupListMarker(childType string) int32 {
	if m, ok := canon.ChildListMarker(childType); ok {
		return m
	}
	return 3
}
