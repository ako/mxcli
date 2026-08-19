// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// ---------------------------------------------------------------------------
// #935: the entity-context walk stopped at a pluggable widget's own widget
// properties, so a widget sitting inside a DataGrid2 **column** reported no
// enclosing entity. The executor then built its bindings with an empty entity
// context, and an association-navigating ContentParams path was stored verbatim
// as the attribute name (CE1613) instead of resolving into AttributeRef +
// EntityRef steps.
//
// The same walk also never read a **pluggable** widget's datasource, so even a
// widget reached through the grid's own `content`-style property inherited the
// page's (empty) context rather than the grid's entity.
// ---------------------------------------------------------------------------

// buildDataGridWithCustomContentColumn builds the minimal DataGrid2 BSON the
// entity walk has to traverse: a `datasource` property naming the bound entity,
// and a `columns` property whose single column carries its cell widgets under
// `content` — one level deeper than the grid's own widget properties.
//
// entityRef is spliced in as the datasource's EntityRef so one builder serves
// both the database (DirectEntityRef) and association (IndirectEntityRef) cases.
func buildDataGridWithCustomContentColumn(entityRef bson.D, cellWidgets bson.A) bson.D {
	dsID := idBin(0x20)
	colsID := idBin(0x21)
	contentID := idBin(0x22)

	column := bson.D{
		{Key: "$Type", Value: objectListItemType},
		{Key: "Properties", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "TypePointer", Value: contentID},
				{Key: "Value", Value: bson.D{{Key: "Widgets", Value: cellWidgets}}},
			},
		}},
	}

	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "grid1"},
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{{Key: "$ID", Value: dsID}, {Key: "PropertyKey", Value: "datasource"}},
					bson.D{
						{Key: "$ID", Value: colsID},
						{Key: "PropertyKey", Value: "columns"},
						{Key: "ValueType", Value: bson.D{
							{Key: "ObjectType", Value: bson.D{
								{Key: "PropertyTypes", Value: bson.A{
									int32(2),
									bson.D{{Key: "$ID", Value: contentID}, {Key: "PropertyKey", Value: "content"}},
								}},
							}},
						}},
					},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: dsID},
					{Key: "Value", Value: bson.D{
						{Key: "DataSource", Value: bson.D{{Key: "EntityRef", Value: entityRef}}},
					}},
				},
				bson.D{
					{Key: "TypePointer", Value: colsID},
					{Key: "Value", Value: bson.D{{Key: "Objects", Value: bson.A{int32(2), column}}}},
				},
			}},
		}},
	}
}

func directEntityRef(entity string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "DomainModels$DirectEntityRef"},
		{Key: "Entity", Value: entity},
	}
}

func indirectEntityRef(association, destination string) bson.D {
	return bson.D{
		{Key: "$Type", Value: "DomainModels$IndirectEntityRef"},
		{Key: "Steps", Value: bson.A{
			int32(2),
			bson.D{
				{Key: "$Type", Value: "DomainModels$EntityRefStep"},
				{Key: "Association", Value: association},
				{Key: "DestinationEntity", Value: destination},
			},
		}},
	}
}

// cellContainer is the customContent cell: a plain container holding one text.
func cellContainer() bson.A {
	return bson.A{
		int32(2),
		bson.D{
			{Key: "$Type", Value: "Forms$DivContainer"},
			{Key: "Name", Value: "ctnCust"},
			{Key: "Widgets", Value: bson.A{
				int32(2),
				bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "txtCust"}},
			}},
		},
	}
}

// pageWith wraps widgets the way a real page stores them — inside the layout
// call's placeholder arguments — so both the widget finder and the entity walk
// start from the same shape they meet in a project.
func pageWith(widgets ...bson.D) bson.D {
	arr := bson.A{int32(2)}
	for _, w := range widgets {
		arr = append(arr, w)
	}
	return bson.D{
		{Key: "FormCall", Value: bson.D{
			{Key: "Arguments", Value: bson.A{
				int32(2),
				bson.D{{Key: "Widgets", Value: arr}},
			}},
		}},
	}
}

// A widget inside a customContent column must see the grid's entity. Before the
// fix this returned "", and `insert into ctnCust { dynamictext … contentparams:
// [{1} = A/B/Attr] }` wrote the raw path as the attribute name (CE1613).
func TestEnclosingEntity_InsideCustomContentColumn(t *testing.T) {
	grid := buildDataGridWithCustomContentColumn(directEntityRef("Sample.Order"), cellContainer())
	m := &Mutator{rawData: pageWith(grid), widgetFinder: findBsonWidget}

	if got := m.EnclosingEntity("ctnCust"); got != "Sample.Order" {
		t.Errorf("EnclosingEntity(ctnCust) = %q, want %q", got, "Sample.Order")
	}
	if got := m.EnclosingEntity("txtCust"); got != "Sample.Order" {
		t.Errorf("EnclosingEntity(txtCust) = %q, want %q", got, "Sample.Order")
	}
}

