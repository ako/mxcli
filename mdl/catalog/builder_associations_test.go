// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"
)

func TestAssociationsTableQueryable(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer cat.Close()

	// Insert test data directly to verify table schema and query behavior.
	rows := []struct {
		id, name, qn, module, from, to, typ, owner, storage string
	}{
		{"id-1", "Order_Customer", "Sales.Order_Customer", "Sales",
			"Sales.Order", "Sales.Customer", "Reference", "Default", "Column"},
		{"id-2", "Customer_Tags", "Sales.Customer_Tags", "Sales",
			"Sales.Customer", "Sales.Tag", "ReferenceSet", "Both", "Table"},
	}
	for _, r := range rows {
		_, err := cat.CatalogDB().Exec(`
			INSERT INTO associations_data (Id, Name, QualifiedName, ModuleName,
				FromEntity, ToEntity, AssociationType, Owner, StorageFormat)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.id, r.name, r.qn, r.module, r.from, r.to, r.typ, r.owner, r.storage,
		)
		if err != nil {
			t.Fatalf("insert %s: %v", r.name, err)
		}
	}

	// Verify rows are queryable.
	result, err := cat.Query("SELECT * FROM associations WHERE ModuleName = 'Sales'")
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("expected 2 rows, got %d", result.Count)
	}

	// Filter by type.
	result, err = cat.Query("SELECT * FROM associations WHERE AssociationType = 'ReferenceSet'")
	if err != nil {
		t.Fatalf("Query by type: %v", err)
	}
	if result.Count != 1 {
		t.Errorf("expected 1 ReferenceSet, got %d", result.Count)
	}

	// Verify CATALOG.ASSOCIATIONS appears in Tables().
	found := false
	for _, tbl := range cat.Tables() {
		if tbl == "CATALOG.ASSOCIATIONS" {
			found = true
			break
		}
	}
	if !found {
		t.Error("CATALOG.ASSOCIATIONS not found in Tables()")
	}
}
