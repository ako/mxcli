// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

func noAction() bson.D {
	return bson.D{{Key: "$Type", Value: "Forms$NoAction"}, {Key: "DisabledDuringExecution", Value: true}}
}

// makeFileUploader builds a CustomWidget shaped like the File Uploader: two
// action-typed slots and one integer property. Every WidgetValue carries an
// Action field whatever its type — the builder's default value writes a
// NoAction into all of them — which is why the setter has to consult the
// property TYPE rather than the presence of the field.
func makeFileUploader(name string) bson.D {
	id := func(b byte) primitive.Binary {
		return primitive.Binary{Subtype: 0x04, Data: []byte{b, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}}
	}
	propType := func(b byte, key, valueType string) bson.D {
		return bson.D{
			{Key: "$ID", Value: id(b)},
			{Key: "PropertyKey", Value: key},
			{Key: "ValueType", Value: bson.D{{Key: "Type", Value: valueType}}},
		}
	}
	prop := func(b byte, primitiveValue string) bson.D {
		return bson.D{
			{Key: "TypePointer", Value: id(b)},
			{Key: "Value", Value: bson.D{
				{Key: "Action", Value: noAction()},
				{Key: "PrimitiveValue", Value: primitiveValue},
			}},
		}
	}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: name},
		{Key: "Type", Value: bson.D{
			{Key: "$Type", Value: "CustomWidgets$CustomWidgetType"},
			{Key: "ObjectType", Value: bson.D{
				{Key: "PropertyTypes", Value: bson.A{
					int32(2),
					propType(0xA1, "createFileAction", "Action"),
					propType(0xA2, "createImageAction", "Action"),
					propType(0xA3, "maxFileSize", "Integer"),
				}},
			}},
		}},
		{Key: "Object", Value: bson.D{
			{Key: "Properties", Value: bson.A{
				int32(2),
				prop(0xA1, ""),
				prop(0xA2, ""),
				prop(0xA3, "25"),
			}},
		}},
	}
}

// slotAction returns the Action stored on the pluggable property at index i.
func slotAction(t *testing.T, m *Mutator, widget string, i int) bson.D {
	t.Helper()
	result := m.widgetFinder(m.rawData, widget)
	if result == nil {
		t.Fatalf("widget %q not found", widget)
	}
	props := bsonnav.DGetArrayElements(bsonnav.DGet(bsonnav.DGetDoc(result.widget, "Object"), "Properties"))
	return bsonnav.DGetDoc(bsonnav.DGetDoc(props[i].(bson.D), "Value"), "Action")
}

var microflowMarker = bson.D{
	{Key: "$Type", Value: "Forms$MicroflowClientAction"},
	{Key: "Microflow", Value: "MyModule.ACT_CreateFile"},
}

// TestSetWidgetNamedAction_WritesTheSlot is the storage half of
// mendixlabs/mxcli#995: `set 'createFileAction' = microflow M.F on
// fileUploader1` has to land in that slot's Value.Action, the same place the
// CREATE path's widgetobj.Builder.SetAction writes it, and nowhere else.
func TestSetWidgetNamedAction_WritesTheSlot(t *testing.T) {
	for _, spelling := range []string{"createFileAction", "CreateFileAction"} {
		t.Run(spelling, func(t *testing.T) {
			m := New(makeRawPage(makeFileUploader("fileUploader1")), model.ID("u"),
				&stubActionDeps{serialized: microflowMarker})

			err := m.SetWidgetNamedAction("fileUploader1", spelling,
				&pages.MicroflowClientAction{MicroflowName: "MyModule.ACT_CreateFile"})
			if err != nil {
				t.Fatalf("SetWidgetNamedAction: %v", err)
			}
			if got := bsonnav.DGetString(slotAction(t, m, "fileUploader1", 0), "Microflow"); got != "MyModule.ACT_CreateFile" {
				t.Errorf("createFileAction = %q, want MyModule.ACT_CreateFile", got)
			}
			if got := bsonnav.DGetString(slotAction(t, m, "fileUploader1", 1), "$Type"); got != "Forms$NoAction" {
				t.Errorf("createImageAction = %q, want it untouched (Forms$NoAction)", got)
			}
		})
	}
}

// TestSetWidgetNamedAction_RefusesNonActionProperty is the guard. Every
// WidgetValue has an Action field, so writing one into an integer property
// would "succeed" and be ignored by Mendix — a set that reports success and
// changes nothing. It has to be an error that names the action slots.
func TestSetWidgetNamedAction_RefusesNonActionProperty(t *testing.T) {
	m := New(makeRawPage(makeFileUploader("fileUploader1")), model.ID("u"),
		&stubActionDeps{serialized: microflowMarker})

	err := m.SetWidgetNamedAction("fileUploader1", "maxFileSize",
		&pages.MicroflowClientAction{MicroflowName: "M.F"})
	if err == nil {
		t.Fatal("expected an error writing an action into an Integer property")
	}
	for _, want := range []string{"maxFileSize", "createFileAction", "createImageAction"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
	if got := bsonnav.DGetString(slotAction(t, m, "fileUploader1", 2), "$Type"); got != "Forms$NoAction" {
		t.Errorf("a refused set must not write: maxFileSize Action = %q", got)
	}
}

func TestSetWidgetNamedAction_UnknownKeyAndWidget(t *testing.T) {
	m := New(makeRawPage(makeFileUploader("fileUploader1")), model.ID("u"),
		&stubActionDeps{serialized: microflowMarker})
	if err := m.SetWidgetNamedAction("fileUploader1", "createFilAction",
		&pages.MicroflowClientAction{MicroflowName: "M.F"}); err == nil ||
		!strings.Contains(err.Error(), "createFilAction") {
		t.Errorf("unknown key: got %v, want an error naming it", err)
	}
	if err := m.SetWidgetNamedAction("noSuchWidget", "createFileAction",
		&pages.MicroflowClientAction{MicroflowName: "M.F"}); err == nil {
		t.Error("unknown widget: got nil error")
	}
}

// TestSetWidgetNamedAction_RefusesBuiltInWidget: a Forms$ActionButton has no
// pluggable slots; its click action is `set Action = …`.
func TestSetWidgetNamedAction_RefusesBuiltInWidget(t *testing.T) {
	btn := bson.D{{Key: "$Type", Value: "Forms$ActionButton"}, {Key: "Name", Value: "btnGo"}, {Key: "Action", Value: noAction()}}
	m := New(makeRawPage(btn), model.ID("u"), &stubActionDeps{serialized: microflowMarker})
	err := m.SetWidgetNamedAction("btnGo", "onClick", &pages.MicroflowClientAction{MicroflowName: "M.F"})
	if err == nil || !strings.Contains(err.Error(), "set Action") {
		t.Errorf("got %v, want a refusal pointing at `set Action`", err)
	}
}
