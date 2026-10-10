// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#705 item 2. describe -> exec of the Blank template's
// FeedbackModule.ACT_Feedback_UploadImage lost its three annotation connectors
// and three header properties:
//
//   - nanoflowToGen never wrote ObjectCollection.AnnotationFlows, though the
//     reader fills it and the flow builder produces it. microflowToGen always
//     did; ruleToGen had the same omission. With the connectors gone the notes
//     float free, and the next DESCRIBE no longer attaches them to anything.
//   - ExportLevel, UseListParameterByReference and ReturnVariableName were never
//     written, so a rewrite deleted them. None has an MDL spelling except the
//     return variable (`returns T as $Var`), so UpdateNanoflow carries the stored
//     keys. A key the stored document lacks is filled at the write choke point
//     with Studio Pro's value, but only on a version it was measured on — a key
//     a project's metamodel does not declare makes it unopenable
//     (mendixlabs/mxcli#1373, canon.CompletePropertySets).
package modelsdkbackend

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// annotatedCollection is a start event, one activity and a note attached to it.
func annotatedCollection() *microflows.MicroflowObjectCollection {
	start := &microflows.StartEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000001")},
	}}
	end := &microflows.EndEvent{BaseMicroflowObject: microflows.BaseMicroflowObject{
		BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000002")},
		Position:    model.Point{X: 200, Y: 100},
	}}
	note := &microflows.Annotation{
		BaseMicroflowObject: microflows.BaseMicroflowObject{
			BaseElement: model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000003")},
			Position:    model.Point{X: 200, Y: 0},
			Size:        model.Size{Width: 200, Height: 50},
		},
		Caption: "Explains the end",
	}
	return &microflows.MicroflowObjectCollection{
		Objects: []microflows.MicroflowObject{start, end, note},
		Flows: []*microflows.SequenceFlow{{
			BaseElement:   model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000004")},
			OriginID:      start.ID,
			DestinationID: end.ID,
		}},
		AnnotationFlows: []*microflows.AnnotationFlow{{
			BaseElement:   model.BaseElement{ID: model.ID("11111111-0000-0000-0000-000000000005")},
			OriginID:      note.ID,
			DestinationID: end.ID,
		}},
	}
}

func countFlowType(t *testing.T, raw []byte, typeName string) int {
	t.Helper()
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	n := 0
	for _, e := range doc {
		if e.Key != "Flows" {
			continue
		}
		for _, f := range e.Value.(bson.A) {
			if d, ok := f.(bson.D); ok {
				for _, fe := range d {
					if fe.Key == "$Type" && fe.Value == typeName {
						n++
					}
				}
			}
		}
	}
	return n
}

func TestNanoflowToGen_WritesAnnotationFlows(t *testing.T) {
	nf := &microflows.Nanoflow{Name: "NF", ObjectCollection: annotatedCollection()}
	g := nanoflowToGen(nf, 11)
	assignNanoflowIDs(g)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if n := countFlowType(t, raw, "Microflows$AnnotationFlow"); n != 1 {
		t.Errorf("AnnotationFlows written = %d, want 1 — the note is left unattached", n)
	}
	// Control: the sequence flow was always written.
	if n := countFlowType(t, raw, "Microflows$SequenceFlow"); n != 1 {
		t.Errorf("SequenceFlows written = %d, want 1", n)
	}
}

func TestRuleToGen_WritesAnnotationFlows(t *testing.T) {
	r := &microflows.Rule{Name: "R", ReturnType: &microflows.BooleanType{}, ObjectCollection: annotatedCollection()}
	g := ruleToGen(r, 11)
	raw, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if n := countFlowType(t, raw, "Microflows$AnnotationFlow"); n != 1 {
		t.Errorf("AnnotationFlows written = %d, want 1 — the note is left unattached", n)
	}
}

func nanoflowFixture(t *testing.T) (*Backend, *microflows.Nanoflow) {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	nf := &microflows.Nanoflow{ContainerID: mod.ID, Name: "ZzCarryNanoflow", ReturnType: &microflows.BooleanType{}}
	if err := b.CreateNanoflow(nf); err != nil {
		t.Fatalf("CreateNanoflow: %v", err)
	}
	return b, nf
}

