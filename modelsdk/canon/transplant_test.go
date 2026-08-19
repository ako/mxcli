// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// flowDoc models the shape the ticket is about: a flow with two activities and a
// sequence flow whose endpoints are plain binary pointers at the activities.
// caption is the one piece of content, so a "same document, one value changed"
// rewrite can be expressed.
func flowDoc(t *testing.T, root, a, b, seq byte, caption string) []byte {
	t.Helper()
	return marshal(t, bson.D{
		{Key: "$Type", Value: "Microflows$Nanoflow"},
		{Key: "$ID", Value: bin(root)},
		{Key: "Name", Value: "Flow"},
		{Key: "ObjectCollection", Value: bson.D{
			{Key: "$Type", Value: "Microflows$MicroflowObjectCollection"},
			{Key: "$ID", Value: bin(root + 100)},
			{Key: "Objects", Value: bson.A{
				int32(3), // typed-array marker
				bson.D{{Key: "$Type", Value: "Microflows$StartEvent"}, {Key: "$ID", Value: bin(a)}},
				bson.D{
					{Key: "$Type", Value: "Microflows$ActionActivity"},
					{Key: "$ID", Value: bin(b)},
					{Key: "Caption", Value: caption},
				},
			}},
		}},
		{Key: "Flows", Value: bson.A{
			int32(3),
			bson.D{
				{Key: "$Type", Value: "Microflows$SequenceFlow"},
				{Key: "$ID", Value: bin(seq)},
				{Key: "OriginPointer", Value: bin(a)},
				{Key: "DestinationPointer", Value: bin(b)},
			},
		}},
	})
}

// idSet returns every element $ID in a document, as this package's UUID strings.
func idSet(t *testing.T, raw []byte) map[string]bool {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := map[string]bool{}
	for id := range collectElementIDs(d) {
		out[id] = true
	}
	return out
}

func lookupPath(t *testing.T, raw []byte, path ...string) any {
	t.Helper()
	var cur any
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cur = d
	for _, key := range path {
		m, ok := asDoc(cur)
		if !ok {
			t.Fatalf("path %v: not a document at %q", path, key)
		}
		cur = m[key]
	}
	return cur
}

// TestTransplantKeepsStoredIDsAcrossAContentChange is the ticket. Changing one
// value in a nanoflow rewrote 36 of its 37 element identities, because every
// sub-element of a rebuild gets a fresh random $ID. Studio Pro's version-control
// view then paints the whole document as changed.
func TestTransplantKeepsStoredIDsAcrossAContentChange(t *testing.T) {
	stored := flowDoc(t, 1, 2, 3, 4, "before")
	fresh := flowDoc(t, 9, 8, 7, 6, "after") // same shape, all-new IDs, one value changed

	out := TransplantIDs(fresh, stored)

	got, want := idSet(t, out), idSet(t, stored)
	for id := range want {
		if !got[id] {
			t.Errorf("stored element id %s was not carried over", id)
		}
	}
	if len(got) != len(want) {
		t.Errorf("element count changed: %d ids after transplant, %d stored", len(got), len(want))
	}
	// The content change must survive: this is a rewrite, not a revert.
	coll := lookupPath(t, out, "ObjectCollection", "Objects")
	objs, _ := asSlice(coll)
	act, _ := asDoc(objs[2])
	if act["Caption"] != "after" {
		t.Errorf("Caption = %v, want the new value", act["Caption"])
	}
}

// TestTransplantRewritesReferences is the invariant that PR #125 broke: a
// pointer is a primitive binary property, not a containment edge, so a
// containment walk never sees it. Renumbering ids without following every
// reference in the same pass is what made projects unopenable.
func TestTransplantRewritesReferences(t *testing.T) {
	stored := flowDoc(t, 1, 2, 3, 4, "before")
	fresh := flowDoc(t, 9, 8, 7, 6, "after")

	out := TransplantIDs(fresh, stored)

	flows := lookupPath(t, out, "Flows")
	fl, _ := asSlice(flows)
	seq, _ := asDoc(fl[1])
	origin, ok := seq["OriginPointer"].(bson.Binary)
	if !ok {
		t.Fatalf("OriginPointer is %T", seq["OriginPointer"])
	}
	dest := seq["DestinationPointer"].(bson.Binary)

	objs, _ := asSlice(lookupPath(t, out, "ObjectCollection", "Objects"))
	start, _ := asDoc(objs[1])
	activity, _ := asDoc(objs[2])
	startID := start["$ID"].(bson.Binary)
	activityID := activity["$ID"].(bson.Binary)

	if blobToUUID(origin.Data) != blobToUUID(startID.Data) {
		t.Errorf("OriginPointer %s no longer names the start event %s",
			blobToUUID(origin.Data), blobToUUID(startID.Data))
	}
	if blobToUUID(dest.Data) != blobToUUID(activityID.Data) {
		t.Errorf("DestinationPointer %s no longer names the activity %s",
			blobToUUID(dest.Data), blobToUUID(activityID.Data))
	}
}

