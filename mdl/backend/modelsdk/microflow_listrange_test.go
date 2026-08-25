// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// upstream #966: the Range list operation's offset and amount.
//
// Mendix nests them, one level down from where mxcli looked:
//
//	Microflows$ListRange{ListName, CustomRange}
//	  └── Microflows$CustomRange{LimitExpression, OffsetExpression}
//
// This engine had it flat on BOTH sides — the writer put LimitExpression and
// OffsetExpression directly on the ListRange, and the reader looked for them
// there. Being wrong twice is what hid it: mxcli read its own documents back
// perfectly, so a round trip through describe was clean and the authoring side
// looked correct. Only a document from Studio Pro (or from the legacy engine,
// whose PARSER has always read the nested form) exposed the read half, and only
// mxbuild exposed the write half.
//
// Both halves are load-bearing, and each fails differently:
//
//   - reading flat drops a paged range to `range($List)` on describe, which
//     re-executes as an unpaged one;
//   - writing flat is CE6520 "Amount and offset are not specified. Either
//     amount or offset or both must be specified." — measured on mxbuild
//     11.13.0, against the same project with the fix in place at 0 errors.
//
// Three sources agree on the nested shape: gen (`ListRange.customRange` is a
// Part), `generated/metamodel` (`CustomRange *MicroflowsCustomRange`), and the
// legacy parser's own fixture in sdk/mpr/parser_listoperation_test.go.

// listRangeBSON builds a ListRange in MENDIX's shape — the nested one — so a
// reader that looks flat finds nothing.
func listRangeBSON(list, offset, limit string) bson.Raw {
	return mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "lr-1"},
		{Key: "$Type", Value: "Microflows$ListRange"},
		{Key: "ListName", Value: list},
		{Key: "CustomRange", Value: bson.D{
			{Key: "$ID", Value: "cr-1"},
			{Key: "$Type", Value: "Microflows$CustomRange"},
			{Key: "LimitExpression", Value: limit},
			{Key: "OffsetExpression", Value: offset},
		}},
	})
}

// The reported defect: a stored range describes as `range($List)`.
func TestListRangeReadsTheNestedCustomRange(t *testing.T) {
	op := listOperationFromRaw(listRangeBSON("RawMessages", "$offset", "$amount"))

	rng, ok := op.(*microflows.ListRangeOperation)
	if !ok {
		t.Fatalf("listOperationFromRaw → %T, want *microflows.ListRangeOperation", op)
	}
	if rng.ListVariable != "RawMessages" {
		t.Errorf("ListVariable = %q, want %q", rng.ListVariable, "RawMessages")
	}
	if rng.OffsetExpression != "$offset" {
		t.Errorf("OffsetExpression = %q, want %q — the offset was dropped, so describe emits the unpaged range($List)",
			rng.OffsetExpression, "$offset")
	}
	if rng.LimitExpression != "$amount" {
		t.Errorf("LimitExpression = %q, want %q — the amount was dropped, so describe emits the unpaged range($List)",
			rng.LimitExpression, "$amount")
	}
}

// A range with only one bound is valid in Mendix (either amount or offset), and
// the absent one must stay absent rather than becoming a literal "".
func TestListRangeReadsASingleBound(t *testing.T) {
	for _, tc := range []struct{ name, offset, limit string }{
		{"amount only", "", "$amount"},
		{"offset only", "$offset", ""},
	} {
		op := listOperationFromRaw(listRangeBSON("L", tc.offset, tc.limit))
		rng := op.(*microflows.ListRangeOperation)
		if rng.OffsetExpression != tc.offset {
			t.Errorf("%s: OffsetExpression = %q, want %q", tc.name, rng.OffsetExpression, tc.offset)
		}
		if rng.LimitExpression != tc.limit {
			t.Errorf("%s: LimitExpression = %q, want %q", tc.name, rng.LimitExpression, tc.limit)
		}
	}
}

// A project written by the broken writer — mxcli 0.18 and earlier — has the
// bounds flat. It does not build (CE6520), but the author's expressions are on
// disk, so reading them means DESCRIBE shows the range that was meant and
// re-executing that output writes the nested form, repairing the project.
// Without the fallback the upgrade silently discards them instead.
func TestListRangeReadsTheFlatFormWrittenByOlderVersions(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "lr-1"},
		{Key: "$Type", Value: "Microflows$ListRange"},
		{Key: "ListName", Value: "All"},
		{Key: "LimitExpression", Value: "$Amount"},
		{Key: "OffsetExpression", Value: "$Offset"},
	})

	rng, ok := listOperationFromRaw(raw).(*microflows.ListRangeOperation)
	if !ok {
		t.Fatal("listOperationFromRaw did not produce a *microflows.ListRangeOperation")
	}
	if rng.OffsetExpression != "$Offset" || rng.LimitExpression != "$Amount" {
		t.Errorf("offset=%q limit=%q, want $Offset/$Amount — an mxcli-0.18 project loses "+
			"the bounds it does have, and with them the chance to repair itself on the next exec",
			rng.OffsetExpression, rng.LimitExpression)
	}
}

