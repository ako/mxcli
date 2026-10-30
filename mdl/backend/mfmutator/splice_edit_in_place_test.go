// SPDX-License-Identifier: Apache-2.0

package mfmutator

import (
	"bytes"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1761 (mendixlabs/mxcli#1667): re-running `create or modify
// microflow` with only a filter's expression changed did not edit the
// activity, it replaced it — the activity, its action and its operation all
// got new $IDs, while a decision whose condition changed the same way kept its
// own. Studio Pro compares and merges by $ID, so it showed the edited filter
// as one activity removed and another added.
//
// A replace by one activity of the same kind (same activity and action $Type)
// is an edit of the stored activity: the new content is written where the
// stored one is, and the stored $IDs are carried onto every element that pairs
// up — the same key of two paired elements, a list entry with the same name
// (an attribute, an association, a parameter), or the same position in a list
// of unnamed entries of the same length. What does not pair is new.

// editDeps serializes an activity the way fakeDeps does, with action as its
// Action — the document the codec would write for the fragment's statement.
type editDeps struct {
	fakeDeps
	action bson.D
}

func (d *editDeps) SerializeObject(o microflows.MicroflowObject) (bson.D, error) {
	doc, err := d.fakeDeps.SerializeObject(o)
	if err != nil || d.action == nil {
		return doc, err
	}
	if _, ok := o.(*microflows.ActionActivity); ok && !dSet(doc, "Action", d.action) {
		doc = append(doc, bson.E{Key: "Action", Value: d.action})
	}
	return doc, nil
}

// el is an element: its $ID is the named one, or a freshly minted one for "",
// as the builder mints them for a fragment.
func el(name, typ string, props ...bson.E) bson.D {
	id := bin(name)
	if name == "" {
		id = bson.Binary{Data: types.UUIDToBlob(types.GenerateID())}
	}
	return append(bson.D{{Key: "$ID", Value: id}, {Key: "$Type", Value: typ}}, props...)
}

func kv(k string, v any) bson.E { return bson.E{Key: k, Value: v} }

// list is a stored list with its marker.
func list(marker int32, els ...bson.D) bson.A {
	out := bson.A{marker}
	for _, e := range els {
		out = append(out, e)
	}
	return out
}

// The six kinds the issue names. Each returns the action with the given $IDs
// ("" mints) and the value under test.
func filterAction(act, op, expr string) bson.D {
	return el(act, "Microflows$ListOperationsAction", kv("ErrorHandlingType", "Rollback"),
		kv("NewOperation", el(op, "Microflows$FilterByExpression", kv("Expression", expr), kv("ListName", "SaleList"))),
		kv("ResultVariableName", "BigList"))
}

func findAction(act, op, expr string) bson.D {
	return el(act, "Microflows$ListOperationsAction", kv("ErrorHandlingType", "Rollback"),
		kv("NewOperation", el(op, "Microflows$FindByExpression", kv("Expression", expr), kv("ListName", "SaleList"))),
		kv("ResultVariableName", "Found"))
}

func retrieveAction(act, src, rng, sorts, xpath string) bson.D {
	return el(act, "Microflows$RetrieveAction", kv("ErrorHandlingType", "Rollback"),
		kv("RetrieveSource", el(src, "Microflows$DatabaseRetrieveSource", kv("Entity", "M.Sale"),
			kv("Range", el(rng, "Microflows$ConstantRange", kv("SingleObject", false))),
			kv("XpathConstraint", xpath),
			kv("NewSortings", el(sorts, "Microflows$SortingsList", kv("Sortings", list(2)))))),
		kv("ResultVariableName", "Big"))
}

func memberItem(id, attr, assoc, value string) bson.D {
	return el(id, "Microflows$ChangeActionItem", kv("Attribute", attr), kv("Association", assoc),
		kv("Type", "Set"), kv("Value", value))
}

func changeAction(typ, act string, items ...bson.D) bson.D {
	return el(act, typ, kv("ErrorHandlingType", "Rollback"), kv("Items", list(2, items...)),
		kv("RefreshInClient", false), kv("Commit", "No"), kv("ChangeVariableName", "Found"))
}

func mapping(id, param, arg string) bson.D {
	return el(id, "Microflows$MicroflowCallParameterMapping", kv("Argument", arg), kv("Parameter", param))
}

func callAction(act, call string, mappings ...bson.D) bson.D {
	return el(act, "Microflows$MicroflowCallAction", kv("ErrorHandlingType", "Rollback"),
		kv("MicroflowCall", el(call, "Microflows$MicroflowCall", kv("Microflow", "M.SUB"),
			kv("ParameterMappings", list(2, mappings...)))),
		kv("ResultVariableName", "Ok"))
}

// withAction is line() with the activity a carrying action.
func withAction(action bson.D) []bson.D {
	objs := line()
	objs[1] = append(objs[1], bson.E{Key: "Action", Value: action})
	return objs
}

// replaceA replaces a, which holds stored, by one activity serialized with
// replacement, and returns the mutator, what was saved and the fragment.
func replaceA(t *testing.T, stored, replacement bson.D) (*Mutator, []byte, *backend.MicroflowFragment) {
	t.Helper()
	raw, err := bson.Marshal(unit(withAction(stored), lineFlows()))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	deps := &editDeps{action: replacement}
	m, err := New(d, "unit", deps)
	if err != nil {
		t.Fatal(err)
	}
	frag := oneActivity()
	if err := m.Replace(model.ID(uid("a")), frag); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	return m, deps.saved, frag
}

func TestSplice_ReplaceBySameKindEditsTheStoredActivity(t *testing.T) {
	for _, tc := range []struct {
		name string
		// stored is a's action as stored; replacement the fragment's, with
		// fresh $IDs; want the replacement with the stored $IDs.
		stored, replacement, want bson.D
	}{
		{"filter expression",
			filterAction("act", "op", "$currentObject/Amount > 10"),
			filterAction("", "", "$currentObject/Amount >= 10"),
			filterAction("act", "op", "$currentObject/Amount >= 10")},
		{"find expression",
			findAction("act", "op", "$currentObject/Amount = 3"),
			findAction("", "", "$currentObject/Amount = 4"),
			findAction("act", "op", "$currentObject/Amount = 4")},
		{"retrieve where",
			retrieveAction("act", "src", "rng", "srt", "[Amount > 100]"),
			retrieveAction("", "", "", "", "[Amount > 200]"),
			retrieveAction("act", "src", "rng", "srt", "[Amount > 200]")},
		{"change member value",
			changeAction("Microflows$ChangeAction", "act", memberItem("i1", "M.Sale.Amount", "", "7"), memberItem("i2", "M.Sale.Note", "", "'x'")),
			changeAction("Microflows$ChangeAction", "", memberItem("", "M.Sale.Amount", "", "8"), memberItem("", "M.Sale.Note", "", "'x'")),
			changeAction("Microflows$ChangeAction", "act", memberItem("i1", "M.Sale.Amount", "", "8"), memberItem("i2", "M.Sale.Note", "", "'x'"))},
		{"create member value",
			changeAction("Microflows$CreateChangeAction", "act", memberItem("i1", "M.Sale.Amount", "", "1")),
			changeAction("Microflows$CreateChangeAction", "", memberItem("", "M.Sale.Amount", "", "2")),
			changeAction("Microflows$CreateChangeAction", "act", memberItem("i1", "M.Sale.Amount", "", "2"))},
		{"call microflow argument",
			callAction("act", "call", mapping("p1", "M.SUB.Sale", "$New"), mapping("p2", "M.SUB.Factor", "2")),
			callAction("", "", mapping("", "M.SUB.Sale", "$New"), mapping("", "M.SUB.Factor", "3")),
			callAction("act", "call", mapping("p1", "M.SUB.Sale", "$New"), mapping("p2", "M.SUB.Factor", "3"))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, saved, _ := replaceA(t, tc.stored, tc.replacement)
			// What the edit should leave: the stored unit with a's action
			// holding the new value under the stored $IDs — a keeps its
			// $ID, its slot in the collection, its place and size, and the
			// flows through it are untouched.
			want, _ := bson.Marshal(unit(withAction(tc.want), lineFlows()))
			if !bytes.Equal(saved, want) {
				var got bson.D
				_ = bson.Unmarshal(saved, &got)
				t.Errorf("the replace did not edit the stored activity in place:\n got %v\nwant %v", got, unit(withAction(tc.want), lineFlows()))
			}
			if !m.EditedInPlace(model.ID(uid("a"))) {
				t.Error("EditedInPlace reports a swapped activity")
			}
		})
	}
}

// itemIDs maps each entry of the list at path under a's action to its $ID, by
// the entry's name property.
func itemIDs(t *testing.T, saved []byte, nameKey string, path ...string) map[string]string {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(saved, &d); err != nil {
		t.Fatal(err)
	}
	m, _ := New(d, "unit", &fakeDeps{})
	a := m.graph().nodes[uid("a")]
	if a == nil {
		t.Fatal("a is not in the unit: the activity got a new $ID")
	}
	cur := dDoc(a.doc, "Action")
	for _, k := range path[:len(path)-1] {
		cur = dDoc(cur, k)
	}
	out := map[string]string{}
	for _, e := range arrayElements(dGet(cur, path[len(path)-1])) {
		if ed, ok := e.(bson.D); ok {
			out[dString(ed, nameKey)+dString(ed, "Association")] = binaryID(dGet(ed, "$ID"))
		}
	}
	return out
}

// A member list pairs by member, never by position: a reordered member keeps
// its own $ID, a removed member's $ID goes with it — onto no other member —
// and an added member is new.
func TestSplice_ReplaceBySameKindPairsMembersByName(t *testing.T) {
	t.Run("change members reordered, one dropped, one added", func(t *testing.T) {
		_, saved, _ := replaceA(t,
			changeAction("Microflows$ChangeAction", "act",
				memberItem("i1", "M.Sale.Amount", "", "7"),
				memberItem("i2", "M.Sale.Note", "", "'x'"),
				memberItem("i3", "", "M.Sale_Customer", "$Cust")),
			changeAction("Microflows$ChangeAction", "",
				memberItem("", "M.Sale.Note", "", "'y'"),
				memberItem("", "M.Sale.Amount", "", "8"),
				memberItem("", "M.Sale.Code", "", "'c'")))
		ids := itemIDs(t, saved, "Attribute", "Items")
		if ids["M.Sale.Note"] != uid("i2") || ids["M.Sale.Amount"] != uid("i1") {
			t.Errorf("a reordered member lost its $ID: %v", ids)
		}
		if c := ids["M.Sale.Code"]; c == uid("i1") || c == uid("i2") || c == uid("i3") || c == "" {
			t.Errorf("the added member took a stored $ID: %s", c)
		}
		if bytes.Contains(saved, types.UUIDToBlob(uid("i3"))) {
			t.Error("the dropped association member's $ID is still in the unit")
		}
	})
	t.Run("call arguments reordered, one dropped", func(t *testing.T) {
		_, saved, _ := replaceA(t,
			callAction("act", "call", mapping("p1", "M.SUB.Sale", "$New"), mapping("p2", "M.SUB.Factor", "2")),
			callAction("", "", mapping("", "M.SUB.Factor", "3")))
		ids := itemIDs(t, saved, "Parameter", "MicroflowCall", "ParameterMappings")
		if len(ids) != 1 || ids["M.SUB.Factor"] != uid("p2") {
			t.Errorf("want Factor alone, keeping p2's $ID, got %v", ids)
		}
		if bytes.Contains(saved, types.UUIDToBlob(uid("p1"))) {
			t.Error("the dropped argument's $ID is still in the unit")
		}
	})
	t.Run("an association set member is not an add member", func(t *testing.T) {
		add := memberItem("", "", "M.Sale_Customer", "$Cust")
		dSet(add, "Type", "Add")
		_, saved, _ := replaceA(t,
			changeAction("Microflows$ChangeAction", "act", memberItem("i3", "", "M.Sale_Customer", "$Cust")),
			changeAction("Microflows$ChangeAction", "", add))
		if bytes.Contains(saved, types.UUIDToBlob(uid("i3"))) {
			t.Error("a member that now adds to the association kept the $ID of the one that set it")
		}
	})
}

// A replace by another kind of activity is not an edit: the activity is new,
// with new $IDs throughout, and the stored one is gone. The same action with
// another operation keeps the activity and the action, and only the operation
// is new.
func TestSplice_ReplaceByAnotherKindIsNew(t *testing.T) {
	t.Run("another action", func(t *testing.T) {
		m, saved, frag := replaceA(t,
			filterAction("act", "op", "$currentObject/Amount > 10"),
			changeAction("Microflows$ChangeAction", "", memberItem("", "M.Sale.Amount", "", "8")))
		for _, gone := range []string{"a", "act", "op"} {
			if bytes.Contains(saved, types.UUIDToBlob(uid(gone))) {
				t.Errorf("%s's $ID is still in the unit", gone)
			}
		}
		if !edges(t, saved)[[2]string{uid("start"), string(frag.Entry)}] {
			t.Error("the flow into the replaced activity does not enter the new one")
		}
		if m.EditedInPlace(model.ID(uid("a"))) {
			t.Error("EditedInPlace reports an activity of another kind as edited")
		}
	})
	t.Run("another operation", func(t *testing.T) {
		_, saved, _ := replaceA(t,
			filterAction("act", "op", "$currentObject/Amount > 10"),
			findAction("", "", "$currentObject/Amount > 10"))
		var d bson.D
		_ = bson.Unmarshal(saved, &d)
		m, _ := New(d, "unit", &fakeDeps{})
		a := m.graph().nodes[uid("a")]
		if a == nil {
			t.Fatal("the activity got a new $ID")
		}
		act := dDoc(a.doc, "Action")
		if binaryID(dGet(act, "$ID")) != uid("act") {
			t.Error("the action got a new $ID")
		}
		op := dDoc(act, "NewOperation")
		if dString(op, "$Type") != "Microflows$FindByExpression" || binaryID(dGet(op, "$ID")) == uid("op") {
			t.Errorf("the find kept the filter's $ID: %v", op)
		}
		if bytes.Contains(saved, types.UUIDToBlob(uid("op"))) {
			t.Error("the filter's $ID is still in the unit")
		}
	})
}

// The notes a create or modify states for the activity come with the edit,
// their lines attached to the stored activity; and a stored note linked to the
// fragment's activity after the replace (LinkNote, ako/mxcli#1695) is linked
// to the stored one.
func TestSplice_ReplaceBySameKindKeepsNotesOnTheStoredActivity(t *testing.T) {
	objs := append(withAction(filterAction("act", "op", "x > 10")), obj("old", "Microflows$Annotation", 250, 330))
	raw, _ := bson.Marshal(unit(objs, lineFlows()))
	var d bson.D
	_ = bson.Unmarshal(raw, &d)
	deps := &editDeps{action: filterAction("", "", "x >= 10")}
	m, err := New(d, "unit", deps)
	if err != nil {
		t.Fatal(err)
	}
	frag := oneActivity()
	note := &microflows.Annotation{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		Position:    model.Point{X: 360, Y: 100},
	}, Caption: "edited"}
	frag.Objects = append(frag.Objects, note)
	frag.AnnotationFlows = []*microflows.AnnotationFlow{{
		BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
		OriginID:    note.ID, DestinationID: frag.Entry,
	}}
	if err := m.Replace(model.ID(uid("a")), frag); err != nil {
		t.Fatal(err)
	}
	if err := m.LinkNote(model.ID(uid("old")), frag.Entry); err != nil {
		t.Fatalf("link the stored note to the replacement: %v", err)
	}
	if err := m.Save(); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(deps.saved, types.UUIDToBlob(string(frag.Entry))) {
		t.Error("the fragment's activity $ID is in the unit: the stored activity was not kept")
	}
	var got bson.D
	_ = bson.Unmarshal(deps.saved, &got)
	g, _ := New(got, "unit", &fakeDeps{})
	lines := map[string]bool{}
	for _, f := range g.graph().annotationFlows(uid("a")) {
		lines[f.origin] = true
	}
	if !lines[string(note.ID)] || !lines[uid("old")] {
		t.Errorf("want the stated note and the stored one attached to a, got %v", lines)
	}
	if p := g.graph().nodes[string(note.ID)]; p == nil || p.pos != (point{250, 100}) {
		t.Errorf("the note is not drawn where it is relative to the activity: %v", p)
	}
}