// TestTransplantRoundTripIsStable is what makes the churn cumulative: changing a
// value and changing it back produced a semantically identical document with
// another set of fresh ids. With identities carried, the round trip returns to
// exactly the bytes it started from.
func TestTransplantRoundTripIsStable(t *testing.T) {
	original := flowDoc(t, 1, 2, 3, 4, "before")

	changed := TransplantIDs(flowDoc(t, 9, 8, 7, 6, "after"), original)
	reverted := TransplantIDs(flowDoc(t, 5, 4, 3, 2, "before"), changed)

	if string(reverted) != string(original) {
		t.Errorf("a change and its revert did not return to the original bytes\n got %x\nwant %x",
			reverted, original)
	}
}

// TestTransplantLeavesInsertedElementsFresh pins that only *corresponding*
// elements are matched. An activity added in the middle must not take over its
// neighbour's identity and push the churn down the rest of the list.
func TestTransplantLeavesInsertedElementsFresh(t *testing.T) {
	mk := func(root, a, b, extra byte, withExtra bool) []byte {
		objs := bson.A{
			int32(3),
			bson.D{{Key: "$Type", Value: "Microflows$StartEvent"}, {Key: "$ID", Value: bin(a)}},
		}
		if withExtra {
			objs = append(objs, bson.D{
				{Key: "$Type", Value: "Microflows$ActionActivity"},
				{Key: "$ID", Value: bin(extra)},
				{Key: "Caption", Value: "inserted"},
			})
		}
		objs = append(objs, bson.D{
			{Key: "$Type", Value: "Microflows$EndEvent"},
			{Key: "$ID", Value: bin(b)},
		})
		return marshal(t, bson.D{
			{Key: "$Type", Value: "Microflows$Nanoflow"},
			{Key: "$ID", Value: bin(root)},
			{Key: "Objects", Value: objs},
		})
	}

	stored := mk(1, 2, 3, 0, false)
	out := TransplantIDs(mk(9, 8, 7, 6, true), stored)

	objs, _ := asSlice(lookupPath(t, out, "Objects"))
	if len(objs) != 4 {
		t.Fatalf("expected marker + 3 objects, got %d entries", len(objs))
	}
	start, _ := asDoc(objs[1])
	inserted, _ := asDoc(objs[2])
	end, _ := asDoc(objs[3])

	if got := blobToUUID(start["$ID"].(bson.Binary).Data); got != blobToUUID(bin(2).Data) {
		t.Errorf("start event took id %s, want the stored one", got)
	}
	// The one the ticket is really about: the end event is *after* the insertion
	// point, and must still keep its identity.
	if got := blobToUUID(end["$ID"].(bson.Binary).Data); got != blobToUUID(bin(3).Data) {
		t.Errorf("end event took id %s, want the stored one — an insertion must not "+
			"shift every following element's identity", got)
	}
	if got := blobToUUID(inserted["$ID"].(bson.Binary).Data); got != blobToUUID(bin(6).Data) {
		t.Errorf("inserted activity took id %s, want its own fresh one", got)
	}
}

