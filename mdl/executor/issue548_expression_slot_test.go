// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// ako/mxcli#548: a pluggable widget's expression-typed property was written
// with its value in PrimitiveValue as well as in Expression. Studio Pro never
// does that: across ako/TestApp and PedApp (221 pages, snippets and layouts),
// every one of 939 Expression-typed WidgetValues stores PrimitiveValue "", and
// no other value type stores an Expression. So the first describe → exec of a
// page mxcli wrote was never "Unchanged" against one Studio Pro saved, and
// mxcli's own output differed from Studio Pro's on every combo box
// (customEditabilityExpression) — the property was hidden, and reset to its
// declared default with the primitive setter.

const issue548Page = `create or modify page MyFirstModule.Things548 (Title: 'Things', Layout: Atlas_Core.Atlas_Default, Params: ($Thing: MyFirstModule.Thing)) {
  dataview dvThing (DataSource: $Thing) {
    combobox cbColour (Label: 'Colour', Attribute: Colour)
  }
  datagrid dgThings (DataSource: database from MyFirstModule.Thing) {
    column (Attribute: Name, Caption: 'Name')
  }
  gallery galThings (DataSource: database from MyFirstModule.Thing) {
    dynamictext txtName (Content: '{1}', ContentParams: ({1} = Name))
  }
  image imgLogo (ImageType: 'imageUrl', ImageUrl: 'https://example.com/logo.png')
};`

// expressionSlots lists, for every pluggable widget in doc, the Expression-
// typed properties whose stored value fills PrimitiveValue.
func expressionSlots(doc bson.D) (expressionProps int, both []string) {
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if bsonString(n, "$Type") == "CustomWidgets$CustomWidget" {
				types := map[string][2]string{} // property type id -> key, value type
				var collect func(ot bson.D)
				collect = func(ot bson.D) {
					for _, pt := range bsonDocs(bsonValue(ot, "PropertyTypes")) {
						vt := bsonDocField(pt, "ValueType")
						types[bsonIDKey(bsonValue(pt, "$ID"))] = [2]string{bsonString(pt, "PropertyKey"), bsonString(vt, "Type")}
						collect(bsonDocField(vt, "ObjectType"))
					}
				}
				collect(bsonDocField(bsonDocField(n, "Type"), "ObjectType"))
				var obj func(o bson.D)
				obj = func(o bson.D) {
					for _, p := range bsonDocs(bsonValue(o, "Properties")) {
						kt := types[bsonIDKey(bsonValue(p, "TypePointer"))]
						val := bsonDocField(p, "Value")
						if kt[1] == "Expression" {
							expressionProps++
							if pv := bsonString(val, "PrimitiveValue"); pv != "" {
								both = append(both, fmt.Sprintf("%s.%s PrimitiveValue=%q Expression=%q",
									bsonString(n, "Name"), kt[0], pv, bsonString(val, "Expression")))
							}
						}
						for _, item := range bsonDocs(bsonValue(val, "Objects")) {
							obj(item)
						}
					}
				}
				obj(bsonDocField(n, "Object"))
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	return expressionProps, both
}

func TestIssue548_ExpressionPropertyIsNotWrittenAsAPrimitive(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, `create enumeration MyFirstModule.Colour (Red 'Red', Blue 'Blue');
create persistent entity MyFirstModule.Thing (Colour: Enumeration(MyFirstModule.Colour), Name: String(100));`)
	run(t, exec, issue548Page)
	doc := storedUnitDoc(t, exec, unitIDByName(t, exec, "page", "Things548"))

	n, both := expressionSlots(doc)
	if n < 4 {
		t.Fatalf("found only %d Expression-typed properties, so the page shows nothing", n)
	}
	if len(both) > 0 {
		t.Errorf("%d of %d Expression-typed properties store their value in PrimitiveValue too:\n  %s",
			len(both), n, strings.Join(both, "\n  "))
	}
	// The hidden combo-box property the report names keeps its declared
	// default, in the Expression slot.
	cb := namedDoc(t, doc, "cbColour")
	v := widgetProp(t, cb, "customEditabilityExpression")
	if got := bsonString(v, "Expression"); got != "false" {
		t.Errorf("customEditabilityExpression Expression = %q, want its declared default false", got)
	}
}
