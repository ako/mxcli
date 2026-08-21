// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// wfDoc models a workflow flow with two activities, each carrying the
// PersistentId every Workflows$* element has. pidA/pidB are the identities; the
// captions are the content, so "same document, one value changed" is expressible.
func wfDoc(t *testing.T, idA, pidA, idB, pidB byte, capA, capB string) []byte {
	t.Helper()
	return marshal(t, bson.D{
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "$ID", Value: bin(1)},
		{Key: "PersistentId", Value: bin(2)},
		{Key: "Flow", Value: bson.D{
			{Key: "$Type", Value: "Workflows$Flow"},
			{Key: "$ID", Value: bin(3)},
			{Key: "Activities", Value: bson.A{
				int32(3), // typed-array marker
				bson.D{
					{Key: "$Type", Value: "Workflows$SingleUserTaskActivity"},
					{Key: "$ID", Value: bin(idA)},
					{Key: "PersistentId", Value: bin(pidA)},
					{Key: "Name", Value: "TaskA"},
					{Key: "Caption", Value: capA},
				},
				bson.D{
					{Key: "$Type", Value: "Workflows$SingleUserTaskActivity"},
					{Key: "$ID", Value: bin(idB)},
					{Key: "PersistentId", Value: bin(pidB)},
					{Key: "Name", Value: "TaskB"},
					{Key: "Caption", Value: capB},
				},
			}},
		}},
	})
}

func persistentIDs(t *testing.T, raw []byte) []string {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var out []string
	var walk func(v any)
	walk = func(v any) {
		if d, ok := asDoc(v); ok {
			if b, ok := binary16(d, "PersistentId"); ok {
				out = append(out, blobToUUID(b))
			}
			for _, k := range sortedKeys(d) {
				walk(d[k])
			}
			return
		}
		if s, ok := asSlice(v); ok {
			for _, e := range s {
				walk(e)
			}
		}
	}
	walk(doc)
	return out
}

// Issue #949. Both engines mint a fresh PersistentId on every activity write, so
// a rebuilt workflow never equalled the stored one and no-op elision could not
// fire. The stored values must be carried onto the elements that correspond.
func TestCarryPersistentIDs_CarriesStoredValues(t *testing.T) {
	stored := wfDoc(t, 10, 20, 11, 21, "A", "B")
	rebuilt := wfDoc(t, 90, 91, 92, 93, "A", "B") // every identity re-minted

	got := persistentIDs(t, CarryPersistentIDs(rebuilt, stored))
	want := persistentIDs(t, stored)
	if len(got) != len(want) {
		t.Fatalf("got %d PersistentIds, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("PersistentId[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

// The point of carrying them: a rebuild of an unchanged workflow must compare
// equal, so the write is elided.
func TestReconcile_NoOpWorkflowRebuildIsElided(t *testing.T) {
	stored := wfDoc(t, 10, 20, 11, 21, "A", "B")
	rebuilt := wfDoc(t, 90, 91, 92, 93, "A", "B")

	_, unchanged := Reconcile(rebuilt, stored)
	if !unchanged {
		t.Error("a rebuild that changed nothing was not recognised as unchanged")
	}
}

// Control: a real edit must still be seen as a change, or the carry has turned
// into a way of losing writes.
func TestReconcile_ChangedWorkflowStillWrites(t *testing.T) {
	stored := wfDoc(t, 10, 20, 11, 21, "A", "B")
	rebuilt := wfDoc(t, 90, 91, 92, 93, "A", "CHANGED")

	if _, unchanged := Reconcile(rebuilt, stored); unchanged {
		t.Error("a caption change was elided")
	}
}

// A differing $Type means the element was replaced, not edited — inheriting its
// identity would claim otherwise.
func TestCarryPersistentIDs_DoesNotCrossTypes(t *testing.T) {
	stored := wfDoc(t, 10, 20, 11, 21, "A", "B")
	rebuilt := marshal(t, bson.D{
		{Key: "$Type", Value: "Workflows$Workflow"},
		{Key: "$ID", Value: bin(1)},
		{Key: "PersistentId", Value: bin(2)},
		{Key: "Flow", Value: bson.D{
			{Key: "$Type", Value: "Workflows$Flow"},
			{Key: "$ID", Value: bin(3)},
			{Key: "Activities", Value: bson.A{
				int32(3),
				bson.D{ // a different activity kind at the same position
					{Key: "$Type", Value: "Workflows$CallMicroflowActivity"},
					{Key: "$ID", Value: bin(90)},
					{Key: "PersistentId", Value: bin(91)},
					{Key: "Name", Value: "TaskA"},
				},
			}},
		}},
	})
	got := persistentIDs(t, CarryPersistentIDs(rebuilt, stored))
	for _, id := range got {
		if id == blobToUUID(bin(20).Data) {
			t.Error("a CallMicroflowActivity inherited a SingleUserTaskActivity's PersistentId")
		}
	}
}

// Injectivity: two elements must never end up sharing one identity, whatever the
// alignment does.
func TestCarryPersistentIDs_NeverDuplicates(t *testing.T) {
	stored := wfDoc(t, 10, 20, 11, 20, "A", "B") // stored already has a duplicate
	rebuilt := wfDoc(t, 90, 91, 92, 93, "A", "B")

	got := persistentIDs(t, CarryPersistentIDs(rebuilt, stored))
	seen := map[string]bool{}
	for _, id := range got {
		if seen[id] {
			t.Fatalf("two elements share PersistentId %s", id)
		}
		seen[id] = true
	}
}

// A document that cannot be read on either side passes through untouched.
func TestCarryPersistentIDs_MalformedPassesThrough(t *testing.T) {
	stored := wfDoc(t, 10, 20, 11, 21, "A", "B")
	junk := []byte{1, 2, 3}
	if got := CarryPersistentIDs(junk, stored); string(got) != string(junk) {
		t.Error("malformed contents were modified")
	}
	good := wfDoc(t, 90, 91, 92, 93, "A", "B")
	if got := CarryPersistentIDs(good, junk); string(got) != string(good) {
		t.Error("contents were modified against malformed stored bytes")
	}
}

// MXCLI_ALWAYS_WRITE turns off eliding the write, not preserving what the
// document is — the same rule StableId follows.
func TestReconcile_AlwaysWriteStillCarriesPersistentIDs(t *testing.T) {
	t.Setenv("MXCLI_ALWAYS_WRITE", "1")
	stored := wfDoc(t, 10, 20, 11, 21, "A", "B")
	rebuilt := wfDoc(t, 90, 91, 92, 93, "A", "B")

	out, unchanged := Reconcile(rebuilt, stored)
	if unchanged {
		t.Fatal("MXCLI_ALWAYS_WRITE must not elide")
	}
	got, want := persistentIDs(t, out), persistentIDs(t, stored)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("forced write re-minted PersistentId[%d]: %s, want %s", i, got[i], want[i])
		}
	}
}
