// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestLegacyRunnerRenamesLowerCaseAssignments: the after-startup runner compiles
// every test into one microflow, so a variable two tests both assign has to be
// renamed per test. Which variables are renamed was decided by a pattern that
// matched only upper-case `CALL` and `CREATE` — the spelling the runner's own
// examples used — so `$r = call microflow …`, how mdl 1 and `fmt` spell it, was
// not renamed. Two tests that both assign `$r` then declare it twice, and
// injection fails with MDL063 (CE0111 "Duplicate variable name" from mxbuild).
// Found alongside ako/mxcli#1753.
func TestLegacyRunnerRenamesLowerCaseAssignments(t *testing.T) {
	cases := []struct {
		name, header, body, renamed string
	}{
		{"call microflow, mdl 1", "mdl 1;\n",
			"$r = call microflow Probe.SUB_Make();", "$r_%d = call microflow"},
		{"call microflow, mdl 0", "",
			"$r = call microflow Probe.SUB_Make();", "$r_%d = call microflow"},
		{"call java action", "mdl 1;\n",
			"$r = call java action Probe.JA_Upper(Text = 'a');", "$r_%d = call java action"},
		{"create object", "mdl 1;\n",
			"$r = create Probe.Item (Name = 'a');", "$r_%d = create Probe.Item"},
		{"create list", "mdl 1;\n",
			"$r = create list of Probe.Item;", "$r_%d = create list of"},
		{"mixed case", "mdl 1;\n",
			"$r = Call Microflow Probe.SUB_Make();", "$r_%d = Call Microflow"},
		// Control: the spelling the pattern always matched.
		{"upper case (control)", "",
			"$r = CALL MICROFLOW Probe.SUB_Make();", "$r_%d = CALL MICROFLOW"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			block := func(n string) string {
				return "/**\n * @test " + n + "\n * @expect $r != empty\n */\n" + c.body + "\n/\n"
			}
			tests, err := parseMDLTests(c.header+block("one")+"\n"+block("two"), "rename.test.mdl")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			src := GenerateTestRunner(&TestSuite{Name: "s", Tests: tests})

			for i, want := range []string{
				strings.Replace(c.renamed, "%d", "1", 1),
				strings.Replace(c.renamed, "%d", "2", 1),
			} {
				if !strings.Contains(src, want) {
					t.Errorf("test %d: no %q — the assignment was not renamed", i+1, want)
				}
			}
			if strings.Contains(src, "($r != empty)") {
				t.Errorf("an assertion still reads the unrenamed $r")
			}

			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("generated runner does not parse: %v\n%s", errs[0], src)
			}
			for _, v := range executor.ValidateProgram(prog, "") {
				if v.RuleID == "MDL063" {
					t.Errorf("%s: %s", v.RuleID, v.Message)
				}
			}
			if t.Failed() {
				t.Logf("generated runner:\n%s", src)
			}
		})
	}
}

// TestAssignPatternNeedsAWholeKeyword: matching case-insensitively must not turn
// an assignment whose value merely starts with the letters of a keyword into one.
func TestAssignPatternNeedsAWholeKeyword(t *testing.T) {
	for _, body := range []string{
		"$r = callback;",
		"$r = createdDate;",
	} {
		if names := extractVariableNames(body); len(names) != 0 {
			t.Errorf("%q: renamed %v, want nothing", body, names)
		}
	}
	if names := extractVariableNames("$r = call microflow M.F();"); !names["r"] {
		t.Errorf("control: $r = call microflow was not picked up: %v", names)
	}
}
