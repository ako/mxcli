// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/settingsoverlay"
	"github.com/mendixlabs/mxcli/modelsdk/bsoncompat"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// seedFlatConstantValue gives the fixture's Default configuration one constant
// override in the shape Mendix stores below 10.9 (ako/mxcli#1163; `mx convert`
// 9.24.30): `{$ID, $Type, ConstantId, Value}`, the value a flat string.
func seedFlatConstantValue(t *testing.T, proj, constantID, value string) {
	t.Helper()
	r, err := mmpr.OpenWithOptions(proj, mmpr.OpenOptions{ReadOnly: false})
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer r.Close()
	refs, err := r.ListUnitsByType("Settings$ProjectSettings")
	if err != nil || len(refs) != 1 {
		t.Fatalf("ListUnitsByType(Settings$ProjectSettings) = %v, %v", refs, err)
	}
	raw, err := r.GetRawUnitBytes(refs[0].ID)
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var doc bson.M
	if err := bsoncompat.UnmarshalDocM(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	seeded := false
	for _, part := range settingsoverlay.ArrayElements(doc["Settings"]) {
		if part["$Type"] != "Settings$ConfigurationSettings" {
			continue
		}
		cfgs := settingsoverlay.ArrayElements(part["Configurations"])
		if len(cfgs) == 0 {
			t.Fatalf("fixture has no configurations to seed")
		}
		cfgs[0]["ConstantValues"] = bson.A{int32(3), bson.D{
			{Key: "$ID", Value: bsonutil.NewIDBsonBinary()},
			{Key: "$Type", Value: "Settings$ConstantValue"},
			{Key: "ConstantId", Value: constantID},
			{Key: "Value", Value: value},
		}}
		seeded = true
	}
	if !seeded {
		t.Fatalf("fixture has no Settings$ConfigurationSettings part")
	}
	contents, err := bson.Marshal(bsonutil.OrderStorageValue(doc))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := mmpr.NewWriterWithReader(r).UpdateRawUnit(refs[0].ID, contents); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
}

// A Mendix 9 constant override is read from its flat Value, and updating it
// keeps that shape rather than introducing the nested one 9.x does not have.
func TestProjectSettings_Mendix9FlatConstantValue(t *testing.T) {
	proj := copyFixture(t)
	const id = "MyFirstModule.ServerUrl"
	seedFlatConstantValue(t, proj, id, "kafka:9092")

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	ps, err := b.GetProjectSettings()
	if err != nil {
		t.Fatalf("GetProjectSettings: %v", err)
	}
	cvs := ps.Configuration.Configurations[0].ConstantValues
	if len(cvs) != 1 || cvs[0].ConstantId != id || cvs[0].Value != "kafka:9092" {
		t.Fatalf("flat override read as %+v, want %s = kafka:9092", cvs, id)
	}

	cvs[0].Value = "broker:9092"
	if err := b.UpdateProjectSettings(ps); err != nil {
		t.Fatalf("UpdateProjectSettings: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	after := settingsoverlay.ArrayElements(readConfiguration(t, proj)["ConstantValues"])
	if len(after) != 1 || after[0]["Value"] != "broker:9092" {
		t.Fatalf("flat override after update = %#v, want Value broker:9092", after)
	}
	if _, nested := after[0]["SharedOrPrivateValue"]; nested {
		t.Errorf("updating a flat override added the nested shape: %#v", after[0])
	}
}
