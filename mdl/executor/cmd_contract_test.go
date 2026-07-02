// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestAttrNameForOData_ReservedWords verifies that Mendix-reserved attribute
// names are prefixed with the entity name so Studio Pro does not reject them.
// Regression test for issue #526.
func TestAttrNameForOData_ReservedWords(t *testing.T) {
	cases := []struct {
		prop   string
		entity string
		want   string
	}{
		// Already-covered names
		{"Id", "Photo", "PhotoId"},
		{"id", "Photo", "Photoid"},
		{"Name", "Airline", "AirlineName"},
		{"name", "Airline", "Airlinename"},
		// Newly-added reserved names (issue #526)
		{"Owner", "Trip", "TripOwner"},
		{"owner", "Trip", "Tripowner"},
		{"Type", "Flight", "FlightType"},
		{"type", "Flight", "Flighttype"},
		{"Context", "Person", "PersonContext"},
		{"context", "Person", "Personcontext"},
		{"ChangedBy", "Event", "EventChangedBy"},
		{"changedby", "Event", "Eventchangedby"},
		{"ChangedDate", "Event", "EventChangedDate"},
		{"changeddate", "Event", "Eventchangeddate"},
		{"CreatedDate", "Event", "EventCreatedDate"},
		{"createddate", "Event", "Eventcreateddate"},
		// Non-reserved names must pass through unchanged
		{"AirlineCode", "Airline", "AirlineCode"},
		{"Concurrency", "Airline", "Concurrency"},
		{"FirstName", "Person", "FirstName"},
	}

	for _, tc := range cases {
		got := attrNameForOData(tc.prop, tc.entity)
		if got != tc.want {
			t.Errorf("attrNameForOData(%q, %q) = %q; want %q", tc.prop, tc.entity, got, tc.want)
		}
	}
}

// TestApplyExternalEntityFields_ConservativeCapabilityDefault guards the OData
// capability defaulting: an entity set with no InsertRestrictions/
// DeleteRestrictions annotation must import as Creatable/Deletable = false,
// matching Mendix's conservative read-only default. Verified against the
// TripPin RESTier service (zero capability annotations, entities Creatable=False
// per mx check). An earlier permissive default caused CE6630 regressions.
func TestApplyExternalEntityFields_ConservativeCapabilityDefault(t *testing.T) {
	ent := &domainmodel.Entity{}
	et := &types.EdmEntityType{Name: "Person"}
	// entitySet with no Insertable/Deletable annotation (nil) — the TripPin case.
	es := &types.EdmEntitySet{Name: "People"}

	applyExternalEntityFields(ent, et, true /*isTopLevel*/, "Svc.TripPin", es, nil, nil)

	if ent.Creatable {
		t.Error("Creatable = true, want false (absent InsertRestrictions ⇒ conservative read-only)")
	}
	if ent.Deletable {
		t.Error("Deletable = true, want false (absent DeleteRestrictions ⇒ conservative read-only)")
	}

	// An explicit annotation must turn the capability on.
	on := true
	es2 := &types.EdmEntitySet{Name: "People", Insertable: &on}
	ent2 := &domainmodel.Entity{}
	applyExternalEntityFields(ent2, et, true, "Svc.TripPin", es2, nil, nil)
	if !ent2.Creatable {
		t.Error("Creatable = false, want true (explicit InsertRestrictions=true)")
	}
}
