// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// stubActionDeps serializes a client action to a marker document so the mutator
// test can assert what was written without pulling in an engine.
type stubActionDeps struct {
	Deps
	serialized bson.D
}

func (d *stubActionDeps) SerializeClientAction(a pages.ClientAction) bson.D {
	return d.serialized
}

// TestSetWidgetAction_ReplacesAction covers the FINDINGS gap: retargeting a
// button's action required REPLACEing the whole widget, which silently drops
// every property the author did not restate. Setting the action in place has to
// leave the rest of the widget alone.
func TestSetWidgetAction_ReplacesAction(t *testing.T) {
	btn := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnGo"},
		{Key: "Action", Value: bson.D{
			{Key: "$Type", Value: "Forms$MicroflowClientAction"},
			{Key: "Microflow", Value: "M.ACT_Save"},
		}},
		// The properties REPLACE would lose.
		{Key: "ButtonStyle", Value: "Primary"},
		{Key: "CaptionTemplate", Value: "Go"},
	}
	raw := makeRawPage(btn)
	newAction := bson.D{
		{Key: "$Type", Value: "Forms$MicroflowClientAction"},
		{Key: "Microflow", Value: "M.ACT_Other"},
	}
	m := New(raw, model.ID("unit-1"), &stubActionDeps{serialized: newAction})

	if err := m.SetWidgetAction("btnGo", &pages.MicroflowClientAction{MicroflowName: "M.ACT_Other"}); err != nil {
		t.Fatalf("SetWidgetAction: %v", err)
	}

	widget := m.widgetFinder(m.rawData, "btnGo")
	if widget == nil {
		t.Fatal("btnGo not found after mutation")
	}
	action := bsonnav.DGetDoc(widget.widget, "Action")
	if action == nil {
		t.Fatal("Action missing after mutation")
	}
	if got := bsonnav.DGet(action, "Microflow"); got != "M.ACT_Other" {
		t.Errorf("Microflow = %v, want M.ACT_Other", got)
	}
	// The whole point: everything else survives.
	if got := bsonnav.DGet(widget.widget, "ButtonStyle"); got != "Primary" {
		t.Errorf("ButtonStyle = %v, want Primary — a set must not disturb sibling properties", got)
	}
	if got := bsonnav.DGet(widget.widget, "CaptionTemplate"); got != "Go" {
		t.Errorf("CaptionTemplate = %v, want Go", got)
	}
}

// TestSetWidgetAction_RefusesWidgetWithoutAction is the guard-don't-drop half.
//
// Studio Pro resolves every stored property against the type's property list and
// throws on one it does not know, while mxbuild's deserializer tolerates it — so
// writing an Action onto a text would build clean and then fail to open.
// Refusing is the only safe answer, and the build is not a safety net here.
func TestSetWidgetAction_RefusesWidgetWithoutAction(t *testing.T) {
	text := bson.D{
		{Key: "$Type", Value: "Forms$DynamicText"},
		{Key: "Name", Value: "t1"},
	}
	raw := makeRawPage(text)
	m := New(raw, model.ID("unit-1"), &stubActionDeps{serialized: bson.D{{Key: "$Type", Value: "Forms$MicroflowClientAction"}}})

	err := m.SetWidgetAction("t1", &pages.MicroflowClientAction{MicroflowName: "M.ACT_Save"})
	if err == nil {
		t.Fatal("expected an error setting Action on a dynamic text")
	}
	for _, want := range []string{"t1", "Forms$DynamicText", "no on-click action"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
	// And nothing was written.
	widget := m.widgetFinder(m.rawData, "t1")
	if bsonnav.DGet(widget.widget, "Action") != nil {
		t.Error("Action was written to a widget that has no such property")
	}
}

// mendixlabs/mxcli#1592 (ako/mxcli#1503): a clickable container stores its
// on-click action as OnClickAction, and a list view or an image as
// ClickAction — never under "Action". SET looked only for "Action", so it
// refused the clickable container its own message named as a valid target.
// The new action has to land in the widget's own key, and no "Action" key may
// appear (Studio Pro throws on a property the type does not have).
func TestSetWidgetAction_WritesTheTypesOwnClickKey(t *testing.T) {
	for _, tc := range []struct{ typ, key string }{
		{"Forms$DivContainer", "OnClickAction"},
		{"Forms$ListView", "ClickAction"},
		{"Forms$StaticImageViewer", "ClickAction"},
		{"Forms$ImageViewer", "ClickAction"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			w := bson.D{
				{Key: "$Type", Value: tc.typ},
				{Key: "Name", Value: "w1"},
				{Key: tc.key, Value: bson.D{
					{Key: "$Type", Value: "Forms$MicroflowClientAction"},
					{Key: "Microflow", Value: "M.ACT_OpenOld"},
				}},
				{Key: "Class", Value: "card"},
			}
			newAction := bson.D{
				{Key: "$Type", Value: "Forms$MicroflowClientAction"},
				{Key: "Microflow", Value: "M.ACT_OpenNew"},
			}
			m := New(makeRawPage(w), model.ID("unit-1"), &stubActionDeps{serialized: newAction})
			if err := m.SetWidgetAction("w1", &pages.MicroflowClientAction{MicroflowName: "M.ACT_OpenNew"}); err != nil {
				t.Fatalf("SetWidgetAction: %v", err)
			}
			got := m.widgetFinder(m.rawData, "w1").widget
			if mf := bsonnav.DGet(bsonnav.DGetDoc(got, tc.key), "Microflow"); mf != "M.ACT_OpenNew" {
				t.Errorf("%s.Microflow = %v, want M.ACT_OpenNew", tc.key, mf)
			}
			if bsonnav.DGet(got, "Action") != nil {
				t.Errorf("an Action key was added to %s, which has no such property", tc.typ)
			}
			if bsonnav.DGet(got, "Class") != "card" {
				t.Error("a sibling property was disturbed")
			}
		})
	}
}

// TestSetWidgetAction_UnknownWidget keeps the not-found path a clear error.
func TestSetWidgetAction_UnknownWidget(t *testing.T) {
	raw := makeRawPage()
	m := New(raw, model.ID("unit-1"), &stubActionDeps{})
	err := m.SetWidgetAction("nope", &pages.MicroflowClientAction{MicroflowName: "M.ACT_Save"})
	if err == nil {
		t.Fatal("expected an error for an unknown widget")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q does not name the widget", err.Error())
	}
}

// Compile-time reminder that the Mutator still satisfies the backend interface
// the new method was added to.
var _ backend.PageMutator = (*Mutator)(nil)
