// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	// This package uses BOTH driver majors: navigation_write.go is on v1 and
	// navigation_read.go on v2. The aliases are what keep a test that spans
	// the write and read paths from silently mixing them.
	bsonv1 "go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"
)

func boolPtr(b bool) *bool { return &b }

func throwOf(t *testing.T, doc bsonv1.D) any {
	t.Helper()
	for _, e := range doc {
		if e.Key == "ThrowPartialSyncError" {
			return e.Value
		}
	}
	return nil
}

// The spec field is a POINTER, and this is why. ThrowPartialSyncError is a bare
// bool with no unset value of its own, so a non-pointer spec would make every
// rewrite that never mentions the clause reset the stored flag — silently, on a
// property neither generated source declares and nothing else reports.
func TestThrowSyncErrorIsLeftAloneWhenTheStatementIsSilent(t *testing.T) {
	// BOTH stored values, because false is also the zero value: a writer that
	// reset the flag on every rewrite would be indistinguishable from a correct
	// one if the fixture only ever stored false. Same trap as CompatibilityMode,
	// and it was found by a control that failed to fail.
	for _, storedValue := range []bool{true, false} {
		stored := bsonv1.D{
			{Key: "Name", Value: "TabletOffline"},
			{Key: "ThrowPartialSyncError", Value: storedValue},
		}
		out := navPatchWebProfile(stored, types.NavigationProfileSpec{})
		if got := throwOf(t, out); got != storedValue {
			t.Errorf("a rewrite that never mentioned the clause changed a stored %v to %v",
				storedValue, got)
		}
	}

	stored := bsonv1.D{
		{Key: "Name", Value: "TabletOffline"},
		{Key: "ThrowPartialSyncError", Value: false},
	}

	// Control in the other direction: when the statement DOES say something,
	// it must actually be written — otherwise the first assertion passes for
	// a writer that ignores the field entirely.
	out := navPatchWebProfile(stored, types.NavigationProfileSpec{ThrowSyncError: boolPtr(true)})
	if got := throwOf(t, out); got != true {
		t.Errorf("`on sync error throw` did not write: %v", got)
	}
	out = navPatchWebProfile(stored, types.NavigationProfileSpec{ThrowSyncError: boolPtr(false)})
	if got := throwOf(t, out); got != false {
		t.Errorf("`on sync error continue` did not write: %v", got)
	}
}

// Absent must read as true, not as the zero value. Every reference profile
// carries true and Studio Pro's box is checked by default, so defaulting to
// false would silently turn off error reporting for a document that simply
// predates the key.
func TestAbsentThrowPartialSyncErrorReadsAsTrue(t *testing.T) {
	empty, err := bsonv2.Marshal(bsonv2.D{{Key: "Name", Value: "Responsive"}})
	if err != nil {
		t.Fatal(err)
	}
	if !throwPartialSyncError(bsonv2.Raw(empty)) {
		t.Error("an absent key must read as true")
	}

	// Control: a present false is honoured, so the default is not masking the
	// read.
	set, err := bsonv2.Marshal(bsonv2.D{{Key: "ThrowPartialSyncError", Value: false}})
	if err != nil {
		t.Fatal(err)
	}
	if throwPartialSyncError(bsonv2.Raw(set)) {
		t.Error("a stored false must be read as false")
	}
}
