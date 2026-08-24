// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

// TestCommitAndRefreshFlagsReachStorage proves #895 at the layer the symptom
// lives in: the bytes mxcli writes.
//
// Everything above this layer can look right while the document is wrong, and
// on this bug everything did — `mxcli check`, `mxcli lint`, Studio Pro's
// consistency check and `mxbuild` all passed against a model whose commits
// silently skipped their event handlers. So the assertion is on the stored
// BSON, not on the semantic model that produced it.
//
// The reference values are measured, not assumed: a Commit activity dropped
// into Studio Pro with its properties untouched stores WithEvents=true /
// RefreshInClient=false, and its deliberately-inverted sibling stores the
// opposite (ako/TestApp module CommitActivity, Mendix 11.13). Both keys are
// present in both documents, so neither is an omission Mendix fills in on load.
func TestCommitAndRefreshFlagsReachStorage(t *testing.T) {
	proj := copyFixture(t)

	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	mf := &microflows.Microflow{
		ContainerID: mod.ID,
		Name:        "ZZ_CommitFlags",
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				actionActivity(&microflows.CommitObjectsAction{CommitVariable: "Obj", WithEvents: true}),
				actionActivity(&microflows.CommitObjectsAction{CommitVariable: "Obj"}),
				actionActivity(&microflows.DeleteObjectAction{DeleteVariable: "Obj", RefreshInClient: true}),
				actionActivity(&microflows.CreateObjectAction{
					OutputVariable:      "New",
					EntityQualifiedName: "MyFirstModule.Car",
					RefreshInClient:     true,
				}),
			},
		},
	}
	if err := b.CreateMicroflow(mf); err != nil {
		t.Fatalf("CreateMicroflow: %v", err)
	}
	if err := b.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	got := collectActionProps(t, readUnitBytes(t, proj, string(mf.ID)))

	want := []map[string]any{
		{"$Type": "Microflows$CommitAction", "WithEvents": true, "RefreshInClient": false},
		{"$Type": "Microflows$CommitAction", "WithEvents": false, "RefreshInClient": false},
		{"$Type": "Microflows$DeleteAction", "RefreshInClient": true},
		{"$Type": "Microflows$CreateChangeAction", "RefreshInClient": true},
	}
	if len(got) != len(want) {
		t.Fatalf("stored %d actions, want %d: %#v", len(got), len(want), got)
	}
	for i, w := range want {
		for k, v := range w {
			stored, ok := got[i][k]
			if !ok {
				t.Errorf("action %d (%s): %s missing from the stored document", i, w["$Type"], k)
				continue
			}
			if stored != v {
				t.Errorf("action %d (%s): %s = %#v, want %#v", i, w["$Type"], k, stored, v)
			}
		}
	}
}

func actionActivity(a microflows.MicroflowAction) *microflows.ActionActivity {
	return &microflows.ActionActivity{
		BaseActivity: microflows.BaseActivity{
			BaseMicroflowObject: microflows.BaseMicroflowObject{
				BaseElement: model.BaseElement{ID: model.ID(types.GenerateID())},
			},
		},
		Action: a,
	}
}

// collectActionProps walks the stored unit and returns one flat map per object
// action, in document order.
func collectActionProps(t *testing.T, raw []byte) []map[string]any {
	t.Helper()

	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal unit: %v", err)
	}

	interesting := map[string]bool{
		"Microflows$CommitAction":       true,
		"Microflows$DeleteAction":       true,
		"Microflows$CreateChangeAction": true,
	}

	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.M:
			if t, _ := n["$Type"].(string); interesting[t] {
				out = append(out, map[string]any(n))
			}
			for _, child := range n {
				walk(child)
			}
		case bson.D:
			m := bson.M{}
			for _, e := range n {
				m[e.Key] = e.Value
			}
			walk(m)
		case bson.A:
			for _, child := range n {
				walk(child)
			}
		}
	}
	walk(doc)
	return out
}
