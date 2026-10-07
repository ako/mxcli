// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// tabStubDeps serializes a tab container the way the codec does: a
// Forms$TabControl whose TabPages carry each page's Name.
type tabStubDeps struct{ stubWidgetDeps }

func (d *tabStubDeps) SerializeWidget(w pages.Widget) bson.D {
	tc, ok := w.(*pages.TabContainer)
	if !ok {
		return d.stubWidgetDeps.SerializeWidget(w)
	}
	arr := bson.A{int32(3)}
	for _, tp := range tc.TabPages {
		arr = append(arr, bson.D{
			{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(tp.ID))},
			{Key: "$Type", Value: "Forms$TabPage"},
			{Key: "Name", Value: tp.Name},
			{Key: "Widgets", Value: bson.A{int32(2)}},
		})
	}
	return bson.D{{Key: "$Type", Value: "Forms$TabControl"}, {Key: "TabPages", Value: arr}}
}

func newTabPage(name string) *pages.TabPage {
	return &pages.TabPage{BaseElement: model.BaseElement{ID: model.ID(types.GenerateID()), TypeName: "Forms$TabPage"}, Name: name}
}

func tabPageNames(t *testing.T, m *Mutator) []string {
	t.Helper()
	var out []string
	for _, el := range bsonnav.DGetArrayElements(bsonnav.DGet(findBsonWidget(m.rawData, "tabControl").widget, "TabPages")) {
		out = append(out, bsonnav.DGetString(el.(bson.D), "Name"))
	}
	return out
}

// mendixlabs/mxcli#1215: a tab page is added to the control's TabPages list —
// appended for INTO the control, next to the sibling for BEFORE/AFTER a tab
// page — and the control keeps the default page it had.
func TestInsertTabPages_Positions(t *testing.T) {
	for _, tc := range []struct {
		target, position string
		want             string
	}{
		{"tabControl", "into", "tabPage2,tpNew"},
		{"tabPage2", "after", "tabPage2,tpNew"},
		{"tabPage2", "before", "tpNew,tabPage2"},
	} {
		t.Run(tc.position, func(t *testing.T) {
			m := New(makeTabControlPage(), model.ID("page-1"), &tabStubDeps{})
			before := bsonnav.DGet(findBsonWidget(m.rawData, "tabControl").widget, "DefaultPagePointer")
			if err := m.InsertTabPages(tc.target, toPos(tc.position), []*pages.TabPage{newTabPage("tpNew")}); err != nil {
				t.Fatalf("insert %s %s: %v", tc.position, tc.target, err)
			}
			if got := strings.Join(tabPageNames(t, m), ","); got != tc.want {
				t.Errorf("TabPages = %s, want %s", got, tc.want)
			}
			ctl := findBsonWidget(m.rawData, "tabControl").widget
			if arr := bsonnav.ToBsonA(bsonnav.DGet(ctl, "TabPages")); !isListMarker(arr[0]) {
				t.Error("TabPages lost its list marker")
			}
			if after := bsonnav.DGet(ctl, "DefaultPagePointer"); !equalIDs(after, before) {
				t.Errorf("DefaultPagePointer changed from %v to %v", before, after)
			}
			if r := findBsonWidget(m.rawData, "tpNew"); r == nil || r.parentKey != "TabPages" {
				t.Error("the new tab page does not resolve inside TabPages")
			}
		})
	}
}

func toPos(s string) backend.InsertPosition { return backend.InsertPosition(s) }

// equalIDs compares two stored pointers; nil equals nil.
func equalIDs(a, b any) bool {
	ab, aok := a.(primitive.Binary)
	bb, bok := b.(primitive.Binary)
	if !aok || !bok {
		return a == nil && b == nil
	}
	return bsonutil.BsonBinaryToID(ab) == bsonutil.BsonBinaryToID(bb)
}

// An empty tab container gains the inserted page as its default.
func TestInsertTabPages_EmptyControlGetsDefault(t *testing.T) {
	m := New(makeRawPage(bson.D{
		{Key: "$Type", Value: "Forms$TabControl"},
		{Key: "Name", Value: "tabControl"},
		{Key: "DefaultPagePointer", Value: nil},
		{Key: "TabPages", Value: bson.A{int32(3)}},
	}), model.ID("page-1"), &tabStubDeps{})
	if err := m.InsertTabPages("tabControl", "into", []*pages.TabPage{newTabPage("tpNew")}); err != nil {
		t.Fatal(err)
	}
	ctl := findBsonWidget(m.rawData, "tabControl").widget
	newID := bsonnav.DGet(findBsonWidget(m.rawData, "tpNew").widget, "$ID")
	if !equalIDs(bsonnav.DGet(ctl, "DefaultPagePointer"), newID) {
		t.Errorf("DefaultPagePointer = %v, want the new page %v", bsonnav.DGet(ctl, "DefaultPagePointer"), newID)
	}
}

// A tab page belongs only in a TabPages list: not inside a tab page, and not
// next to an ordinary widget.
func TestInsertTabPages_Refusals(t *testing.T) {
	for _, tc := range []struct{ target, position, want string }{
		{"tabPage2", "into", "insert after tabPage2"},
		{"inner", "after", "siblings are tab pages"},
		{"inner", "into", "only be added to a tab container"},
		{"nosuch", "into", "not found"},
	} {
		t.Run(tc.target+"/"+tc.position, func(t *testing.T) {
			m := New(makeTabControlPage(), model.ID("page-1"), &tabStubDeps{})
			err := m.InsertTabPages(tc.target, toPos(tc.position), []*pages.TabPage{newTabPage("tpNew")})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
			if got := tabPageNames(t, m); len(got) != 1 {
				t.Errorf("TabPages changed to %v by a refused insert", got)
			}
		})
	}
}
