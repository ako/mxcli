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
	// object list at all.
	if withItems == 0 {
		t.Fatal("no widget has an authorable object list with item properties — " +
			"nothing here would exercise the item literal")
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
