// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Mendix stores Owner / ChangedBy / CreatedDate / ChangedDate as BOOLEANS on
// the entity's generalization node, not as attributes, so they never appear in
// CATALOG.ATTRIBUTES. Before these columns existed, the obvious query for
// "which entities lack an audit trail" --
//
//	SELECT e.QualifiedName FROM CATALOG.ENTITIES e
//	  LEFT JOIN CATALOG.ATTRIBUTES a
//	    ON a.EntityQualifiedName = e.QualifiedName AND a.Name = 'CreatedDate'
//	 WHERE a.Name IS NULL
//
// returned every entity forever, including ones that already had the field.
// DESCRIBE ENTITY renders them in the attribute list, which is what makes the
// omission surprising rather than obviously by-design.
func TestEntitiesCarryTheAuditMembers(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()

	const modID = model.ID("mod-sales")
	b := &Builder{
		catalog:   cat,
		snapshot:  &Snapshot{ID: "snap"},
		hierarchy: &hierarchy{moduleIDs: map[model.ID]bool{modID: true}, moduleNames: map[model.ID]string{modID: "Sales"}},
		domainModelCache: []*domainmodel.DomainModel{{
			ContainerID: modID,
			Entities: []*domainmodel.Entity{
				{
					BaseElement: model.BaseElement{ID: "e-audited"},
					Name:        "Order", Persistable: true,
					HasOwner: true, HasChangedBy: true,
					HasCreatedDate: true, HasChangedDate: true,
				},
				{
					BaseElement: model.BaseElement{ID: "e-bare"},
					Name:        "Lookup", Persistable: true,
				},
			},
		}},
	}

	tx, err := cat.CatalogDB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	b.tx = tx
	if err := b.buildEntities(); err != nil {
		t.Fatalf("buildEntities: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	res, err := cat.Query(`SELECT QualifiedName, HasCreatedDate, HasChangedDate,
		HasOwner, HasChangedBy FROM entities_data ORDER BY QualifiedName`)
	if err != nil {
		t.Fatalf("query: %v -- the columns must exist, or the catalog cannot answer this at all", err)
	}
	if res.Count != 2 {
		t.Fatalf("got %d rows, want 2", res.Count)
	}

	got := map[string][4]int64{}
	for _, row := range res.Rows {
		qn, _ := row[0].(string)
		var flags [4]int64
		for i := 0; i < 4; i++ {
			flags[i], _ = row[i+1].(int64)
		}
		got[qn] = flags
	}

	if want := [4]int64{1, 1, 1, 1}; got["Sales.Order"] != want {
		t.Errorf("Sales.Order audit flags = %v, want %v -- the entity's booleans "+
			"are not reaching the row", got["Sales.Order"], want)
	}
	if want := [4]int64{0, 0, 0, 0}; got["Sales.Lookup"] != want {
		t.Errorf("Sales.Lookup audit flags = %v, want %v -- an entity without "+
			"audit members must not claim them", got["Sales.Lookup"], want)
	}
}
