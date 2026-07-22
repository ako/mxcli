// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// A ListView database source must serialize as Forms$ListViewXPathSource on the
// modelsdk engine (previously only microflow sources were supported, forcing
// --engine legacy).
func TestListViewSourceToGen_Database(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{
		EntityName:      "LvBug.Item",
		XPathConstraint: "[Rank > 5]",
		Sorting:         []*pages.GridSort{{AttributePath: "Name", Direction: "Ascending"}},
	})
	if err != nil {
		t.Fatalf("listViewSourceToGen(database): %v", err)
	}
	src, ok := el.(*genPg.ListViewXPathSource)
	if !ok {
		t.Fatalf("type = %T, want *genPg.ListViewXPathSource", el)
	}
	if src.XPathConstraint() != "[Rank > 5]" {
		t.Errorf("XPathConstraint = %q, want [Rank > 5]", src.XPathConstraint())
	}
	if src.EntityRef() == nil {
		t.Error("EntityRef must be set for a database source")
	}
	// Studio Pro requires the SortBar and Search sub-elements on a ListViewXPathSource.
	if src.SortBar() == nil {
		t.Error("SortBar must be set")
	}
	if src.Search() == nil {
		t.Error("Search must be set")
	}
}

// The encoded ListView database source must carry Search.SearchRefs. Without the
// Forms$ListViewSearch TypeDefaults registration the codec drops the empty,
// never-Set PartList, and the Mendix client crashes reading searchRefs.length in
// retrieveByXPath/processResult. Regression guard for that runtime crash.
func TestListViewSourceToGen_SearchRefsEmitted(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{EntityName: "LvBug.Item"})
	if err != nil {
		t.Fatalf("listViewSourceToGen: %v", err)
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	search, ok := lookup(d, "Search").(bson.D)
	if !ok {
		t.Fatalf("Search missing or not a document: %T", lookup(d, "Search"))
	}
	if lookup(search, "SearchRefs") == nil {
		t.Errorf("Search.SearchRefs must be emitted; Search had keys %v", dKeys(search))
	}
	sortBar, ok := lookup(d, "SortBar").(bson.D)
	if !ok {
		t.Fatalf("SortBar missing: %T", lookup(d, "SortBar"))
	}
	if lookup(sortBar, "SortItems") == nil {
		t.Errorf("SortBar.SortItems must be emitted; SortBar had keys %v", dKeys(sortBar))
	}
}

func lookup(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

func dKeys(d bson.D) []string {
	ks := make([]string, 0, len(d))
	for _, e := range d {
		ks = append(ks, e.Key)
	}
	return ks
}

// Database source with no explicit sorting still produces a valid source (empty SortBar).
func TestListViewSourceToGen_DatabaseNoSort(t *testing.T) {
	el, err := listViewSourceToGen(&pages.DatabaseSource{EntityName: "LvBug.Item"})
	if err != nil {
		t.Fatalf("listViewSourceToGen: %v", err)
	}
	if _, ok := el.(*genPg.ListViewXPathSource); !ok {
		t.Fatalf("type = %T, want *genPg.ListViewXPathSource", el)
	}
}

// Microflow source still works (regression guard).
func TestListViewSourceToGen_Microflow(t *testing.T) {
	el, err := listViewSourceToGen(&pages.MicroflowSource{Microflow: "LvBug.DS_Items"})
	if err != nil {
		t.Fatalf("listViewSourceToGen(microflow): %v", err)
	}
	if _, ok := el.(*genPg.MicroflowSource); !ok {
		t.Fatalf("type = %T, want *genPg.MicroflowSource", el)
	}
}
