// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// upstream #978: `describe page` emitted MDL that mxcli's own
// `check --references` rejected.
//
//	- duplicate widget name 'row1' (used 2 times) … (CE0495)
//	- duplicate widget name 'col1' (used 2 times) … (CE0495)
//	- duplicate widget name 'FullName' (used 2 times) … (CE0495)
//
// on two stock Administration pages, unmodified. Two mechanisms, and the fix is
// different for each.
//
// The names are DERIVED at describe time. Measured on the stored page:
// Forms$LayoutGridRow and Forms$LayoutGridColumn carry no `Name` key at all, so
// Mendix has no name to make unique and CE0495 cannot be what it does about
// them — the rule was citing a Mendix error for something Mendix does not
// check. A DataGrid2 column is the same: mxcli's own MDL-WIDGET16 already says
// "DataGrid 2 stores no column names", and derives one from the bound attribute
// so ALTER PAGE has something to address. Two columns over the same attribute
// therefore derive the same name, and no renaming can fix that without breaking
// the documented addressing.
//
// So the check stops claiming CE0495 for names the model does not store.
//
// The triage of this issue predicted the opposite — that DESCRIBE should number
// rows and columns uniquely and that exempting them was the weaker option.
// Measuring changed the answer twice over: Mendix stores no name to make unique,
// and ALTER PAGE already handles a repeated derived column name, erroring with
// "column %q is ambiguous … qualify the reference as `ON gridName.%s`". Renaming
// would break that documented addressing to fix a collision that is not one.

func namedWidget(kind, name string, children ...*ast.WidgetV3) *ast.WidgetV3 {
	return &ast.WidgetV3{Type: kind, Name: name, Children: children}
}

// The reported case: duplicates among widgets whose names Mendix does not store.
func TestCheckDuplicateWidgetNames_IgnoresUnnamedWidgetKinds(t *testing.T) {
	page := []*ast.WidgetV3{
		namedWidget("layoutgrid", "lg1",
			namedWidget("row", "row1", namedWidget("column", "col1")),
		),
		namedWidget("layoutgrid", "lg2",
			namedWidget("row", "row1", namedWidget("column", "col1")),
		),
		namedWidget("datagrid", "grid1",
			namedWidget("column", "FullName"),
			namedWidget("column", "FullName"),
		),
	}

	if errs := checkDuplicateWidgetNames(page); len(errs) != 0 {
		t.Errorf("a widget kind whose name is not stored cannot be a CE0495 duplicate; got:\n  %s",
			strings.Join(errs, "\n  "))
	}
}

// The control, and the reason the rule exists at all (FINDINGS #15): a real
// duplicate among widgets Mendix DOES name must still be reported.
func TestCheckDuplicateWidgetNames_StillCatchesRealDuplicates(t *testing.T) {
	page := []*ast.WidgetV3{
		namedWidget("layoutgrid", "lg1",
			namedWidget("row", "row1",
				namedWidget("column", "col1",
					namedWidget("textbox", "txtName"),
					namedWidget("textbox", "txtName"),
				),
			),
		),
	}

	errs := checkDuplicateWidgetNames(page)
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1:\n  %s", len(errs), strings.Join(errs, "\n  "))
	}
	if !strings.Contains(errs[0], "txtName") || !strings.Contains(errs[0], "CE0495") {
		t.Errorf("the message should name the widget and CE0495, got: %s", errs[0])
	}
}

// A named widget colliding with a derived name is still a real collision: the
// exemption is per widget KIND, not per name.
func TestCheckDuplicateWidgetNames_NamedWidgetCollidingWithADerivedName(t *testing.T) {
	page := []*ast.WidgetV3{
		namedWidget("layoutgrid", "lg1", namedWidget("row", "row1")),
		namedWidget("container", "row1"),
		namedWidget("container", "row1"),
	}

	errs := checkDuplicateWidgetNames(page)
	if len(errs) != 1 || !strings.Contains(errs[0], "row1") {
		t.Fatalf("two containers named row1 are a real duplicate; got %d errors:\n  %s",
			len(errs), strings.Join(errs, "\n  "))
	}
	// …and the count must not include the layout row, which Mendix does not name.
	if strings.Contains(errs[0], "3 times") {
		t.Errorf("the unnamed layout row was counted as an occurrence: %s", errs[0])
	}
}
