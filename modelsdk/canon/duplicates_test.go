// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func binID(b byte) bson.Binary {
	d := make([]byte, 16)
	for i := range d {
		d[i] = b
	}
	return bson.Binary{Subtype: 0, Data: d}
}

func mustBSON(t *testing.T, d bson.D) []byte {
	t.Helper()
	raw, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

func TestDuplicateElementIDs_FindsTheReportedShape(t *testing.T) {
	// The reported corruption: two Texts$Translation elements on one page
	// sharing an id. Studio Pro refuses the WHOLE PROJECT for this, so it has to
	// be caught before the bytes land.
	raw := mustBSON(t, bson.D{
		{Key: "$ID", Value: binID(1)},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Title", Value: bson.D{
			{Key: "$ID", Value: binID(2)},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{
				bson.D{{Key: "$ID", Value: binID(9)}, {Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "en_US"}},
				bson.D{{Key: "$ID", Value: binID(9)}, {Key: "$Type", Value: "Texts$Translation"},
					{Key: "LanguageCode", Value: "de_DE"}},
			}},
		}},
	})
	dups := DuplicateElementIDs(raw)
	if len(dups) != 1 {
		t.Fatalf("got %d duplicates, want 1: %+v", len(dups), dups)
	}
	if len(dups[0].Types) != 2 || dups[0].Types[0] != "Texts$Translation" {
		t.Errorf("types = %v, want two Texts$Translation", dups[0].Types)
	}

	err := DuplicateElementIDError("page 'M.P'", raw)
	if err == nil {
		t.Fatal("no error for a document with duplicate ids")
	}
	// The message has to name the unit and the type, because that is what Studio
	// Pro's own error names and what the user will otherwise meet much later.
	for _, want := range []string{"page 'M.P'", "Texts$Translation", "cannot be opened"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message missing %q: %v", want, err)
		}
	}
}

func TestDuplicateElementIDs_PointersAreNotDuplicates(t *testing.T) {
	// The control that decides whether this guard is usable at all. An element's
	// id is referenced by POINTER properties elsewhere in the same document —
	// primitive properties holding the same 16-byte shape under another key — and
	// a real document is full of them. Counting those would flag every reference
	// as a duplicate, i.e. refuse every write.
	raw := mustBSON(t, bson.D{
		{Key: "$ID", Value: binID(1)},
		{Key: "$Type", Value: "DomainModels$DomainModel"},
		{Key: "Entities", Value: bson.A{
			bson.D{{Key: "$ID", Value: binID(2)}, {Key: "$Type", Value: "DomainModels$Entity"}},
			bson.D{{Key: "$ID", Value: binID(3)}, {Key: "$Type", Value: "DomainModels$Entity"}},
		}},
		{Key: "Associations", Value: bson.A{
			bson.D{
				{Key: "$ID", Value: binID(4)},
				{Key: "$Type", Value: "DomainModels$Association"},
				// Both point at real elements. Neither is a second definition.
				{Key: "ParentPointer", Value: binID(2)},
				{Key: "ChildPointer", Value: binID(3)},
			},
		}},
	})
	if dups := DuplicateElementIDs(raw); len(dups) != 0 {
		t.Errorf("pointers reported as duplicates, which would refuse every write: %+v", dups)
	}
	if err := DuplicateElementIDError("dm", raw); err != nil {
		t.Errorf("clean document refused: %v", err)
	}
}

func TestDuplicateElementIDs_UntypedSubDocumentIsNotAnElement(t *testing.T) {
	// $Type is what makes a sub-document an element. A bare {$ID: ...} carried
	// twice by something that is not an element must not be counted, or the
	// guard reports on shapes Mendix does not treat as identities.
	raw := mustBSON(t, bson.D{
		{Key: "$ID", Value: binID(1)},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "A", Value: bson.D{{Key: "$ID", Value: binID(7)}}},
		{Key: "B", Value: bson.D{{Key: "$ID", Value: binID(7)}}},
	})
	if dups := DuplicateElementIDs(raw); len(dups) != 0 {
		t.Errorf("untyped sub-documents counted as elements: %+v", dups)
	}
}

func TestDuplicateElementIDs_UnreadableBytesAreNotAnError(t *testing.T) {
	// This runs on the write path. Refusing a write because the guard could not
	// parse the bytes would be a worse failure than the one it prevents — and a
	// unit that does not unmarshal has a bigger problem than duplicate ids.
	if dups := DuplicateElementIDs([]byte("not bson")); dups != nil {
		t.Errorf("got %+v, want nil", dups)
	}
	if err := DuplicateElementIDError("u", []byte("not bson")); err != nil {
		t.Errorf("unreadable bytes refused the write: %v", err)
	}
	if err := DuplicateElementIDError("u", nil); err != nil {
		t.Errorf("empty contents refused the write: %v", err)
	}
}

func TestDuplicateElementIDs_ReportsSeveralButCapsTheMessage(t *testing.T) {
	// A document that has gone wrong this way has usually gone wrong repeatedly;
	// the count is worth reporting in full, the list is not.
	items := bson.A{}
	for i := 0; i < 5; i++ {
		items = append(items,
			bson.D{{Key: "$ID", Value: binID(byte(20 + i))}, {Key: "$Type", Value: "Texts$Translation"}},
			bson.D{{Key: "$ID", Value: binID(byte(20 + i))}, {Key: "$Type", Value: "Texts$Translation"}})
	}
	raw := mustBSON(t, bson.D{
		{Key: "$ID", Value: binID(1)}, {Key: "$Type", Value: "Forms$Page"},
		{Key: "Items", Value: items},
	})
	if got := len(DuplicateElementIDs(raw)); got != 5 {
		t.Fatalf("got %d duplicates, want 5", got)
	}
	msg := DuplicateElementIDError("page 'M.P'", raw).Error()
	if !strings.Contains(msg, "5 element id(s)") {
		t.Errorf("message should report the full count: %s", msg)
	}
	if !strings.Contains(msg, "and 2 more") {
		t.Errorf("message should cap the list: %s", msg)
	}
}
