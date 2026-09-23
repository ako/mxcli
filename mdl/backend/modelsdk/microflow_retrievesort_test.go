// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TestRetrieveSourceFromGen_SortBy guards the retrieve "sort by …" clause. The
// sort columns live in the DatabaseRetrieveSource's NewSortings child (a
// SortingsList); without reading it the clause is silently dropped.
func TestRetrieveSourceFromGen_SortBy(t *testing.T) {
	raw := mustMarshalFlow(bson.D{
		{Key: "$ID", Value: "rs-1"},
		{Key: "$Type", Value: "Microflows$DatabaseRetrieveSource"},
		{Key: "Entity", Value: "LoftManagement.Application"},
		{Key: "NewSortings", Value: bson.D{
			{Key: "$ID", Value: "sl-1"},
			{Key: "$Type", Value: "Microflows$SortingsList"},
			{Key: "Sortings", Value: bson.A{
				int32(1), // typed-array marker
				bson.D{
					{Key: "$ID", Value: "rsort-1"},
					{Key: "$Type", Value: "Microflows$RetrieveSorting"},
					{Key: "SortOrder", Value: "asc"},
					{Key: "AttributeRef", Value: bson.D{
						{Key: "$ID", Value: "ar-1"},
						{Key: "$Type", Value: "DomainModels$AttributeRef"},
						{Key: "Attribute", Value: "LoftManagement.Application.Name"},
					}},
				},
			}},
		}},
	})
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	src, ok := retrieveSourceFromGen(el).(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatalf("retrieveSourceFromGen → not a DatabaseRetrieveSource")
	}
	if len(src.Sorting) != 1 {
		t.Fatalf("Sorting = %d items, want 1 (NewSortings dropped)", len(src.Sorting))
	}
	if got := src.Sorting[0]; got.AttributeQualifiedName != "LoftManagement.Application.Name" || string(got.Direction) != "asc" {
		t.Errorf("sort item = {%q, %q}, want {LoftManagement.Application.Name, asc}", got.AttributeQualifiedName, got.Direction)
	}
}