// setStoredKeys stands in for a Studio Pro nanoflow: the three header keys at
// values that differ from anything a rebuild would produce.
func setStoredKeys(t *testing.T, b *Backend, id model.ID, kv bson.D) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, e := range kv {
		set := false
		for i := range d {
			if d[i].Key == e.Key {
				d[i].Value, set = e.Value, true
			}
		}
		if !set {
			d = append(d, e)
		}
	}
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := b.writer.UpdateRawUnit(string(id), out); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
}

func storedKeys(t *testing.T, b *Backend, id model.ID) map[string]any {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := map[string]any{}
	for _, e := range d {
		out[e.Key] = e.Value
	}
	return out
}

func TestUpdateNanoflow_CarriesStoredHeaderKeys(t *testing.T) {
	b, nf := nanoflowFixture(t)
	setStoredKeys(t, b, nf.ID, bson.D{
		{Key: "ExportLevel", Value: "API"},
		{Key: "UseListParameterByReference", Value: false},
		{Key: "ReturnVariableName", Value: "IsValid"},
	})

	nf.Documentation = "rewritten"
	if err := b.UpdateNanoflow(nf); err != nil {
		t.Fatalf("UpdateNanoflow: %v", err)
	}
	got := storedKeys(t, b, nf.ID)
	for k, want := range map[string]any{
		"ExportLevel": "API", "UseListParameterByReference": false, "ReturnVariableName": "IsValid",
		// Control: the authored change still lands.
		"Documentation": "rewritten",
	} {
		if got[k] != want {
			t.Errorf("%s = %#v, want %#v", k, got[k], want)
		}
	}
}

// The return variable is the one of the three MDL can author, and what the
// statement says wins over what is stored.
func TestUpdateNanoflow_AuthoredReturnVariableWins(t *testing.T) {
	b, nf := nanoflowFixture(t)
	setStoredKeys(t, b, nf.ID, bson.D{{Key: "ReturnVariableName", Value: "IsValid"}})

	nf.ReturnVariableName = "Result"
	if err := b.UpdateNanoflow(nf); err != nil {
		t.Fatalf("UpdateNanoflow: %v", err)
	}
	if got := storedKeys(t, b, nf.ID)["ReturnVariableName"]; got != "Result" {
		t.Errorf("ReturnVariableName = %#v, want the authored Result", got)
	}
	if got, err := b.GetNanoflow(nf.ID); err != nil || got.ReturnVariableName != "Result" {
		t.Errorf("read back ReturnVariableName = %+v (%v), want Result", got, err)
	}
}

// A stored document without the keys gets the values Studio Pro writes, but
// only the keys MEASURED on a version this old: a key the project's metamodel
// may not declare is worse than leaving it absent. ExportLevel was measured on
// 10.24 and 11.14, so the 11.6 fixture gets it; UseListParameterByReference only
// on 11.14, so it does not (modelsdk/canon/studiopro_property_defaults.json).
// Before mendixlabs/mxcli#1373 neither was written, and a nanoflow without them
// is one Mendix's merge engine cannot compare with Studio Pro's next save.
func TestUpdateNanoflow_GetsTheMeasuredKeysTheStoredDocumentLacks(t *testing.T) {
	b, nf := nanoflowFixture(t)
	if pv := b.ProjectVersion(); pv == nil || pv.IsAtLeast(11, 14) || !pv.IsAtLeast(10, 24) {
		t.Fatalf("precondition: the fixture must be 10.24 ≤ v < 11.14 to sit between the two floors; got %+v", pv)
	}
	nf.Documentation = "rewritten"
	if err := b.UpdateNanoflow(nf); err != nil {
		t.Fatalf("UpdateNanoflow: %v", err)
	}
	got := storedKeys(t, b, nf.ID)
	if got["ExportLevel"] != "Hidden" {
		t.Errorf("ExportLevel = %#v, want Studio Pro's Hidden", got["ExportLevel"])
	}
	if v, ok := got["UseListParameterByReference"]; ok {
		t.Errorf("UseListParameterByReference = %#v was written below the version it was measured on", v)
	}
}

