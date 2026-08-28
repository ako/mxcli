// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// ListXmlSchemas reads the two fields a mapping's `with xml schema` reference
// needs — the document's Name and its owning module — from XmlSchemas$XmlSchema
// units (ako/mxcli#259).
//
// The type string and the Name key are not guesses. They were established
// against mxbuild 11.13.0 by planting a synthetic unit carrying exactly the keys
// this test writes: a mapping referring to it stopped being
//
//	[error] [CE1613] "The selected XML schema 'XGap.Probe_Xsd' no longer exists."
//
// and became
//
//	[error] [CE0292] "Please import an XSD file." at XML schema 'XGap.Probe_Xsd'
//
// — mxbuild naming, by module and name, the document it had just found. (The
// second error is expected: the synthetic schema carries no XSD contents.) The
// same string is what modelsdk/gen/xmlschemas registers with the codec and what
// modelsdk/gen/mappings/refs.go names as the reference target.
func TestListXmlSchemasReadsNameAndModule(t *testing.T) {
	writer, _ := newTestWriterV1(t, unitTableSchemaV1)

	const moduleID = "22222222-2222-2222-2222-222222222222"
	writeUnit(t, writer, moduleID, "", "Modules", "Projects$ModuleImpl", bson.D{
		{Key: "$Type", Value: "Projects$ModuleImpl"},
		{Key: "Name", Value: "XGap"},
	})
	writeUnit(t, writer, "33333333-3333-3333-3333-333333333333", moduleID, "Documents",
		"XmlSchemas$XmlSchema", bson.D{
			{Key: "$Type", Value: "XmlSchemas$XmlSchema"},
			{Key: "Documentation", Value: "orders"},
			{Key: "Entries", Value: bson.A{int32(2)}},
			{Key: "Excluded", Value: false},
			{Key: "ExportLevel", Value: "Hidden"},
			{Key: "FilePath", Value: "orders.xsd"},
			{Key: "Name", Value: "Orders_Xsd"},
		})

	got, err := writer.reader.ListXmlSchemas()
	if err != nil {
		t.Fatalf("ListXmlSchemas: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d schemas, want 1", len(got))
	}
	if got[0].Name != "Orders_Xsd" {
		t.Errorf("Name = %q, want Orders_Xsd", got[0].Name)
	}
	// The module is what makes the reference check module-aware; without it,
	// `A.Orders_Xsd` would resolve against `B.Orders_Xsd`.
	if got[0].Module != "XGap" {
		t.Errorf("Module = %q, want XGap", got[0].Module)
	}
	if got[0].FilePath != "orders.xsd" {
		t.Errorf("FilePath = %q, want orders.xsd", got[0].FilePath)
	}
}

// TestListXmlSchemasIgnoresOtherDocuments is the control: the type filter has to
// be doing the work, not the fact that the fixture holds only one document.
func TestListXmlSchemasIgnoresOtherDocuments(t *testing.T) {
	writer, _ := newTestWriterV1(t, unitTableSchemaV1)

	const moduleID = "22222222-2222-2222-2222-222222222222"
	writeUnit(t, writer, moduleID, "", "Modules", "Projects$ModuleImpl", bson.D{
		{Key: "$Type", Value: "Projects$ModuleImpl"},
		{Key: "Name", Value: "XGap"},
	})
	writeUnit(t, writer, "44444444-4444-4444-4444-444444444444", moduleID, "Documents",
		"JsonStructures$JsonStructure", bson.D{
			{Key: "$Type", Value: "JsonStructures$JsonStructure"},
			{Key: "Name", Value: "JSON_Orders"},
		})

	got, err := writer.reader.ListXmlSchemas()
	if err != nil {
		t.Fatalf("ListXmlSchemas: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %d schemas, want 0: %+v", len(got), got)
	}
}

const unitTableSchemaV1 = `
	CREATE TABLE Unit (
		UnitID BLOB PRIMARY KEY NOT NULL,
		ContainerID BLOB,
		ContainmentName TEXT,
		TreeConflict LONG,
		ContentsHash TEXT,
		ContentsConflicts TEXT,
		Contents BLOB
	)
`

func writeUnit(t *testing.T, w *Writer, unitID, containerID, containment, unitType string, doc bson.D) {
	t.Helper()
	contents, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal %s: %v", unitType, err)
	}
	if err := w.insertUnit(unitID, containerID, containment, unitType, contents); err != nil {
		t.Fatalf("insertUnit %s: %v", unitType, err)
	}
}
