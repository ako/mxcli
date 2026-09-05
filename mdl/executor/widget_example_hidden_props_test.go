// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

var examplePropLine = regexp.MustCompile(`(?m)^\s{2}([A-Za-z][A-Za-z0-9_]*)\s*:`)

// exampleScalarKeys returns the property keys the generated example writes in the
// widget's own head — the `  key: value,` lines, not container item properties.
func exampleScalarKeys(example string) []string {
	head := example
	if i := strings.Index(head, ") {"); i >= 0 {
		head = head[:i]
	}
	var out []string
	for _, m := range examplePropLine.FindAllStringSubmatch(head, -1) {
		out = append(out, m[1])
	}
	return out
}

// The generator narrows its example by the widget's editorConfig hide-rules, and
// the validator implements the same rules as MDL-WIDGET10. They disagreed: 32
// warnings over the fixture's widgets, every one of them naming a property the
// example ITSELF had just emitted.
//
//	videoplayer example:  heightUnit: 'aspectRatio',  …  height: 500
//	validator:            property `height` is hidden when `heightUnit` is
//	                      "aspectRatio" — the value will be ignored
//
// The generator chose `heightUnit: 'aspectRatio'` and then wrote the `height`
// its own rule hides. Cause: hiddenUnder was consulted only on the branch that
// asks for a BINDING (attribute/datasource/action/expression/selection) and not
// on the scalar branch that emits literals — so the narrowing existed and half
// the properties skipped it.
//
// Reported by an external test project (14 of 14 warnings were self-inflicted
// there); reproduced here at 668ad9ae over all 42 definitions.
func TestUsageExampleDoesNotEmitItsOwnHiddenProperties(t *testing.T) {
	project := fixtureProject(t)
	registry := LoadWidgetRegistry(project)
	if registry == nil {
		t.Fatal("no registry")
	}

	var offenders []string
	var checked int
	for _, def := range registry.All() {
		if def == nil || def.MDLName == "" {
			continue
		}
		desc, err := DescribeWidget(def.MDLName, project)
		if err != nil || desc == nil || desc.Example == "" {
			continue
		}
		checked++
		for _, key := range exampleScalarKeys(desc.Example) {
			if hiddenUnder(*desc, key) {
				offenders = append(offenders, def.MDLName+"."+key)
			}
		}
	}
	if checked == 0 {
		t.Skip("no widgets described")
	}
	if len(offenders) > 0 {
		t.Errorf("%d properties are emitted by the example and hidden by that same "+
			"example's configuration — the generator and MDL-WIDGET10 must not disagree:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// The control. "Emit nothing hidden" is trivially satisfied by emitting nothing,
// and it is also satisfied if hiddenUnder always returns false — in which case
// the test above proves nothing at all. Assert that pruning REALLY fires: some
// widget must have a scalar property that hiddenUnder reports hidden under the
// example's own configuration, i.e. something was actually removed.
func TestUsageExamplePruningActuallyFires(t *testing.T) {
	project := fixtureProject(t)
	registry := LoadWidgetRegistry(project)
	if registry == nil {
		t.Fatal("no registry")
	}

	var pruned int
	var nonEmpty int
	for _, def := range registry.All() {
		if def == nil || def.MDLName == "" {
			continue
		}
		desc, err := DescribeWidget(def.MDLName, project)
		if err != nil || desc == nil || desc.Example == "" {
			continue
		}
		if len(exampleScalarKeys(desc.Example)) > 0 {
			nonEmpty++
		}
		for _, p := range desc.Properties {
			if p.System || !p.Required {
				continue
			}
			switch strings.ToLower(p.Type) {
			case "boolean", "integer", "enumeration", "string", "texttemplate":
				if hiddenUnder(*desc, p.Key) {
					pruned++
				}
			}
		}
	}
	if nonEmpty == 0 {
		t.Fatal("no example emits any property — the first test would pass vacuously")
	}
	if pruned == 0 {
		t.Fatal("hiddenUnder never fires on a required scalar, so the assertion in " +
			"TestUsageExampleDoesNotEmitItsOwnHiddenProperties is vacuous")
	}
	t.Logf("%d required scalars pruned across %d widgets with a non-empty example", pruned, nonEmpty)
}

// The end-to-end form of the same invariant, and the one that matches how the
// disagreement was reported: build each widget's example, parse it into a real
// page statement, and run the property validator over it. Zero MDL-WIDGET10.
//
// This is stronger than the structural test above, which can only see what the
// generator itself considers hidden. The residue it catches is the OTHER
// direction of the same split: the validator resolves a property's value from
// the widget DEFINITION's mapping defaults (gallery's `itemSelection` defaults
// to "Single", so `keepSelection` is hidden), while exampleValues read only the
// .mpk, where a selection property carries no defaultValue — so the generator
// called it indeterminable and emitted the property the validator then warned
// about. Two value sources for one question.
func TestGeneratedExamplesProduceNoHiddenPropertyWarnings(t *testing.T) {
	project := fixtureProject(t)
	registry := LoadWidgetRegistry(project)
	if registry == nil {
		t.Fatal("no registry")
	}

	var offenders []string
	var validated int
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
			continue // the example-parses guarantee is a different test's business
		}
		for _, stmt := range prog.Statements {
			for _, v := range ValidateWidgetPropertiesForStatement(stmt, registry) {
				if v.RuleID == "MDL-WIDGET10" {
					offenders = append(offenders, def.MDLName+": "+v.Message)
				}
			}
		}
		validated++
	}

	if validated == 0 {
		t.Skip("no example validated")
	}
	if len(offenders) > 0 {
		t.Errorf("%d hidden-property warnings on mxcli's OWN generated examples "+
			"(%d widgets validated) — the generator and MDL-WIDGET10 read the same "+
			"editorConfig rules and must reach the same answer:\n  %s",
			len(offenders), validated, strings.Join(offenders, "\n  "))
	}
}
