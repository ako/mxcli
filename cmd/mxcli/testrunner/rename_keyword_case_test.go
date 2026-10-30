// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// collisionRules are the checks that report one variable name defined twice in
// a flow: MDL063 for any definition, MDL052 for two loop iterators. Both are
// CE0111 "Duplicate variable name" from mxbuild, and both are what the legacy
// runner's per-test renaming exists to prevent.
var collisionRules = map[string]bool{"MDL063": true, "MDL052": true}

// legacyRunnerCollisions generates the after-startup runner for tests and
// returns every duplicate-variable violation ValidateProgram reports on it.
func legacyRunnerCollisions(t *testing.T, tests []TestCase) (string, []string) {
	t.Helper()
	src := GenerateTestRunner(&TestSuite{Name: "s", Tests: tests})
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("generated runner does not parse: %v\n%s", errs[0], src)
	}
	var out []string
	for _, v := range executor.ValidateProgram(prog, "") {
		if collisionRules[v.RuleID] {
			out = append(out, v.RuleID+": "+v.Message)
		}
	}
	return src, out
}

// TestLegacyRunnerRenamesLowerCaseAssignments: the after-startup runner compiles
// every test into one microflow, so a variable two tests both assign has to be
// renamed per test. Which variables are renamed was decided by a pattern that
// matched only upper-case `CALL` and `CREATE` — the spelling the runner's own
// examples used — so `$r = call microflow …`, how mdl 1 and `fmt` spell it, was
// not renamed. Two tests that both assign `$r` then declare it twice, and
// injection fails with MDL063 (CE0111 "Duplicate variable name" from mxbuild).
// Found alongside ako/mxcli#1753.
//
// The pattern never matched any other statement that defines a variable either
// — retrieve, declare, a list operation or aggregate, send rest request, execute
// database query, a loop iterator — in any case, so two tests defining the same
// name one of those ways still collided (ako/mxcli#1775). The names now come from
// the parsed body, so every definition MDL063 counts is renamed, nested ones
// included: a Mendix flow's variable namespace is flat, and the runner joins
// every test into one flow.
func TestLegacyRunnerRenamesLowerCaseAssignments(t *testing.T) {
	const items = "retrieve $items from Probe.Item;\n"
	cases := []struct {
		name, header, body, expect string
		renamed                    []string
	}{
		{name: "call microflow, mdl 1", header: "mdl 1;\n",
			body: "$r = call microflow Probe.SUB_Make();", renamed: []string{"$r_%d = call microflow"}},
		{name: "call microflow, mdl 0",
			body: "$r = call microflow Probe.SUB_Make();", renamed: []string{"$r_%d = call microflow"}},
		{name: "call java action", header: "mdl 1;\n",
			body: "$r = call java action Probe.JA_Upper(Text = 'a');", renamed: []string{"$r_%d = call java action"}},
		{name: "create object", header: "mdl 1;\n",
			body: "$r = create Probe.Item (Name = 'a');", renamed: []string{"$r_%d = create Probe.Item"}},
		{name: "create list", header: "mdl 1;\n",
			body: "$r = create list of Probe.Item;", renamed: []string{"$r_%d = create list of"}},
		{name: "mixed case", header: "mdl 1;\n",
			body: "$r = Call Microflow Probe.SUB_Make();", renamed: []string{"$r_%d = Call Microflow"}},

		// ako/mxcli#1775: definitions the pattern never matched, in any case.
		{name: "retrieve, mdl 1", header: "mdl 1;\n",
			body: "retrieve $r from Probe.Item;", renamed: []string{"retrieve $r_%d from"}},
		{name: "retrieve, mdl 0",
			body: "retrieve $r from Probe.Item;", renamed: []string{"retrieve $r_%d from"}},
		{name: "retrieve first", header: "mdl 1;\n",
			body: "retrieve $r from Probe.Item first;", renamed: []string{"retrieve $r_%d from"}},
		{name: "RETRIEVE upper case",
			body: "RETRIEVE $r FROM Probe.Item;", renamed: []string{"RETRIEVE $r_%d FROM"}},
		{name: "declare", header: "mdl 1;\n",
			body: "declare $r String = 'a';", renamed: []string{"declare $r_%d String"}},
		{name: "declare, mdl 0",
			body: "DECLARE $r String = 'a';", renamed: []string{"DECLARE $r_%d String"}},
		{name: "list operation head", header: "mdl 1;\n",
			body:    items + "$r = head($items);",
			renamed: []string{"retrieve $items_%d from", "$r_%d = head($items_%d)"}},
		{name: "list operation, statement form", header: "mdl 1;\n",
			body:    items + "$r = head $items;",
			renamed: []string{"$r_%d = head $items_%d;"}},
		{name: "list operation filter", header: "mdl 1;\n", expect: "count($r) >= 0",
			body:    items + "$r = filter($items, Name = 'a');",
			renamed: []string{"$r_%d = filter($items_%d"}},
		{name: "aggregate", header: "mdl 1;\n", expect: "$r >= 0",
			body:    items + "$r = count($items);",
			renamed: []string{"$r_%d = count($items_%d)"}},
		{name: "send rest request", header: "mdl 1;\n",
			body: "$r = send rest request Probe.Api.GetThing;", renamed: []string{"$r_%d = send rest request"}},
		{name: "execute database query", header: "mdl 1;\n", expect: "count($r) >= 0",
			body: "$r = execute database query Probe.Db.ListRows;", renamed: []string{"$r_%d = execute database query"}},

		// Nested definitions share the flow's one namespace: an IF branch, a
		// loop body and an error handler do not open a scope (MDL063), and a
		// loop iterator is flow-wide too (MDL052).
		{name: "declare in an if branch", header: "mdl 1;\n",
			body:    "declare $r String = 'a';\nif $r = 'a' then\n  declare $inner String = 'b';\n  set $r = $inner;\nend if;",
			renamed: []string{"declare $inner_%d String", "set $r_%d = $inner_%d"}},
		{name: "loop iterator and a definition in its body", header: "mdl 1;\n", expect: "count($items) >= 0",
			body:    items + "loop $it in $items begin\n  retrieve $r from Probe.Item first;\nend loop;",
			renamed: []string{"loop $it_%d in $items_%d", "retrieve $r_%d from"}},
		{name: "declare in a custom error handler", header: "mdl 1;\n", expect: "1 = 1",
			body:    "call microflow Probe.SUB_Make() on error begin\n  declare $msg String = 'failed';\n  log error node 'T' $msg;\nend error;",
			renamed: []string{"declare $msg_%d String", "node 'T' $msg_%d"}},

		// Control: the spelling the pattern always matched.
		{name: "upper case (control)",
			body: "$r = CALL MICROFLOW Probe.SUB_Make();", renamed: []string{"$r_%d = CALL MICROFLOW"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			expect := c.expect
			if expect == "" {
				expect = "$r != empty"
			}
			block := func(n string) string {
				return "/**\n * @test " + n + "\n * @expect " + expect + "\n */\n" + c.body + "\n/\n"
			}
			tests, err := parseMDLTests(c.header+block("one")+"\n"+block("two"), "rename.test.mdl")
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			src, collisions := legacyRunnerCollisions(t, tests)

			for _, pattern := range c.renamed {
				for i, n := range []string{"1", "2"} {
					want := strings.ReplaceAll(pattern, "%d", n)
					if !strings.Contains(src, want) {
						t.Errorf("test %d: no %q — the definition was not renamed", i+1, want)
					}
				}
			}
			if strings.Contains(expect, "$r") && strings.Contains(src, "($r ") {
				t.Errorf("an assertion still reads the unrenamed $r")
			}
			for _, v := range collisions {
				t.Error(v)
			}
			if t.Failed() {
				t.Logf("generated runner:\n%s", src)
			}
		})
	}
}

