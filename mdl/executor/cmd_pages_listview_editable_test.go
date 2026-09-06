// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// ako/CapTrackV3 FINDINGS §6. Every input widget inside an authored list view
// rendered as `<div class="form-control-static">` — a value, not a field.
//
// It reads as a styling bug or a missing entity-access grant and is neither:
// `show access on entity` reported ReadWrite for the role and `mx check` reported
// 0 errors. Mendix's List View has an `Editable` property of its OWN (default
// No), and that is what makes the inputs inside it editable; `Editable:` on the
// individual textbox is a different property, and the list view's read-only
// context wins over it.
//
// The parser accepted `Editable: true`, buildListViewV3 dropped it, and the
// writer wrote false — so the property never existed as far as the model was
// concerned, with every gate green.

func editableListView(props map[string]any) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: "listview", Name: "lvRows", Properties: props}
}

func listViewPB() *pageBuilder {
	return &pageBuilder{
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
	}
}

func TestListView_EditableIsCarriedToTheModel(t *testing.T) {
	pb := listViewPB()
	lv, err := pb.buildListViewV3(editableListView(map[string]any{"Editable": true}))
	if err != nil {
		t.Fatalf("buildListViewV3: %v", err)
	}
	if !lv.Editable {
		t.Error("`Editable: true` was dropped — every input inside the list view renders read-only, " +
			"with entity access ReadWrite and mx check at 0 errors")
	}
}

// CONTROL: the default. Mendix's own default is No, so a list view that does not
// ask for it must not become editable — otherwise this fix trades one silent
// wrong answer for its mirror image.
func TestListView_EditableDefaultsToFalse(t *testing.T) {
	pb := listViewPB()
	lv, err := pb.buildListViewV3(editableListView(map[string]any{}))
	if err != nil {
		t.Fatalf("buildListViewV3: %v", err)
	}
	if lv.Editable {
		t.Error("a list view with no Editable property came out editable")
	}
}

// CONTROL: an explicit false is still false, so `Editable: false` is not read as
// "unset and therefore whatever the builder feels like".
func TestListView_ExplicitFalseStaysFalse(t *testing.T) {
	pb := listViewPB()
	lv, err := pb.buildListViewV3(editableListView(map[string]any{"Editable": false}))
	if err != nil {
		t.Fatalf("buildListViewV3: %v", err)
	}
	if lv.Editable {
		t.Error("`Editable: false` came out true")
	}
}
