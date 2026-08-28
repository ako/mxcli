// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"os"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// A unit holding two elements with the same $ID makes the whole PROJECT
// unopenable — Studio Pro reports "Duplicate Guid in unit ..." and refuses to
// load anything. The guard has to sit at the write choke point, so these tests
// go through the Writer rather than calling the detector, which is the half
// that can silently come unwired. (ako/mxcli-captrack #2)

func dupTranslationUnit(t *testing.T, sharedID string) []byte {
	t.Helper()
	tr := func(lang string) bson.D {
		return bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob(sharedID)}},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: lang},
			{Key: "Text", Value: "hallo"},
		}
	}
	b, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("22222222-2222-2222-2222-222222222222")}},
		{Key: "Name", Value: "P"},
		{Key: "Title", Value: bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("33333333-3333-3333-3333-333333333333")}},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{tr("de_DE"), tr("nl_NL")}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestUpdateUnitRefusesDuplicateElementIDs(t *testing.T) {
	const unitID = "44444444-4444-4444-4444-444444444444"
	w, unitPath := newV2WriterForCommitTest(t, unitID, unitDoc(t, "before"))

	before, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read seeded unit: %v", err)
	}

	err = w.UpdateRawUnit(unitID, dupTranslationUnit(t, "55555555-5555-5555-5555-555555555555"))
	if err == nil {
		t.Fatal("write accepted a unit with duplicate element ids")
	}
	for _, want := range []string{unitID, "Texts$Translation", "cannot be opened"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message missing %q: %v", want, err)
		}
	}

	// The refusal has to leave the stored unit alone. A guard that reports the
	// problem after writing the bytes is the failure it exists to prevent.
	after, err := os.ReadFile(unitPath)
	if err != nil {
		t.Fatalf("read unit after refusal: %v", err)
	}
	if string(after) != string(before) {
		t.Error("the refused write still changed the stored unit")
	}
}

func TestUpdateUnitAcceptsAUnitWhoseIDsAreDistinct(t *testing.T) {
	// The control. Two translations differing only in their id are the ORDINARY
	// shape of a translated page — a guard that refuses this refuses every
	// multilingual project, which is a far worse failure than the one it catches.
	const unitID = "66666666-6666-6666-6666-666666666666"
	w, unitPath := newV2WriterForCommitTest(t, unitID, unitDoc(t, "before"))

	tr := func(id, lang string) bson.D {
		return bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob(id)}},
			{Key: "$Type", Value: "Texts$Translation"},
			{Key: "LanguageCode", Value: lang},
		}
	}
	good, err := bson.Marshal(bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("77777777-7777-7777-7777-777777777777")}},
		{Key: "Name", Value: "P"},
		{Key: "Title", Value: bson.D{
			{Key: "$ID", Value: bson.Binary{Subtype: 0x00, Data: uuidToBlob("88888888-8888-8888-8888-888888888888")}},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{
				tr("99999999-9999-9999-9999-999999999999", "de_DE"),
				tr("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "nl_NL"),
			}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := w.UpdateRawUnit(unitID, good); err != nil {
		t.Fatalf("a normal translated page was refused: %v", err)
	}
	if _, err := os.ReadFile(unitPath); err != nil {
		t.Fatalf("unit missing after an accepted write: %v", err)
	}
}
