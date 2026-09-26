// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
)

// `alter page … set DynamicClasses = [ if … then 'a' else 'b' ] on w` — the
// bracketed spelling mendixlabs/mxcli#750 proposes — reaches the mutator as a
// []string, because it parses as propertyValueV3's array alternative. The two
// expression setters handled that differently and both wrongly:
//
//   - DynamicClasses wrote only `if s, ok := value.(string)` and returned nil
//     otherwise: success reported, nothing written.
//   - a column's DynamicCellClass formatted the list with %v and wrote
//     `[if$currentObject/Featuredthen'a'else'b']` (the visitor had already fused
//     the tokens) into the Expression field: success reported, garbage stored.
//
// Refusing here also makes `check -p` report it, because validateAlterSetProperties
// dry-runs this setter and keeps its error.

func TestSetWidgetProperty_DynamicClassesRefusesAList(t *testing.T) {
	rawData := makeRawPage(makeStyleableWidget("ctn1"))
	m := &Mutator{rawData: rawData, widgetFinder: findBsonWidget}

	// Control first: a string is written, so the fixture can hold a value.
	const stored = "'kept'"
	if err := m.SetWidgetProperty("ctn1", "DynamicClasses", stored); err != nil {
		t.Fatalf("control: SetWidgetProperty(DynamicClasses, string) failed: %v", err)
	}

	err := m.SetWidgetProperty("ctn1", "DynamicClasses", []string{"if$currentObject/Featuredthen'a'else'b'"})
	if err == nil {
		t.Fatal("a bracketed list was accepted for DynamicClasses and reported as success")
	}
	if !strings.Contains(err.Error(), "without brackets") {
		t.Errorf("error = %q, want it to name the spelling that works", err)
	}
	app := bsonnav.DGetDoc(findBsonWidget(rawData, "ctn1").widget, "Appearance")
	if got := bsonnav.DGetString(app, "DynamicClasses"); got != stored {
		t.Errorf("Appearance.DynamicClasses = %q after a refused set, want %q unchanged", got, stored)
	}
}

func TestSetColumnProperty_ExpressionRefusesAList(t *testing.T) {
	col, keys, kinds := columnFixture()

	// Control: a string reaches the Expression field.
	if err := setColumnPropertyMut(col, keys, kinds, "DynamicCellClass", "'kept'"); err != nil {
		t.Fatalf("control: DynamicCellClass string rejected: %v", err)
	}

	err := setColumnPropertyMut(col, keys, kinds, "DynamicCellClass", []string{"if$currentObject/Featuredthen'a'else'b'"})
	if err == nil {
		t.Fatal("a bracketed list was accepted for DynamicCellClass and reported as success")
	}
	if !strings.Contains(err.Error(), "without brackets") {
		t.Errorf("error = %q, want it to name the spelling that works", err)
	}
	if got := fieldOf(t, col, idClass, "Expression"); got != "'kept'" {
		t.Errorf("Expression = %v after a refused set, want 'kept' unchanged", got)
	}
}
