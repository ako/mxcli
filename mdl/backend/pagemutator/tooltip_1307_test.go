// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// mendixlabs/mxcli#1307: `alter page … set (Tooltip: '…') on btn` was refused
// with `property "Tooltip" is not a property of this built-in widget` — the
// key fell through to the pluggable setter. An action button stores its
// tooltip as a Texts$Text (empty when unset), the same shape as a tab caption.
func TestSetTooltip_ActionButton(t *testing.T) {
	withAuthoringLanguage(t, "en_US")
	btn := bson.D{
		{Key: "$Type", Value: "Forms$ActionButton"},
		{Key: "Name", Value: "btnEdit"},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
		{Key: "Tooltip", Value: textsText()},
	}
	raw := makeRawPage(btn)
	m := &Mutator{rawData: raw, widgetFinder: findBsonWidget}
	if err := m.SetWidgetProperty("btnEdit", "Tooltip", "Edit this order"); err != nil {
		t.Fatalf("set Tooltip: %v", err)
	}
	tip := bsonnav.DGetDoc(findBsonWidget(raw, "btnEdit").widget, "Tooltip")
	if got := translationsOf(t, tip); got["en_US"] != "Edit this order" || len(got) != 1 {
		t.Fatalf("tooltip translations = %v", got)
	}
}

// Control: a widget with no Tooltip slot is refused, not reported as altered.
func TestSetTooltip_WidgetWithoutTooltipIsRefused(t *testing.T) {
	cont := bson.D{
		{Key: "$Type", Value: "Forms$DivContainer"},
		{Key: "Name", Value: "box"},
		{Key: "Appearance", Value: bson.D{{Key: "$Type", Value: "Forms$Appearance"}}},
	}
	m := &Mutator{rawData: makeRawPage(cont), widgetFinder: findBsonWidget}
	err := m.SetWidgetProperty("box", "Tooltip", "x")
	if err == nil {
		t.Fatal("set Tooltip on a container returned nil; it stored nothing")
	}
	if !strings.Contains(err.Error(), "Tooltip") {
		t.Errorf("error does not name the property: %v", err)
	}
}
