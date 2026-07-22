// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

func dLookup(d bson.D, key string) (any, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// A ListView database source (legacy engine) must serialize with the same
// metamodel-valid shape as the pluggable CustomWidgetXPathSource: a
// Forms$GridSortBar with SortItems, and a Forms$ListViewSearch with SearchRefs.
// The old code emitted a bogus Forms$ListViewSort and a `Paths` key (the search
// list was renamed to SearchRefs in 7.11.0), producing a client model that
// omitted the arrays the Mendix client reads .length of → runtime crash in
// retrieveByXPath/processResult.
func TestSerializeListViewDataSource_Database(t *testing.T) {
	doc := serializeListViewDataSource(&pages.DatabaseSource{
		EntityName: "M.Item",
		Sorting:    []*pages.GridSort{{AttributePath: "M.Item.Name", Direction: "Ascending"}},
	})

	if v, _ := dLookup(doc, "$Type"); v != "Forms$ListViewXPathSource" {
		t.Fatalf("$Type = %v", v)
	}
	// Must NOT carry the bogus keys.
	if _, ok := dLookup(doc, "Sort"); ok {
		t.Error("legacy ListView source must not emit a `Sort` key (Forms$ListViewSort is not a property of ListViewXPathSource)")
	}

	// SortBar → GridSortBar with a SortItems list holding the GridSortItem.
	sortBarV, ok := dLookup(doc, "SortBar")
	if !ok {
		t.Fatal("SortBar missing")
	}
	sortBar := sortBarV.(bson.D)
	if v, _ := dLookup(sortBar, "$Type"); v != "Forms$GridSortBar" {
		t.Errorf("SortBar $Type = %v", v)
	}
	items, ok := dLookup(sortBar, "SortItems")
	if !ok {
		t.Fatal("SortBar.SortItems missing")
	}
	if a, _ := items.(bson.A); len(a) < 2 {
		t.Errorf("SortItems should contain the sort item, got %v", items)
	}

	// Search → ListViewSearch with SearchRefs (not `Paths`).
	searchV, ok := dLookup(doc, "Search")
	if !ok {
		t.Fatal("Search missing")
	}
	search := searchV.(bson.D)
	if _, ok := dLookup(search, "SearchRefs"); !ok {
		t.Error("Search.SearchRefs missing")
	}
	if _, ok := dLookup(search, "Paths"); ok {
		t.Error("Search must not emit the obsolete `Paths` key (renamed SearchRefs in 7.11.0)")
	}
}

// The empty-datasource fallback (nil source) must produce the same valid shape.
func TestEmptyListViewXPathSource_Shape(t *testing.T) {
	doc := emptyListViewXPathSource()
	if _, ok := dLookup(doc, "SortBar"); !ok {
		t.Error("fallback source missing SortBar")
	}
	searchV, ok := dLookup(doc, "Search")
	if !ok {
		t.Fatal("fallback source missing Search")
	}
	if _, ok := dLookup(searchV.(bson.D), "SearchRefs"); !ok {
		t.Error("fallback Search missing SearchRefs")
	}
}