// TestLegacyRunnerRenamesTheThrowsPath: an @throws test goes through its own
// block writer, which renames the body too. Two of them defining one name must
// not collide either.
func TestLegacyRunnerRenamesTheThrowsPath(t *testing.T) {
	block := func(n string) string {
		return "/**\n * @test " + n + "\n * @throws\n */\nretrieve $r from Probe.Item first;\n" +
			"call microflow Probe.SUB_Fail(Item = $r);\n/\n"
	}
	tests, err := parseMDLTests("mdl 1;\n"+block("one")+"\n"+block("two"), "throws.test.mdl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	src, collisions := legacyRunnerCollisions(t, tests)
	for _, n := range []string{"1", "2"} {
		for _, want := range []string{"retrieve $r_" + n + " from", "(Item = $r_" + n + ")"} {
			if !strings.Contains(src, want) {
				t.Errorf("no %q", want)
			}
		}
	}
	for _, v := range collisions {
		t.Error(v)
	}
	if t.Failed() {
		t.Logf("generated runner:\n%s", src)
	}
}

// TestAssignPatternNeedsAWholeKeyword: an assignment whose value merely starts
// with the letters of a keyword defines nothing — it changes a variable — so it
// must not be renamed as if it were a call or a create. Nor does `set`, the
// spelled-out form of the same Change Variable.
func TestAssignPatternNeedsAWholeKeyword(t *testing.T) {
	for _, body := range []string{
		"$r = callback;",
		"$r = createdDate;",
		"set $r = 1;",
	} {
		if names := extractVariableNames(body, langver.V0); len(names) != 0 {
			t.Errorf("%q: renamed %v, want nothing", body, names)
		}
	}
	if names := extractVariableNames("$r = call microflow M.F();", langver.V0); !names["r"] {
		t.Errorf("control: $r = call microflow was not picked up: %v", names)
	}
}

// TestLegacyRunnerRepositoryTestFilesDefineEachVariableOnce runs every test
// file in the repository through the after-startup runner's generator and the
// validator `exec` applies at injection. A test file the endpoint runner runs
// must not fail on this one because two of its tests define a name the same
// way. The sweep is what found ako/mxcli#1775: before it, 2 of these files drew
// MDL063 (`retrieve $Brands` in three tests; `retrieve $Request` and
// `$Request = head(…)`).
//
// A `.fail.` file is a negative fixture whose point is another refusal, and is
// left out.
func TestLegacyRunnerRepositoryTestFilesDefineEachVariableOnce(t *testing.T) {
	var files []string
	for _, root := range []string{"../../../mdl-examples", "../../../docs-site"} {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && isTestFile(d.Name()) && !strings.Contains(d.Name(), ".fail.") {
				files = append(files, p)
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Fatal("no test files found")
	}
	for _, p := range files {
		t.Run(filepath.Base(p), func(t *testing.T) {
			if _, err := os.Stat(p); err != nil {
				t.Fatal(err)
			}
			suite, err := ParseTestFile(p)
			if err != nil {
				t.Fatalf("ParseTestFile: %v", err)
			}
			if len(suite.Tests) < 2 {
				t.Skip("fewer than two tests: nothing can collide")
			}
			src, collisions := legacyRunnerCollisions(t, suite.Tests)
			for _, v := range collisions {
				t.Error(v)
			}
			if t.Failed() {
				t.Logf("generated runner:\n%s", src)
			}
		})
	}
}
