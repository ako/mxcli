// SPDX-License-Identifier: Apache-2.0

package pagemutator

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// `set (ShowContentAs: attribute) on dg column(Name)` on a column created as
// dynamicText left the column's dynamicText template filled ('Item {1}', one
// parameter). Data grid 2 hides it unless showContentAs is "dynamicText"
// (editorConfig: `"dynamicText"!==n.showContentAs&&hidePropertyIn(…,"columns",
// o,"dynamicText")`), CREATE stores a hidden template null, and the project as
// stored failed with CE0463 at the grid — measured on 11.13.0 with
// `docker check --no-update-widgets`: 0 errors before the SET, 1 after, and 0
// for the control column created as attribute. The column reset handled
// primitives only; templates are now brought in line as they are for an
// object-list item.

var (
	tpDGCols    = idBin(0x40)
	tpDGAttr    = idBin(0x41)
	tpDGShowAs  = idBin(0x42)
	tpDGDynText = idBin(0x43)
	dynTextRule = []types.WidgetVisibilityRule{{PropertyKey: "dynamicText", ListPropertyKey: "columns",
		HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "showContentAs", Operator: "ne", Value: "dynamicText", Scope: types.ConditionScopeItem}}}
)

// dynTextGrid is grid dg with one column over Name, in mode showAs, its
// dynamicText template filled when filled is set and null otherwise.
func dynTextGrid(showAs string, filled bool) bson.D {
	var tmpl any
	if filled {
		tmpl = textTemplate("Item {1}")
	}
	pt := func(id bson.Binary, key, kind string) bson.D {
		return bson.D{{Key: "$ID", Value: id}, {Key: "PropertyKey", Value: key},
			{Key: "ValueType", Value: bson.D{{Key: "Type", Value: kind}, {Key: "DefaultValue", Value: ""}}}}
	}
	prop := func(id bson.Binary, value bson.D) bson.D {
		return bson.D{{Key: "TypePointer", Value: id}, {Key: "Value", Value: value}}
	}
	col := bson.D{{Key: "$Type", Value: "CustomWidgets$WidgetObject"}, {Key: "Properties", Value: bson.A{int32(2),
		prop(tpDGAttr, bson.D{{Key: "AttributeRef", Value: bson.D{{Key: "Attribute", Value: "G.Item.Name"}}}}),
		prop(tpDGShowAs, bson.D{{Key: "PrimitiveValue", Value: showAs}}),
		prop(tpDGDynText, bson.D{{Key: "TextTemplate", Value: tmpl}}),
	}}}
	return bson.D{
		{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
		{Key: "Name", Value: "dg"},
		{Key: "Type", Value: bson.D{
			{Key: "WidgetId", Value: "com.mendix.widget.web.datagrid.Datagrid"},
			{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{int32(2), bson.D{
				{Key: "$ID", Value: tpDGCols},
				{Key: "PropertyKey", Value: "columns"},
				{Key: "ValueType", Value: bson.D{{Key: "Type", Value: "Object"}, {Key: "IsList", Value: true},
					{Key: "ObjectType", Value: bson.D{{Key: "PropertyTypes", Value: bson.A{int32(2),
						pt(tpDGAttr, "attribute", "Attribute"),
						pt(tpDGShowAs, "showContentAs", "Enumeration"),
						pt(tpDGDynText, "dynamicText", "TextTemplate"),
					}}}}}},
			}}}}},
		}},
		{Key: "Object", Value: bson.D{{Key: "Properties", Value: bson.A{int32(2),
			prop(tpDGCols, bson.D{{Key: "Objects", Value: bson.A{int32(2), col}}}),
		}}}},
	}
}

func dynTextTemplate(t *testing.T, m *Mutator) any {
	t.Helper()
	col, err := findBsonColumn(m.rawData, "dg", "Name", m.widgetFinder)
	if err != nil {
		t.Fatal(err)
	}
	return bsonnav.DGet(itemPropValue(col.widget, col.colPropKeys, "dynamicText"), "TextTemplate")
}

func TestColumnSetNullsTheTemplateItHides(t *testing.T) {
	m := New(makeRawPage(dynTextGrid("dynamicText", true)), model.ID("u"), &templateDeps{})
	col, err := findBsonColumn(m.rawData, "dg", "Name", m.widgetFinder)
	if err != nil {
		t.Fatal(err)
	}
	if err := setColumnPropertyMut(col.widget, col.colPropKeys, col.colPropKinds, "showContentAs", "attribute"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ResetHiddenColumnProperties("dg", "Name", dynTextRule, []string{"ShowContentAs"}); err != nil {
		t.Fatalf("ResetHiddenColumnProperties: %v", err)
	}
	if tmpl := dynTextTemplate(t, m); tmpl != nil {
		t.Errorf("dynamicText is hidden in attribute mode and must be null, got %v", tmpl)
	}
}

// The other way round: switching to dynamicText reveals a null template, and
// the stored schema has no default text for it, so the SET must say so.
func TestColumnSetRefusesARevealedTemplateWithNoText(t *testing.T) {
	m := New(makeRawPage(dynTextGrid("attribute", false)), model.ID("u"), &templateDeps{})
	col, _ := findBsonColumn(m.rawData, "dg", "Name", m.widgetFinder)
	if err := setColumnPropertyMut(col.widget, col.colPropKeys, col.colPropKinds, "showContentAs", "dynamicText"); err != nil {
		t.Fatal(err)
	}
	_, err := m.ResetHiddenColumnProperties("dg", "Name", dynTextRule, []string{"ShowContentAs"})
	if err == nil || !strings.Contains(err.Error(), "dynamicText") {
		t.Fatalf("want a refusal asking for the text, got %v", err)
	}
}

// With the text in the same statement it works: `set (ShowContentAs: dynamicText,
// dynamicText: 'Item {1}')` creates the template and writes the text. (`Content:` in a
// column SET names the custom-content slot, not this text — a separate quirk.)
func TestColumnSetWithTextCreatesTheTemplate(t *testing.T) {
	m := New(makeRawPage(dynTextGrid("attribute", false)), model.ID("u"), &templateDeps{})
	if err := m.SetColumnProperty("dg", "Name", "ShowContentAs", "dynamicText"); err != nil {
		t.Fatal(err)
	}
	if err := m.SetColumnProperty("dg", "Name", "dynamicText", "Item {1}"); err != nil {
		t.Fatalf("set dynamicText on a null template: %v", err)
	}
	if _, err := m.ResetHiddenColumnProperties("dg", "Name", dynTextRule, []string{"ShowContentAs", "dynamicText"}); err != nil {
		t.Fatalf("ResetHiddenColumnProperties: %v", err)
	}
	if dynTextTemplate(t, m) == nil {
		t.Error("dynamicText template was not created")
	}
}