// TestTransplantIgnoresMismatchedTypes pins that correspondence is structural. A
// slot whose $Type changed is a different element, and inheriting the old one's
// identity would claim a change is not a change.
func TestTransplantIgnoresMismatchedTypes(t *testing.T) {
	mk := func(root, child byte, childType string) []byte {
		return marshal(t, bson.D{
			{Key: "$Type", Value: "Microflows$Nanoflow"},
			{Key: "$ID", Value: bin(root)},
			{Key: "Action", Value: bson.D{
				{Key: "$Type", Value: childType},
				{Key: "$ID", Value: bin(child)},
			}},
		})
	}
	stored := mk(1, 2, "Microflows$MicroflowCallAction")
	out := TransplantIDs(mk(9, 8, "Microflows$JavaScriptActionCallAction"), stored)

	child, _ := asDoc(lookupPath(t, out, "Action"))
	if got := blobToUUID(child["$ID"].(bson.Binary).Data); got != blobToUUID(bin(8).Data) {
		t.Errorf("an element of a different $Type inherited id %s", got)
	}
	// The document itself still corresponds, so its own id is carried.
	root, _ := asDoc(lookupPath(t, out))
	if got := blobToUUID(root["$ID"].(bson.Binary).Data); got != blobToUUID(bin(1).Data) {
		t.Errorf("root id = %s, want the stored one", got)
	}
}

// TestTransplantNeverDuplicatesAnID is the correctness guard. Quality of the
// match only affects how big a diff looks, but two elements sharing an $ID is a
// corrupt document — so a mapping that would rename one element onto an id
// another element still holds must be dropped, not applied.
func TestTransplantNeverDuplicatesAnID(t *testing.T) {
	// The mapping is partial: the second child's $Type changed, so it does not
	// correspond and keeps its own id — which is 8, the very id the first child
	// is about to be renamed onto. Applying that entry would leave two elements
	// answering to 8.
	mk := func(root, a, b byte, bType string) []byte {
		return marshal(t, bson.D{
			{Key: "$Type", Value: "Microflows$Nanoflow"},
			{Key: "$ID", Value: bin(root)},
			{Key: "Objects", Value: bson.A{
				int32(3),
				bson.D{{Key: "$Type", Value: "Microflows$StartEvent"}, {Key: "$ID", Value: bin(a)}},
				bson.D{{Key: "$Type", Value: bType}, {Key: "$ID", Value: bin(b)}},
			}},
		})
	}
	stored := mk(1, 8, 2, "Microflows$EndEvent")
	out := TransplantIDs(mk(1, 3, 8, "Microflows$ActionActivity"), stored)

	if n, distinct := countIDSlots(t, out), len(idSet(t, out)); n != distinct {
		t.Errorf("%d $ID slots but only %d distinct ids — the transplant duplicated an identity",
			n, distinct)
	}
}

func countIDSlots(t *testing.T, raw []byte) int {
	t.Helper()
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	n := 0
	var walk func(any)
	walk = func(v any) {
		if m, ok := asDoc(v); ok {
			if _, ok := m["$ID"]; ok {
				n++
			}
			for _, k := range sortedKeys(m) {
				walk(m[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(d)
	return n
}

// TestTransplantOnANewUnit pins the no-stored-document case: there is nothing to
// carry, and the contents must come through byte-identical.
func TestTransplantOnANewUnit(t *testing.T) {
	fresh := flowDoc(t, 9, 8, 7, 6, "after")
	if got := TransplantIDs(fresh, nil); string(got) != string(fresh) {
		t.Error("a unit with no stored counterpart was rewritten")
	}
	if got := TransplantIDs(fresh, []byte("not bson")); string(got) != string(fresh) {
		t.Error("an unreadable stored document must leave the contents alone")
	}
}

// TestTransplantDoesNotMutateItsInput pins that the caller's buffer is not
// patched behind its back — the same contract CarryIdentity holds to.
func TestTransplantDoesNotMutateItsInput(t *testing.T) {
	stored := flowDoc(t, 1, 2, 3, 4, "before")
	fresh := flowDoc(t, 9, 8, 7, 6, "after")
	before := string(fresh)

	TransplantIDs(fresh, stored)

	if string(fresh) != before {
		t.Error("TransplantIDs mutated the contents it was given")
	}
}

// TestReconcileCarriesIDs pins the wiring: both engines reach identity
// preservation through Reconcile, so the transplant has to happen there rather
// than at one engine's call site.
func TestReconcileCarriesIDs(t *testing.T) {
	stored := flowDoc(t, 1, 2, 3, 4, "before")
	out, unchanged := Reconcile(flowDoc(t, 9, 8, 7, 6, "after"), stored)
	if unchanged {
		t.Fatal("a changed caption must not be elided")
	}
	for id := range idSet(t, stored) {
		if !idSet(t, out)[id] {
			t.Errorf("Reconcile did not carry stored element id %s", id)
		}
	}
}
