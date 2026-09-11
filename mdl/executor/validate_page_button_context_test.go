// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// buttonContextMessages parses MDL and runs the control-bar button check.
func buttonContextMessages(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var msgs []string
	for _, v := range ValidatePageButtonContext(prog) {
		msgs = append(msgs, v.Message)
	}
	return msgs
}

// A control-bar button passing $currentObject is flagged (MDL-BUTTON01 / CE1571).
func TestValidatePageButtonContext_ControlBarCurrentObjectFlagged(t *testing.T) {
	src := `create page P.Grid ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgOrders ( DataSource: database from P.Order ) {
    controlbar cb1 {
      actionbutton btnEdit (Caption: 'Edit', Action: show_page P.Detail (Order: $currentObject))
    }
    column OrderNumber (Attribute: OrderNumber) { }
  }
};`
	msgs := buttonContextMessages(t, src)
	if len(msgs) != 1 || !strings.Contains(msgs[0], "btnEdit") || !strings.Contains(msgs[0], "$currentObject") {
		t.Fatalf("expected one control-bar $currentObject warning for btnEdit, got %v", msgs)
	}
}

// The same button inside a grid COLUMN (row-scoped) is clean.
func TestValidatePageButtonContext_ColumnCurrentObjectClean(t *testing.T) {
	src := `create page P.Grid ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgOrders ( DataSource: database from P.Order ) {
    column OrderNumber (Attribute: OrderNumber, ShowContentAs: customContent) {
      actionbutton btnEdit (Caption: 'Edit', Action: show_page P.Detail (Order: $currentObject))
    }
  }
};`
	if msgs := buttonContextMessages(t, src); len(msgs) != 0 {
		t.Fatalf("row-scoped column button should be clean, got %v", msgs)
	}
}

// A control-bar button that does NOT pass $currentObject is clean.
func TestValidatePageButtonContext_ControlBarNoCurrentObjectClean(t *testing.T) {
	src := `create page P.Grid ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgOrders ( DataSource: database from P.Order ) {
    controlbar cb1 {
      actionbutton btnNew (Caption: 'New', Action: show_page P.Detail)
    }
    column OrderNumber (Attribute: OrderNumber) { }
  }
};`
	if msgs := buttonContextMessages(t, src); len(msgs) != 0 {
		t.Fatalf("control-bar button without $currentObject should be clean, got %v", msgs)
	}
}

// The advice has to name the remedy that applies where the button IS.
// mendixlabs/mxcli#1082 was filed by an author who followed the old suggestion,
// found neither a column nor a page parameter usable, and concluded MDL had no
// syntax for passing the grid's selection — which it has had all along.
func TestValidatePageButtonContext_SuggestionNamesTheGridSelection(t *testing.T) {
	src := `create page P.Grid ( Title: 'Orders', Layout: Atlas_Core.Atlas_Default ) {
  datagrid dgMaterials ( DataSource: database from P.Order, Selection: Single ) {
    controlbar cb1 {
      container cUnlink (Action: nanoflow P.ACT_UnLink ($Order = $currentObject)) {
        actionbutton btnUnlink (Caption: 'Unlink')
      }
    }
    column OrderNumber (Attribute: OrderNumber) { }
  }
};`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	vs := ValidatePageButtonContext(prog)
	if len(vs) != 1 {
		t.Fatalf("expected one violation, got %d: %v", len(vs), vs)
	}
	// The selection is addressed by the DATA WIDGET's name, so the suggestion is
	// only useful if it carries that name through from the walk.
	if !strings.Contains(vs[0].Suggestion, "$dgMaterials") {
		t.Errorf("suggestion should name the grid's selection variable:\n%s", vs[0].Suggestion)
	}
	// …and the container is the widget at fault, not the button inside it.
	if !strings.Contains(vs[0].Message, "cUnlink") {
		t.Errorf("message should name the widget carrying the action:\n%s", vs[0].Message)
	}
}