// TestRetrieveSourceToGen_SortByRoundTrip guards the write path: a database
// retrieve's "sort by …" columns must be serialized into the NewSortings
// envelope. Previously retrieveSourceToGen wrote an empty SortItemList and
// dropped every sort column, so DESCRIBE emitted a retrieve with no sort
// (issue #727).
func TestRetrieveSourceToGen_SortByRoundTrip(t *testing.T) {
	in := &microflows.DatabaseRetrieveSource{
		EntityQualifiedName: "SortBug.Ticket",
		Sorting: []*microflows.SortItem{
			{AttributeQualifiedName: "SortBug.Ticket.Priority", Direction: microflows.SortDirectionDescending},
			{AttributeQualifiedName: "SortBug.Ticket.Name", Direction: microflows.SortDirectionAscending},
		},
	}

	el := retrieveSourceToGen(in)
	if el == nil {
		t.Fatal("retrieveSourceToGen returned nil")
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, ok := retrieveSourceFromGen(decoded).(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatal("round-trip did not yield a DatabaseRetrieveSource")
	}
	if len(out.Sorting) != 2 {
		t.Fatalf("Sorting = %d items after round-trip, want 2 (sort columns dropped on write)", len(out.Sorting))
	}
	if out.Sorting[0].AttributeQualifiedName != "SortBug.Ticket.Priority" || out.Sorting[0].Direction != microflows.SortDirectionDescending {
		t.Errorf("sort[0] = {%q, %q}, want {SortBug.Ticket.Priority, Descending}", out.Sorting[0].AttributeQualifiedName, out.Sorting[0].Direction)
	}
	if out.Sorting[1].AttributeQualifiedName != "SortBug.Ticket.Name" || out.Sorting[1].Direction != microflows.SortDirectionAscending {
		t.Errorf("sort[1] = {%q, %q}, want {SortBug.Ticket.Name, Ascending}", out.Sorting[1].AttributeQualifiedName, out.Sorting[1].Direction)
	}
}

// TestRetrieveSourceToGen_SortOverAssociationWritesEntityRefSteps guards the
// BSON a sort over an association produces. The attribute's qualified name names
// the FAR entity, so on its own it is a reference Mendix cannot resolve against
// the retrieved entity — the hop has to be stored beside it, as an
// AttributeRef.EntityRef (DomainModels$IndirectEntityRef) of EntityRefSteps.
//
// This is the write half of mendixlabs/mxcli#1152, where the executor could not derive
// the hop and refused the statement outright; a fix that derived it but did not
// persist it would leave the same dangling reference in the document.
func TestRetrieveSourceToGen_SortOverAssociationWritesEntityRefSteps(t *testing.T) {
	in := &microflows.DatabaseRetrieveSource{
		EntityQualifiedName: "Administration.Account",
		Sorting: []*microflows.SortItem{{
			AttributeQualifiedName: "System.Language.Code",
			Direction:              microflows.SortDirectionAscending,
			EntityRefSteps: []microflows.EntityRefStep{{
				Association:       "System.User_Language",
				DestinationEntity: "System.Language",
			}},
		}},
	}

	el := retrieveSourceToGen(in)
	if el == nil {
		t.Fatal("retrieveSourceToGen returned nil")
	}
	raw, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	sortings, ok := bson.Raw(raw).Lookup("NewSortings").DocumentOK()
	if !ok {
		t.Fatal("no NewSortings envelope")
	}
	arr, ok := sortings.Lookup("Sortings").ArrayOK()
	if !ok {
		t.Fatal("no Sortings array")
	}
	vals, err := arr.Values()
	if err != nil {
		t.Fatalf("Sortings values: %v", err)
	}
	var item bson.Raw
	for _, v := range vals {
		// The first entry is the typed-array marker, not a document.
		if d, ok := v.DocumentOK(); ok {
			item = d
			break
		}
	}
	if item == nil {
		t.Fatal("Sortings array carries no sort item")
	}
	ref, ok := item.Lookup("AttributeRef").DocumentOK()
	if !ok {
		t.Fatal("sort item has no AttributeRef")
	}
	if got := ref.Lookup("Attribute").StringValue(); got != "System.Language.Code" {
		t.Errorf("Attribute = %q, want System.Language.Code", got)
	}
	entityRef, ok := ref.Lookup("EntityRef").DocumentOK()
	if !ok {
		t.Fatal("AttributeRef has no EntityRef — the association hop was dropped, " +
			"leaving a sort attribute that does not resolve against the retrieved entity")
	}
	if got := entityRef.Lookup("$Type").StringValue(); got != "DomainModels$IndirectEntityRef" {
		t.Errorf("EntityRef $Type = %q, want DomainModels$IndirectEntityRef", got)
	}
	steps, ok := entityRef.Lookup("Steps").ArrayOK()
	if !ok {
		t.Fatal("EntityRef has no Steps array")
	}
	stepVals, err := steps.Values()
	if err != nil {
		t.Fatalf("Steps values: %v", err)
	}
	var step bson.Raw
	for _, v := range stepVals {
		if d, ok := v.DocumentOK(); ok {
			step = d
			break
		}
	}
	if step == nil {
		t.Fatal("Steps array carries no step")
	}
	if got := step.Lookup("Association").StringValue(); got != "System.User_Language" {
		t.Errorf("Association = %q, want System.User_Language", got)
	}
	if got := step.Lookup("DestinationEntity").StringValue(); got != "System.Language" {
		t.Errorf("DestinationEntity = %q, want System.Language", got)
	}
}

// TestRetrieveSourceFromGen_SortOverAssociationReadsHops guards the READ half of
// mendixlabs/mxcli#1152. The hop was written and never read back, so DESCRIBE
// could not emit it even once MDL had a spelling for it — and a describer that
// drops what the writer stores is how a round trip silently changes a program.
func TestRetrieveSourceFromGen_SortOverAssociationReadsHops(t *testing.T) {
	in := &microflows.DatabaseRetrieveSource{
		EntityQualifiedName: "Sales.Order",
		Sorting: []*microflows.SortItem{{
			AttributeQualifiedName: "Sales.Address.City",
			Direction:              microflows.SortDirectionAscending,
			EntityRefSteps: []microflows.EntityRefStep{{
				Association:       "Sales.Order_BillTo",
				DestinationEntity: "Sales.Address",
			}},
		}},
	}

	raw, err := (&codec.Encoder{}).Encode(retrieveSourceToGen(in))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out, ok := retrieveSourceFromGen(decoded).(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatal("round-trip did not yield a DatabaseRetrieveSource")
	}
	if len(out.Sorting) != 1 {
		t.Fatalf("Sorting = %d items, want 1", len(out.Sorting))
	}
	got := out.Sorting[0]
	if got.AttributeQualifiedName != "Sales.Address.City" {
		t.Errorf("attribute = %q, want Sales.Address.City", got.AttributeQualifiedName)
	}
	if len(got.EntityRefSteps) != 1 {
		t.Fatalf("EntityRefSteps = %+v, want one hop — the association the sort navigates "+
			"is invisible to DESCRIBE without it", got.EntityRefSteps)
	}
	if got.EntityRefSteps[0].Association != "Sales.Order_BillTo" ||
		got.EntityRefSteps[0].DestinationEntity != "Sales.Address" {
		t.Errorf("hop = %+v, want Sales.Order_BillTo -> Sales.Address", got.EntityRefSteps[0])
	}
}

// CONTROL: an own-entity sort carries a DirectEntityRef or no EntityRef at all,
// and must read back with no hops. A reader that manufactured an empty hop would
// make DESCRIBE emit a stray `/`.
func TestRetrieveSourceFromGen_PlainSortHasNoHops(t *testing.T) {
	in := &microflows.DatabaseRetrieveSource{
		EntityQualifiedName: "Sales.Order",
		Sorting: []*microflows.SortItem{{
			AttributeQualifiedName: "Sales.Order.OrderNo",
			Direction:              microflows.SortDirectionAscending,
		}},
	}
	raw, err := (&codec.Encoder{}).Encode(retrieveSourceToGen(in))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	out := retrieveSourceFromGen(decoded).(*microflows.DatabaseRetrieveSource)
	if len(out.Sorting) != 1 || len(out.Sorting[0].EntityRefSteps) != 0 {
		t.Errorf("got %+v, want one sort item with no hops", out.Sorting)
	}
}
