// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// TestIsODataEntitySource pins which entity Sources count as external.
//
// Mendix stores two, and only the entity-set-backed one was accepted. The other
// covers derived, abstract and contained types — and an OData action's
// parameter and return types, which have no entity set of their own.
func TestIsODataEntitySource(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"Rest$ODataRemoteEntitySource", true},
		// The one that was missing. CREATE EXTERNAL ENTITIES writes this for
		// every type with no entity set (applyExternalEntityFields).
		{"Rest$ODataEntityTypeSource", true},
		{"", false},
		{"Rest$ConsumedODataService", false},
		{"DomainModels$Entity", false},
	}
	for _, tt := range tests {
		if got := isODataEntitySource(tt.source); got != tt.want {
			t.Errorf("isODataEntitySource(%q) = %v, want %v", tt.source, got, tt.want)
		}
	}
}

// TestContractEntityUsageLinksTypeSourcedEntities is the reason the predicate
// matters, exercised through the real join.
//
// contract_entities.UsedByExternalEntity is filled by matching external_entities
// on RemoteName. While that table held only entity-set-backed entities, the
// column was structurally always empty for an action's parameter/return types —
// it read as "this contract entity is linked to nothing" whether or not the
// import had worked, which is what sent mendixlabs/mxcli#1020 hunting for an MPR
// linkage bug that did not exist.
//
// The two rows are the before and after of the catalog fix: Person is
// entity-set-backed and was always catalogued; Airport is an action's return
// type and is catalogued only now.
func TestContractEntityUsageLinksTypeSourcedEntities(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(`
		INSERT INTO odata_clients_data (Id, Name, QualifiedName, ModuleName)
		VALUES ('svc-1', 'TripPin', 'Ext.TripPin', 'Ext')`); err != nil {
		t.Fatalf("seed odata_clients: %v", err)
	}

	// Contract entities: one entity-set-backed, one that exists only as an
	// action's return type.
	for _, r := range []struct{ id, name, set string }{
		{"ce-person", "Person", "People"},
		{"ce-airport", "Airport", ""},
	} {
		if _, err := db.Exec(`
			INSERT INTO contract_entities_data (Id, ServiceId, ServiceQualifiedName, EntityName, EntitySetName)
			VALUES (?, 'svc-1', 'Ext.TripPin', ?, ?)`, r.id, r.name, r.set); err != nil {
			t.Fatalf("seed contract entity %s: %v", r.name, err)
		}
	}

	// External entities, as the fixed builder catalogues them: both sources.
	for _, r := range []struct{ id, name, qn, set, remote string }{
		{"ee-person", "People", "Ext.People", "People", "Person"},
		{"ee-airport", "Airport", "Ext.Airport", "", "Airport"},
	} {
		if _, err := db.Exec(`
			INSERT INTO external_entities_data (Id, Name, QualifiedName, ModuleName, ServiceName, EntitySet, RemoteName)
			VALUES (?, ?, ?, 'Ext', 'Ext.TripPin', ?, ?)`,
			r.id, r.name, r.qn, r.set, r.remote); err != nil {
			t.Fatalf("seed external entity %s: %v", r.name, err)
		}
	}

	if _, err := db.Exec(contractEntityUsageSQL); err != nil {
		t.Fatalf("run usage join: %v", err)
	}

	for _, tt := range []struct{ id, want string }{
		{"ce-person", "Ext.People"},
		// The row that was always empty before.
		{"ce-airport", "Ext.Airport"},
	} {
		var got *string
		if err := db.QueryRow(
			`SELECT UsedByExternalEntity FROM contract_entities WHERE Id = ?`, tt.id,
		).Scan(&got); err != nil {
			t.Fatalf("read %s: %v", tt.id, err)
		}
		if got == nil {
			t.Errorf("%s: UsedByExternalEntity is NULL, want %q", tt.id, tt.want)
			continue
		}
		if *got != tt.want {
			t.Errorf("%s: UsedByExternalEntity = %q, want %q", tt.id, *got, tt.want)
		}
	}
}

// TestContractEntityUsageIsEmptyWhenNotImported is the control: the column is
// only meaningful because an absent import really does leave it empty. Without
// this, the test above would pass against a join that populated every row.
func TestContractEntityUsageIsEmptyWhenNotImported(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(`
		INSERT INTO odata_clients_data (Id, Name, QualifiedName, ModuleName)
		VALUES ('svc-1', 'TripPin', 'Ext.TripPin', 'Ext')`); err != nil {
		t.Fatalf("seed odata_clients: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO contract_entities_data (Id, ServiceId, ServiceQualifiedName, EntityName)
		VALUES ('ce-airport', 'svc-1', 'Ext.TripPin', 'Airport')`); err != nil {
		t.Fatalf("seed contract entity: %v", err)
	}
	// No external entity imported for it.

	if _, err := db.Exec(contractEntityUsageSQL); err != nil {
		t.Fatalf("run usage join: %v", err)
	}

	var got *string
	if err := db.QueryRow(
		`SELECT UsedByExternalEntity FROM contract_entities WHERE Id = 'ce-airport'`,
	).Scan(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got != nil && *got != "" {
		t.Errorf("UsedByExternalEntity = %q, want empty — an unimported type must still read as unlinked", *got)
	}
}
