// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// The example writes one sub-property on each object-list item to show the
// shape. It wrote `'…'` — a literal ellipsis — and mxcli's own MDL-WIDGET08
// then rejected it:
//
//	widget `item2` (series) property `dataSet` has invalid value `…`
//	  — valid values are static, dynamic
//
// 11 across the fixture's 42 widgets. `'…'` reads as a placeholder to a person
// and is simply a wrong value to the checker, so the example was not runnable
// as printed even though it parsed — and "parses as written" is exactly what
// the block claims.
//
// The values are already in hand: propsFromMPK carries an object-list
// property's item properties as Children, with their enums and defaults, so
// exampleLiteral can pick a real one from the same data.
//
// The assertion is "no MDL-WIDGET08", NOT "no ellipsis anywhere". A free-text
// sub-property has no correct value to invent, and the validator accepts any
// string, so a placeholder there is the honest output — narrowing the rule to
// what the checker actually rejects keeps the test about the defect rather than
// about a character.
func TestGeneratedExamplesUseRealItemValues(t *testing.T) {
	project := fixtureProject(t)
	registry := LoadWidgetRegistry(project)
	if registry == nil {
		t.Fatal("no registry")
	}

	var offenders []string
	var validated, withItems int
	for _, def := range registry.All() {
		if def == nil || def.MDLName == "" {
			continue
		}
		desc, err := DescribeWidget(def.MDLName, project)
		if err != nil || desc == nil || desc.Example == "" {
			continue
		}
		src := "create page Probe.P_" + def.MDLName +
			" (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" + desc.Example + "\n}\n"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 || prog == nil {
			continue
		}
		for _, stmt := range prog.Statements {
			for _, v := range ValidateWidgetPropertiesForStatement(stmt, registry) {
				if v.RuleID == "MDL-WIDGET08" {
					offenders = append(offenders, def.MDLName+": "+v.Message)
				}
			}
		}
		validated++
		for _, c := range desc.Containers {
			if c.Kind == "object list" && c.Authorable && len(c.ItemKeys) > 0 {
				withItems++
			}
		}
	}

	// Without this the assertion is satisfied by a build where no example has an
	// object list at all — so when that is the case, this test proves nothing and
	// says so rather than passing quietly.
	//
	// It SKIPS rather than fails, because the empty case is legitimate and is
	// exactly what CI sees: .mxcli/widgets/*.def.json is derived and gitignored,
	// so a fresh checkout has only the hand-crafted definitions in
	// sdk/widgets/definitions/, none of which declares an authorable object list
	// with item properties. The guarantee comes from
	// TestItemExampleLiteral_* below, which is hermetic and runs everywhere;
	// this test is the end-to-end confirmation where the environment can give it.
	if withItems == 0 {
		t.Skip("no authorable object list with item properties in this environment " +
			"(the .def.json cache is gitignored, so CI has only the hand-crafted " +
			"definitions) — see TestItemExampleLiteral_* for the hermetic assertion")
	}
	if len(offenders) > 0 {
		t.Errorf("%d generated examples carry a placeholder value the validator rejects "+
			"(%d validated, %d authorable object lists):\n  %s",
			len(offenders), validated, withItems, strings.Join(offenders, "\n  "))
	}
}

func firstLineContaining(s, needle string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, needle) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// The hermetic half: itemExampleLiteral's contract, asserted on a synthetic
// description so it runs in CI as well as locally.
//
// The end-to-end test above cannot carry this on its own — it needs a widget
// definition with an authorable object list, and CI has none, because the
// .def.json cache those come from is derived and gitignored. A test that only
// runs on a developer's machine is not a guard.
func TestItemExampleLiteral_PrefersASubPropertyWithADerivableValue(t *testing.T) {
	desc := WidgetDescription{
		Properties: []DescribedProperty{{
			Key: "series", Type: "object",
			Children: []DescribedProperty{
				{Key: "staticName", Type: "texttemplate"},
				{Key: "dataSet", Type: "enumeration", Enum: []string{"static", "dynamic"}},
			},
		}},
	}
	c := DescribedContainer{
		Keyword: "series", PropertyKey: "series", Kind: "object list",
		ItemKeys: []string{"staticName", "dataSet"},
	}

	key, lit := itemExampleLiteral(desc, c)
	if key != "dataSet" {
		t.Errorf("key = %q, want dataSet — a sub-property with a derivable value must be "+
			"preferred over a free-text one, or the example shows `'…'` where a real "+
			"member was available", key)
	}
	if lit != "'static'" {
		t.Errorf("literal = %q, want 'static' (the enumeration's first member); `'…'` is "+
			"what MDL-WIDGET08 rejects", lit)
	}
}

// The definition wins over the .mpk, because the definition is what
// MDL-WIDGET08 checks the value against. Measured cause: ParseMPKForWidget
// returns 0 children for a PopupMenu's `basicItems`, while its definition
// carries itemType with enumValues [item, divider].
func TestItemExampleLiteral_DefinitionBeatsThePackage(t *testing.T) {
	desc := WidgetDescription{
		Properties: []DescribedProperty{{
			Key: "basicItems", Type: "object",
			// The package knows the key but nothing about its values.
			Children: []DescribedProperty{{Key: "itemType", Type: "enumeration"}},
		}},
	}
	c := DescribedContainer{
		Keyword: "item", PropertyKey: "basicItems", Kind: "object list",
		ItemKeys: []string{"itemType"},
		items: []DescribedProperty{
			{Key: "itemType", Type: "primitive", Default: "item", Enum: []string{"item", "divider"}},
		},
	}

	key, lit := itemExampleLiteral(desc, c)
	if key != "itemType" || lit != "'item'" {
		t.Errorf("got (%q, %q), want (itemType, 'item') — the definition carries the value "+
			"the package omits, and it is the source the validator reads", key, lit)
	}
}

// A container whose sub-properties are ALL free text keeps the placeholder.
// There is no correct value to invent, the validator accepts any string, and
// inventing one would be worse than admitting the gap.
func TestItemExampleLiteral_FreeTextKeepsThePlaceholder(t *testing.T) {
	desc := WidgetDescription{
		Properties: []DescribedProperty{{
			Key: "attributes", Type: "object",
			Children: []DescribedProperty{{Key: "attributeName", Type: "string"}},
		}},
	}
	c := DescribedContainer{
		Keyword: "attribute", PropertyKey: "attributes", Kind: "object list",
		ItemKeys: []string{"attributeName"},
	}
	key, lit := itemExampleLiteral(desc, c)
	if key != "attributeName" || lit != "'…'" {
		t.Errorf("got (%q, %q), want (attributeName, '…')", key, lit)
	}
}

// No sub-properties at all: nothing to write, and no panic on the empty slice.
func TestItemExampleLiteral_NoItemKeys(t *testing.T) {
	if key, lit := itemExampleLiteral(WidgetDescription{}, DescribedContainer{}); key != "" || lit != "" {
		t.Errorf("got (%q, %q), want empty", key, lit)
	}
}
