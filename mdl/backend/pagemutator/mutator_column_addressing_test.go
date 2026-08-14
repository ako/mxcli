// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ---------------------------------------------------------------------------
// Ledger #78: DataGrid2 column addressing — reject ambiguous ON, and list the
// real (derived) column names on a miss instead of a bare "not found".
// ---------------------------------------------------------------------------

func idBin(b byte) primitive.Binary {
	data := make([]byte, 16)
	data[0] = b
	return primitive.Binary{Subtype: 0x04, Data: data}
}

// buildGridWithColumns constructs the minimal DataGrid2 BSON that findBsonColumn
// walks: a Type.ObjectType.PropertyTypes with a `columns` group whose ValueType
// carries `header` + `attribute` sub-property types, and an Object.Properties
// holding the columns list. Column kinds: {"attr":"M.E.Merchant"} → derives
// "Merchant"; {"caption":"Amount"} → derives "Amount".
func buildGridWithColumns(cols []map[string]string) bson.D {
	colsID := idBin(0x10)
	headerID := idBin(0x11)
	attrID := idBin(0x12)

	colObjects := bson.A{int32(2)}
	for _, c := range cols {
		props := bson.A{int32(2)}
		if attr, ok := c["attr"]; ok {
			props = append(props, bson.D{
				{Key: "TypePointer", Value: attrID},
				{Key: "Value", Value: bson.D{{Key: "AttributeRef", Value: attr}}},
			})
		}
		if caption, ok := c["caption"]; ok {
			props = append(props, bson.D{
				{Key: "TypePointer", Value: headerID},
				{Key: "Value", Value: bson.D{
					{Key: "TextTemplate", Value: bson.D{
						{Key: "Template", Value: bson.D{
							{Key: "Items", Value: bson.A{int32(2), bson.D{{Key: "Text", Value: caption}}}},
						}},
					}},
				}},
			})
		}
		colObjects = append(colObjects, bson.D{{Key: "Properties", Value: props}})
	}

	return bson.D{
		{Key: "Type", Value: bson.D{
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					bson.D{
						{Key: "$ID", Value: colsID},
						{Key: "PropertyKey", Value: "columns"},
						{Key: "ValueType", Value: bson.D{
							{Key: "ObjectType", Value: bson.D{
								{Key: "PropertyTypes", Value: bson.A{
									int32(2),
									bson.D{{Key: "$ID", Value: headerID}, {Key: "PropertyKey", Value: "header"}},
									bson.D{{Key: "$ID", Value: attrID}, {Key: "PropertyKey", Value: "attribute"}},
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
					{Key: "TypePointer", Value: colsID},
					{Key: "Value", Value: bson.D{{Key: "Objects", Value: colObjects}}},
				},
			}},
		}},
	}
}

func gridFinder(grid bson.D) widgetFinder {
	return func(_ bson.D, name string) *bsonWidgetResult {
		if name == "dg" {
			return &bsonWidgetResult{widget: grid}
		}
		return nil
	}
}

func TestFindBsonColumn_UniqueMatch(t *testing.T) {
	grid := buildGridWithColumns([]map[string]string{
		{"attr": "M.E.Merchant"},
		{"caption": "Amount"},
	})
	res, err := findBsonColumn(bson.D{}, "dg", "Merchant", gridFinder(grid))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil {
		t.Fatal("expected a result for the unique 'Merchant' column")
	}
}

func TestFindBsonColumn_AmbiguousIsRejected(t *testing.T) {
	// Two dynamic-text columns captioned 'Amount' derive the same name. Before
	// the fix, ON "Amount" silently mutated the first; now it must error.
	grid := buildGridWithColumns([]map[string]string{
		{"attr": "M.E.Merchant"},
		{"caption": "Amount"},
		{"caption": "Amount"},
	})
	res, err := findBsonColumn(bson.D{}, "dg", "Amount", gridFinder(grid))
	if res != nil {
		t.Error("ambiguous match must not resolve to a column")
	}
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected an ambiguity error, got %v", err)
	}
	if !strings.Contains(err.Error(), "caption") {
		t.Errorf("ambiguity error should point at captions as the cause, got %q", err.Error())
	}
}

func TestFindBsonColumn_NotFoundListsAvailable(t *testing.T) {
	// The authored MDL name never survives a write; addressing by it must fail
	// with an actionable message listing the derived names that DO work.
	grid := buildGridWithColumns([]map[string]string{
		{"attr": "M.E.Merchant"},
		{"caption": "Amount"},
	})
	_, err := findBsonColumn(bson.D{}, "dg", "colAuthoredAttr", gridFinder(grid))
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	msg := err.Error()
	for _, want := range []string{"not found", "Merchant", "Amount", "available"} {
		if !strings.Contains(msg, want) {
			t.Errorf("not-found error missing %q; got: %s", want, msg)
		}
	}
}

func TestFormatColumnNameList(t *testing.T) {
	// Dedup, first-seen order, and quoting of non-identifier names.
	got := formatColumnNameList([]string{"Merchant", "Amount", "Amount"})
	if got != "Merchant, Amount" {
		t.Errorf("got %q, want %q", got, "Merchant, Amount")
	}
	if got := formatColumnNameList([]string{"has space"}); !strings.Contains(got, `"has space"`) {
		t.Errorf("non-identifier name should be quoted, got %q", got)
	}
}

// TestColumnMatchCount_CrossGrid: two different grids on one page each carry a
// "Merchant" column. A bare `ON Merchant` is ambiguous across grids too — the
// count must be page-wide, not per-grid (ledger #78 follow-up).
func TestColumnMatchCount_CrossGrid(t *testing.T) {
	grid1 := buildGridWithColumns([]map[string]string{{"attr": "M.E.Merchant"}, {"caption": "Amount"}})
	grid2 := buildGridWithColumns([]map[string]string{{"attr": "M.E.Merchant"}, {"caption": "Total"}})
	page := bson.D{{Key: "Widgets", Value: bson.A{int32(2), grid1, grid2}}}
	m := &Mutator{rawData: page}

	if n := m.columnMatchCount("Merchant"); n != 2 {
		t.Errorf("Merchant across two grids: count = %d, want 2 (page-wide)", n)
	}
	if n := m.columnMatchCount("Amount"); n != 1 {
		t.Errorf("Amount (grid1 only): count = %d, want 1", n)
	}
	if n := m.columnMatchCount("Nonexistent"); n != 0 {
		t.Errorf("missing name: count = %d, want 0", n)
	}
}

// ---------------------------------------------------------------------------
// #891: a bare DataGrid2 column name resolves through findBsonWidget, so
// INSERT/REPLACE took the generic widget path and wrote a layout container into
// the grid's column list — a project mxbuild could not load. Refuse instead.
//
// These go through ReplaceWidget/InsertWidget rather than calling the helper
// directly, so deleting either call site fails the test. A direct-call test
// would prove the helper works and nothing about the wiring (#884).
// ---------------------------------------------------------------------------

func columnNodeFinder(t *testing.T) widgetFinder {
	t.Helper()
	col := bson.D{
		{Key: "$Type", Value: objectListItemType},
		{Key: "Name", Value: "NextRunAt"},
	}
	return func(_ bson.D, name string) *bsonWidgetResult {
		if name == "NextRunAt" {
			return &bsonWidgetResult{widget: col}
		}
		return nil
	}
}

func TestReplaceWidget_RefusesBareColumnTarget(t *testing.T) {
	m := &Mutator{rawData: bson.D{}, widgetFinder: columnNodeFinder(t)}
	err := m.ReplaceWidget("NextRunAt", "", nil)
	if err == nil {
		t.Fatal("ReplaceWidget accepted a bare column target — this is the #891 corruption path")
	}
	if !strings.Contains(err.Error(), "gridName.NextRunAt") {
		t.Errorf("error should name the qualified form, got: %v", err)
	}
}

func TestInsertWidget_RefusesBareColumnTarget(t *testing.T) {
	m := &Mutator{rawData: bson.D{}, widgetFinder: columnNodeFinder(t)}
	err := m.InsertWidget("NextRunAt", "", "AFTER", nil)
	if err == nil {
		t.Fatal("InsertWidget accepted a bare column target — this is the #891 corruption path")
	}
	if !strings.Contains(err.Error(), "gridName.NextRunAt") {
		t.Errorf("error should name the qualified form, got: %v", err)
	}
}

// A real widget must not trip the guard, or every ordinary REPLACE would break.
// The two tests above already prove the call sites are wired; this pins the
// discriminator itself.
func TestObjectListItemGuard_IgnoresRealWidgets(t *testing.T) {
	w := &bsonWidgetResult{widget: bson.D{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "Name", Value: "txtName"}}}
	if err := refuseObjectListItemTarget(w, "txtName"); err != nil {
		t.Fatalf("guard wrongly refused a real widget: %v", err)
	}
	pluggable := &bsonWidgetResult{widget: bson.D{{Key: "$Type", Value: "CustomWidgets$CustomWidget"}, {Key: "Name", Value: "grid1"}}}
	if err := refuseObjectListItemTarget(pluggable, "grid1"); err != nil {
		t.Fatalf("guard wrongly refused a pluggable widget: %v", err)
	}
}
