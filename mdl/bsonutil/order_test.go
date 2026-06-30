// SPDX-License-Identifier: Apache-2.0

package bsonutil

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestOrderStorageValue_IDFirstTypeSecondRestSorted(t *testing.T) {
	in := bson.M{
		"Name":  "X",
		"$Type": "Some$Type",
		"Apple": 1,
		"$ID":   "id-1",
		"Zebra": 2,
	}
	got, ok := OrderStorageValue(in).(bson.D)
	if !ok {
		t.Fatalf("expected bson.D, got %T", OrderStorageValue(in))
	}
	want := []string{"$ID", "$Type", "Apple", "Name", "Zebra"}
	if len(got) != len(want) {
		t.Fatalf("key count = %d, want %d (%v)", len(got), len(want), got)
	}
	for i, k := range want {
		if got[i].Key != k {
			t.Errorf("key[%d] = %q, want %q", i, got[i].Key, k)
		}
	}
}

func TestOrderStorageValue_RecursesNestedAndArrays(t *testing.T) {
	in := bson.M{
		"$ID":   "parent",
		"$Type": "Parent",
		"Child": bson.M{"Name": "c", "$Type": "Child", "$ID": "child"},
		"Kids": bson.A{
			int32(2), // versioned-array marker must stay first
			bson.M{"Field": "v", "$Type": "Kid", "$ID": "kid-0"},
		},
	}
	got := OrderStorageValue(in).(bson.D)

	if got[0].Key != "$ID" {
		t.Fatalf("parent[0] = %q, want $ID", got[0].Key)
	}

	// Nested Child document: $ID must be first.
	child := findKey(t, got, "Child").(bson.D)
	if child[0].Key != "$ID" {
		t.Errorf("Child[0] = %q, want $ID", child[0].Key)
	}

	// Array: marker preserved as element 0, nested doc ordered.
	kids := findKey(t, got, "Kids").(bson.A)
	if kids[0] != int32(2) {
		t.Errorf("Kids[0] = %v, want marker int32(2)", kids[0])
	}
	kid := kids[1].(bson.D)
	if kid[0].Key != "$ID" {
		t.Errorf("Kids[1][0] = %q, want $ID", kid[0].Key)
	}
}

func TestOrderStorageValue_MarshalsWithIDFirst(t *testing.T) {
	in := bson.M{"B": 1, "$Type": "T", "A": 2, "$ID": "id"}
	raw, err := bson.Marshal(OrderStorageValue(in))
	if err != nil {
		t.Fatal(err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	if d[0].Key != "$ID" {
		t.Errorf("on-the-wire first key = %q, want $ID", d[0].Key)
	}
}

func findKey(t *testing.T, d bson.D, key string) any {
	t.Helper()
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	t.Fatalf("key %q not found in %v", key, d)
	return nil
}
