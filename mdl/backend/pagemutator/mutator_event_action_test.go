// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2602: `alter page … set (OnChange: …) on txtAmount` was refused
// as "not a pluggable widget", though CREATE PAGE writes and DESCRIBE prints
// the text box's own OnChangeAction. A built-in input's event actions are
// written into the key the stored widget holds, by the MDL name or the stored
// one, as CREATE PAGE reads them.

func textBoxWithEvents(name string, keys ...string) bson.D {
	w := bson.D{{Key: "$Type", Value: "Forms$TextBox"}, {Key: "Name", Value: name}}
	for _, k := range keys {
		w = append(w, bson.E{Key: k, Value: noAction()})
	}
	return w
}

func storedAction(t *testing.T, m *Mutator, widget, key string) bson.D {
	t.Helper()
	result := m.widgetFinder(m.rawData, widget)
	if result == nil {
		t.Fatalf("widget %q not found", widget)
	}
	return bsonnav.DGetDoc(result.widget, key)
}

func TestSetWidgetNamedAction_BuiltInInputEvents_2602(t *testing.T) {
	all := []string{"OnChangeAction", "OnEnterAction", "OnLeaveAction", "OnEnterKeyPressAction"}
	for _, c := range []struct{ key, stored string }{
		{"OnChange", "OnChangeAction"},
		{"onchange", "OnChangeAction"},
		{"OnChangeAction", "OnChangeAction"},
		{"OnEnter", "OnEnterAction"},
		{"OnLeave", "OnLeaveAction"},
		{"OnEnterKeyPress", "OnEnterKeyPressAction"},
	} {
		t.Run(c.key, func(t *testing.T) {
			m := New(makeRawPage(textBoxWithEvents("txtAmount", all...)), model.ID("u"), &stubActionDeps{serialized: microflowMarker})
			if err := m.SetWidgetNamedAction("txtAmount", c.key, &pages.MicroflowClientAction{MicroflowName: "M.F"}); err != nil {
				t.Fatalf("set %s: %v", c.key, err)
			}
			for _, k := range all {
				got := bsonnav.DGetString(storedAction(t, m, "txtAmount", k), "$Type")
				want := "Forms$NoAction"
				if k == c.stored {
					want = "Forms$MicroflowClientAction"
				}
				if got != want {
					t.Errorf("%s = %s, want %s", k, got, want)
				}
			}
		})
	}
}

func TestSetWidgetNamedAction_BuiltInWithoutTheEvent_2602(t *testing.T) {
	// A text area has no OnEnterKeyPressAction: refused with that reason, and
	// nothing written under a key Studio Pro would not know.
	ta := bson.D{{Key: "$Type", Value: "Forms$TextArea"}, {Key: "Name", Value: "taNotes"}, {Key: "OnChangeAction", Value: noAction()}}
	m := New(makeRawPage(ta), model.ID("u"), &stubActionDeps{serialized: microflowMarker})
	err := m.SetWidgetNamedAction("taNotes", "OnEnterKeyPress", &pages.MicroflowClientAction{MicroflowName: "M.F"})
	if err == nil || !strings.Contains(err.Error(), "OnEnterKeyPress") || !strings.Contains(err.Error(), "OnChange") {
		t.Fatalf("got %v, want a refusal naming the missing event and the ones the widget has", err)
	}
	if storedAction(t, m, "taNotes", "OnEnterKeyPressAction") != nil {
		t.Error("wrote OnEnterKeyPressAction on a text area")
	}
}