// Inside a loop body, and on an activity with a custom error handler, the
// replace already swapped one activity in place (ako/mxcli#1106,
// mendixlabs/mxcli#1489); of the same kind, it now keeps the stored identity
// there too.
func TestSplice_ReplaceBySameKindInPlaceKeepsIdentityEverywhere(t *testing.T) {
	t.Run("in a loop body", func(t *testing.T) {
		inner := append(obj("inner", "Microflows$ActionActivity", 100, 60), bson.E{Key: "Action", Value: filterAction("act", "op", "x > 10")})
		loop := append(obj("loop", "Microflows$LoopedActivity", 420, 200), bson.E{Key: "ObjectCollection", Value: bson.D{
			{Key: "$ID", Value: bin("loopoc")},
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "Objects", Value: bson.A{int32(3), inner, obj("inner2", "Microflows$ActionActivity", 260, 60)}},
		}})
		raw, _ := bson.Marshal(unit([]bson.D{obj("start", "Microflows$StartEvent", 100, 200), loop},
			[]bson.D{flow("f1", "start", "loop", 1, 3, false), flow("fi", "inner", "inner2", 1, 3, false)}))
		var d bson.D
		_ = bson.Unmarshal(raw, &d)
		deps := &editDeps{action: filterAction("", "", "x >= 10")}
		m, err := New(d, "unit", deps)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Replace(model.ID(uid("inner")), oneActivity()); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		for _, kept := range []string{"inner", "act", "op"} {
			if !bytes.Contains(deps.saved, types.UUIDToBlob(uid(kept))) {
				t.Errorf("%s's $ID is gone", kept)
			}
		}
		if !edges(t, deps.saved)[[2]string{uid("inner"), uid("inner2")}] {
			t.Error("the body flow no longer leaves the stored activity")
		}
		if !m.EditedInPlace(model.ID(uid("inner"))) {
			t.Error("EditedInPlace reports a swapped activity")
		}
	})
	t.Run("with an error handler", func(t *testing.T) {
		objs, flows := handlerFlow()
		act := callAction("act", "call", mapping("p1", "M.SUB.Factor", "2"))
		dSet(act, "ErrorHandlingType", string(microflows.ErrorHandlingTypeCustomWithoutRollback))
		objs[1] = append(objs[1][:len(objs[1])-1], bson.E{Key: "Action", Value: act})
		raw, _ := bson.Marshal(unit(objs, flows))
		var d bson.D
		_ = bson.Unmarshal(raw, &d)
		repl := callAction("", "", mapping("", "M.SUB.Factor", "3"))
		dSet(repl, "ErrorHandlingType", string(microflows.ErrorHandlingTypeCustomWithoutRollback))
		deps := &editDeps{action: repl}
		m, err := New(d, "unit", deps)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.Replace(model.ID(uid("a")), oneActivityEH(microflows.ErrorHandlingTypeCustomWithoutRollback)); err != nil {
			t.Fatal(err)
		}
		if err := m.Save(); err != nil {
			t.Fatal(err)
		}
		for _, kept := range []string{"a", "act", "call", "p1"} {
			if !bytes.Contains(deps.saved, types.UUIDToBlob(uid(kept))) {
				t.Errorf("%s's $ID is gone", kept)
			}
		}
		e := edges(t, deps.saved)
		if !e[[2]string{uid("a"), uid("h")}] || !e[[2]string{uid("start"), uid("a")}] {
			t.Errorf("the flows through the activity, its error flow included, do not run through the stored one: %v", e)
		}
	})
}