// INSERT INTO takes the target's own children context, which falls through to
// the same walk once the container declares no source of its own. This is the
// path the reported statement used, so it is asserted separately from the
// sibling (INSERT BEFORE/AFTER) path above.
func TestEnclosingEntityForChildren_InsideCustomContentColumn(t *testing.T) {
	grid := buildDataGridWithCustomContentColumn(directEntityRef("Sample.Order"), cellContainer())
	m := &Mutator{rawData: pageWith(grid), widgetFinder: findBsonWidget}

	if got := m.EnclosingEntityForChildren("ctnCust"); got != "Sample.Order" {
		t.Errorf("EnclosingEntityForChildren(ctnCust) = %q, want %q", got, "Sample.Order")
	}
}

// A pluggable widget bound `from association` stores its entity on the last
// EntityRefStep, exactly as a plain Forms$ widget does — the pluggable reader
// only handled the direct form, so an association-bound grid reported no entity
// at all (the FINDINGS #55 failure, one level in).
func TestEnclosingEntity_PluggableAssociationSource(t *testing.T) {
	grid := buildDataGridWithCustomContentColumn(
		indirectEntityRef("Sample.Customer_Order", "Sample.Order"), cellContainer())
	m := &Mutator{rawData: pageWith(grid), widgetFinder: findBsonWidget}

	if got := m.EnclosingEntity("ctnCust"); got != "Sample.Order" {
		t.Errorf("EnclosingEntity(ctnCust) = %q, want %q", got, "Sample.Order")
	}
}

// The grid's own widget-valued properties (a Gallery's `content`, a DataGrid2
// filter slot) were already reachable, but inherited the PAGE's context rather
// than the grid's — the same missing pluggable-datasource read. Control for the
// descent fix: this one needs only the datasource half.
func TestEnclosingEntity_PluggableOwnWidgetProperty(t *testing.T) {
	dsID := idBin(0x30)
	contentID := idBin(0x31)
	grid := bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "gallery1"},
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{{Key: "$ID", Value: dsID}, {Key: "PropertyKey", Value: "datasource"}},
					bson.D{{Key: "$ID", Value: contentID}, {Key: "PropertyKey", Value: "content"}},
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				bson.D{
					{Key: "TypePointer", Value: dsID},
					{Key: "Value", Value: bson.D{
						{Key: "DataSource", Value: bson.D{{Key: "EntityRef", Value: directEntityRef("Sample.Order")}}},
					}},
				},
				bson.D{
					{Key: "TypePointer", Value: contentID},
					{Key: "Value", Value: bson.D{{Key: "Widgets", Value: cellContainer()}}},
				},
			}},
		}},
	}
	m := &Mutator{rawData: pageWith(grid), widgetFinder: findBsonWidget}

	if got := m.EnclosingEntity("ctnCust"); got != "Sample.Order" {
		t.Errorf("EnclosingEntity(ctnCust) = %q, want %q", got, "Sample.Order")
	}
}

// False-positive control: a nearer DataView shadows the grid, and a widget
// outside any bound container still reports no entity. Without this the fix
// could "pass" by returning the grid's entity for everything on the page.
func TestEnclosingEntity_NearestSourceStillWins(t *testing.T) {
	grid := buildDataGridWithCustomContentColumn(directEntityRef("Sample.Order"), bson.A{
		int32(2),
		bson.D{
			{Key: "$Type", Value: "Forms$DataView"},
			{Key: "Name", Value: "dvInner"},
			{Key: "DataSource", Value: bson.D{{Key: "EntityRef", Value: directEntityRef("Sample.Customer")}}},
			{Key: "Widgets", Value: bson.A{
				int32(2),
				bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "txtInner"}},
			}},
		},
	})
	loose := bson.D{{Key: "$Type", Value: "Forms$DynamicText"}, {Key: "Name", Value: "txtLoose"}}
	m := &Mutator{rawData: pageWith(grid, loose), widgetFinder: findBsonWidget}

	if got := m.EnclosingEntity("txtInner"); got != "Sample.Customer" {
		t.Errorf("nested DataView must shadow the grid: got %q, want %q", got, "Sample.Customer")
	}
	if got := m.EnclosingEntity("txtLoose"); got != "" {
		t.Errorf("a widget outside every bound container must report no entity, got %q", got)
	}
}