// #843 (rehearsal M2): a nanoflow the writer stored without ReturnVariableName
// (created from `returns Boolean`, no `as $Var`) takes the one a later
// `create or modify … returns Boolean as $Done` states, through the header
// patch — which refused it ("set it in Studio Pro"), so no mdl 1 script could
// ever name the return variable of such a nanoflow.
func TestSetHeader_AddsReturnVariableToAStoredNanoflow(t *testing.T) {
	b, nf := nanoflowFixture(t)
	// mxcli has written ReturnVariableName since mendixlabs/mxcli#1373, so the
	// shape this test is about — a nanoflow an older mxcli stored without the
	// key — can only be produced by editing the unit file behind the writer.
	stripStoredKeyOnDisk(t, b, nf.ID, "ReturnVariableName")
	if _, ok := storedKeys(t, b, nf.ID)["ReturnVariableName"]; ok {
		t.Fatal("precondition: the fixture nanoflow still stores ReturnVariableName")
	}
	if pv := b.ProjectVersion(); pv == nil || !pv.IsAtLeast(10, 12) {
		t.Fatalf("precondition: the fixture must be 10.12+, where Nanoflow declares ReturnVariableName; got %+v", pv)
	}

	declared := *nf
	declared.ReturnVariableName = "Done"
	m, err := b.OpenMicroflowForMutation(nf.ID)
	if err != nil {
		t.Fatalf("OpenMicroflowForMutation: %v", err)
	}
	changed, err := m.SetHeader(&declared)
	if err != nil {
		t.Fatalf("SetHeader: %v", err)
	}
	if len(changed) != 1 || changed[0] != "ReturnVariableName" {
		t.Fatalf("changed %v, want [ReturnVariableName]", changed)
	}
	if err := m.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := storedKeys(t, b, nf.ID)["ReturnVariableName"]; got != "Done" {
		t.Fatalf("stored ReturnVariableName = %#v, want Done", got)
	}

	// The second statement finds it stored: nothing to patch.
	m, err = b.OpenMicroflowForMutation(nf.ID)
	if err != nil {
		t.Fatalf("OpenMicroflowForMutation: %v", err)
	}
	if changed, err := m.SetHeader(&declared); err != nil || len(changed) != 0 {
		t.Fatalf("second run: changed %v, err %v; want nothing", changed, err)
	}
}

// DeclaresProperty follows the metamodel's version data: ReturnVariableName
// (Microflows$MicroflowBase, 10.12) is declared on the fixture's version, a
// property with no version data is declared, and without a project version
// nothing is.
func TestDeclaresProperty_FollowsTheProjectVersion(t *testing.T) {
	b, _ := nanoflowFixture(t)
	d := codecMicroflowDeps{b: b}
	if !d.DeclaresProperty("Microflows$Nanoflow", "ReturnVariableName") {
		t.Error("ReturnVariableName is not declared on a 10.12+ project")
	}
	if !d.DeclaresProperty("Microflows$Nanoflow", "Documentation") {
		t.Error("Documentation, which has no version data, is not declared")
	}
	if (codecMicroflowDeps{b: New()}).DeclaresProperty("Microflows$Nanoflow", "ReturnVariableName") {
		t.Error("a backend with no project version declares a property")
	}
}

// stripStoredKeyOnDisk removes a top-level key from a stored unit by rewriting
// its file directly. Going through the writer would put the key back: every
// write completes the property set Studio Pro writes (mendixlabs/mxcli#1373).
func stripStoredKeyOnDisk(t *testing.T, b *Backend, id model.ID, key string) {
	t.Helper()
	want, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(b.Path()), "mprcontents", "*", "*", "*.mxunit"))
	for _, m := range matches {
		raw, err := os.ReadFile(m)
		if err != nil || !bytes.Equal(raw, want) {
			continue
		}
		var d bson.D
		if err := bson.Unmarshal(raw, &d); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		out := d[:0]
		for _, e := range d {
			if e.Key != key {
				out = append(out, e)
			}
		}
		b2, err := bson.Marshal(out)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if err := os.WriteFile(m, b2, 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		return
	}
	t.Fatalf("no unit file holds %s", id)
}
