// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A domain model can hold annotations — the note boxes Studio Pro draws on the
// canvas to explain the diagram. Every blank Mendix app ships with one, and
// modellers add more to group and label a large model.
//
// The legacy writer hardcoded `Annotations` to an empty typed array, so ANY
// rewrite of a domain model deleted every note in it: adding one entity to the
// blank app's MyFirstModule took its annotation count from 1 to 0 and the
// caption disappeared from the project. `mx check` reports 0 errors, because an
// annotation is decorative — nothing below Studio Pro can see the loss.
//
// The parser had a second, quieter defect: it read `Location` only as a BSON
// sub-document, while Studio Pro stores it as the string "x;y" (measured on a
// stock 11.13.0 app). So even before the write threw them away, the read had
// already lost every position, and Width was never read at all.
//
// This is the same guard-don't-drop rule as ADR-0005: what MDL cannot express, a
// rewrite carries.

// storedAnnotation is the shape Studio Pro writes, taken from the annotation in
// a blank 11.13.0 app's MyFirstModule.
func storedAnnotation() map[string]any {
	return map[string]any{
		"$Type":       "DomainModels$Annotation",
		"Caption":     "This Domain model defines the data structure of this module.\r\n\r\nMore info: https://docs.mendix.com/refguide/domain-model",
		"ExportLevel": "Hidden",
		"Location":    "60;240",
		"Width":       int32(440),
	}
}

// The position is a string, not a sub-document. Reading only the sub-document
// form silently returned (0,0) for every real annotation.
func TestParseAnnotationReadsStringLocationAndWidth(t *testing.T) {
	got := parseAnnotation(storedAnnotation())

	if got.Caption == "" {
		t.Fatal("Caption is empty")
	}
	if got.Location.X != 60 || got.Location.Y != 240 {
		t.Errorf("Location = (%d,%d), want (60,240) — Studio Pro stores it as the string \"x;y\"",
			got.Location.X, got.Location.Y)
	}
	if got.Width != 440 {
		t.Errorf("Width = %d, want 440", got.Width)
	}
}

// The sub-document form is accepted too, exactly as the entity parser does: a
// document written by an older mxcli must still read.
func TestParseAnnotationStillReadsMapLocation(t *testing.T) {
	raw := storedAnnotation()
	raw["Location"] = map[string]any{"x": int32(12), "y": int32(34)}

	got := parseAnnotation(raw)
	if got.Location.X != 12 || got.Location.Y != 34 {
		t.Errorf("Location = (%d,%d), want (12,34)", got.Location.X, got.Location.Y)
	}
}

// The write must carry what the read produced. Before this, a domain model
// rewrite emitted `Annotations: [3]` — the empty typed array — regardless.
func TestSerializeAnnotationRoundTrips(t *testing.T) {
	annot := &domainmodel.Annotation{
		Caption:  "Orders live here",
		Location: model.Point{X: 60, Y: 240},
		Width:    440,
	}
	annot.ID = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"

	got := parseAnnotation(dToM(serializeDomainModelAnnotation(annot)))

	if got.Caption != annot.Caption {
		t.Errorf("Caption = %q, want %q", got.Caption, annot.Caption)
	}
	if got.Location != annot.Location {
		t.Errorf("Location = %+v, want %+v", got.Location, annot.Location)
	}
	if got.Width != annot.Width {
		t.Errorf("Width = %d, want %d", got.Width, annot.Width)
	}
	if got.ID != annot.ID {
		t.Errorf("ID = %q, want the stored %q (a real UUID: idToBsonBinary cannot round-trip a made-up string) — a fresh one makes an unchanged model differ (ADR-0008)",
			got.ID, annot.ID)
	}
}

// The serialized shape must match what Studio Pro writes, key for key: the
// position as the string "x;y", and ExportLevel present. A sub-document position
// is what the parser used to expect and is NOT what Mendix stores.
func TestSerializeAnnotationMatchesStudioProShape(t *testing.T) {
	annot := &domainmodel.Annotation{Caption: "x", Location: model.Point{X: 60, Y: 240}, Width: 440}
	annot.ID = "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d"

	got := dToM(serializeDomainModelAnnotation(annot))

	if got["$Type"] != "DomainModels$Annotation" {
		t.Errorf("$Type = %v", got["$Type"])
	}
	if loc, ok := got["Location"].(string); !ok || loc != "60;240" {
		t.Errorf("Location = %#v, want the string \"60;240\"", got["Location"])
	}
	if got["ExportLevel"] != "Hidden" {
		t.Errorf("ExportLevel = %v, want Hidden — every Studio Pro annotation carries it", got["ExportLevel"])
	}
}