// The fallback must not paper over a genuine nested document that carries only
// one bound: if CustomRange is present, it is the whole truth. Otherwise a
// deliberate `range($L, $Offset)` could pick up a stale flat LimitExpression
// sitting beside it.
func TestListRangeFlatFallbackYieldsToTheNestedChild(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "lr-1"},
		{Key: "$Type", Value: "Microflows$ListRange"},
		{Key: "ListName", Value: "All"},
		{Key: "LimitExpression", Value: "$Stale"},
		{Key: "CustomRange", Value: bson.D{
			{Key: "$ID", Value: "cr-1"},
			{Key: "$Type", Value: "Microflows$CustomRange"},
			{Key: "OffsetExpression", Value: "$Offset"},
		}},
	})

	rng := listOperationFromRaw(raw).(*microflows.ListRangeOperation)
	if rng.LimitExpression != "" {
		t.Errorf("LimitExpression = %q, want empty — the nested CustomRange is authoritative", rng.LimitExpression)
	}
	if rng.OffsetExpression != "$Offset" {
		t.Errorf("OffsetExpression = %q, want $Offset", rng.OffsetExpression)
	}
}

// encodeListOperation runs an operation through the writer and hands back the
// stored document.
func encodeListOperation(t *testing.T, op microflows.ListOperation) bson.Raw {
	t.Helper()
	raw, err := (&codec.Encoder{}).Encode(listOperationToGen(op))
	if err != nil {
		t.Fatalf("encode list operation: %v", err)
	}
	return bson.Raw(raw)
}

// The write half. Flat keys are not merely a different spelling — mxbuild reads
// no bounds at all and fails the build, so the activity a user authored as
// `range($L, $offset, $amount)` is not a paged range in Mendix's eyes.
func TestListRangeWritesTheNestedCustomRange(t *testing.T) {
	doc := encodeListOperation(t, &microflows.ListRangeOperation{
		ListVariable:     "RawMessages",
		OffsetExpression: "$offset",
		LimitExpression:  "$amount",
	})

	if got := rawStr(doc, "$Type"); got != "Microflows$ListRange" {
		t.Fatalf("$Type = %q, want Microflows$ListRange", got)
	}
	if got := rawStr(doc, "ListName"); got != "RawMessages" {
		t.Errorf("ListName = %q, want RawMessages", got)
	}

	// The bounds must NOT sit on the ListRange itself. This half of the
	// assertion is the one that failed: without it, a writer that emits both
	// the flat keys and the nested child would pass.
	if _, ok := doc.Lookup("LimitExpression").StringValueOK(); ok {
		t.Error("LimitExpression is stored on the ListRange itself; Mendix keeps it on the nested CustomRange (CE6520)")
	}
	if _, ok := doc.Lookup("OffsetExpression").StringValueOK(); ok {
		t.Error("OffsetExpression is stored on the ListRange itself; Mendix keeps it on the nested CustomRange (CE6520)")
	}

	cr, ok := doc.Lookup("CustomRange").DocumentOK()
	if !ok {
		t.Fatal("no CustomRange child — mxbuild reports CE6520 \"Amount and offset are not specified\"")
	}
	if got := rawStr(cr, "$Type"); got != "Microflows$CustomRange" {
		t.Errorf("CustomRange $Type = %q, want Microflows$CustomRange", got)
	}
	if got := rawStr(cr, "OffsetExpression"); got != "$offset" {
		t.Errorf("CustomRange.OffsetExpression = %q, want $offset", got)
	}
	if got := rawStr(cr, "LimitExpression"); got != "$amount" {
		t.Errorf("CustomRange.LimitExpression = %q, want $amount", got)
	}
}

// The two halves against each other. A round trip passed before this fix as
// well — both sides were wrong in the same direction — so it is the shape
// assertions above that carry the proof, and this one only guards the pairing.
func TestListRangeRoundTrips(t *testing.T) {
	in := &microflows.ListRangeOperation{
		ListVariable:     "Items",
		OffsetExpression: "$Skip",
		LimitExpression:  "$Take",
	}
	out, ok := listOperationFromRaw(encodeListOperation(t, in)).(*microflows.ListRangeOperation)
	if !ok {
		t.Fatal("round trip did not produce a *microflows.ListRangeOperation")
	}
	if out.ListVariable != in.ListVariable ||
		out.OffsetExpression != in.OffsetExpression ||
		out.LimitExpression != in.LimitExpression {
		t.Errorf("round trip: got %+v, want %+v", out, in)
	}
}
