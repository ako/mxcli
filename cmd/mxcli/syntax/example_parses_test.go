// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/syntax"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestExamplesParse is the anti-drift guard for `mxcli syntax`.
//
// The registry is hand-maintained while the grammar moves underneath it, and
// nothing tied the two together: the other tests here check structure (fields
// populated, aliases resolve, see-also targets exist) rather than whether the
// documented MDL is real. So entries went stale silently — and the registry is
// the first place an agent looks, so a gap here becomes a wrong workaround in a
// generated app rather than a puzzled user.
//
// Only Example is checked. Syntax fields carry metasyntax (`GET|POST`,
// `[OR MODIFY]`, `<statements>`) and are not meant to parse.
//
// Examples come in several legitimate shapes, so each block is tried in turn as
// a whole statement, a microflow activity, a page widget, a workflow activity,
// and a retrieve clause. A failure means the block parses as none of them — not
// that it failed the first attempt.
func TestExamplesParse(t *testing.T) {
	for _, f := range syntax.All() {
		f := f
		t.Run(f.Path, func(t *testing.T) {
			// Examples routinely show several independent snippets separated by
			// blank lines — a statement, then a variant, then a related clause.
			// Checking each block on its own keeps the docs readable while still
			// holding every snippet to "this really parses".
			blocks := mdlBlocks(f.Example)
			// TestFeatureFieldsPopulated already rejects an empty Example, but an
			// example made entirely of comments would yield no blocks and quietly
			// opt out of this guard. Keep that a failure everywhere except the
			// handful of prose topics below, so the opt-out cannot spread by
			// accident.
			if len(blocks) == 0 {
				if nonStatementTopics[f.Path] {
					t.Skipf("example is deliberately not MDL statements — see nonStatementTopics")
				}
				t.Fatalf("Example contains no MDL to check:\n%s", f.Example)
			}
			for _, block := range blocks {
				if ctx, ok := parsesInSomeContext(block); ok {
					t.Logf("block parses as %s", ctx)
					continue
				}
				_, errs := visitor.Build(block)
				var first any = "unknown"
				if len(errs) > 0 {
					first = errs[0]
				}
				t.Errorf("example block parses as no known construct "+
					"(statement / microflow activity / page widget / workflow activity).\n"+
					"top-level error: %v\n--- block ---\n%s", first, block)
			}
		})
	}
}

// nonStatementTopics are the entries whose Example is legitimately not made of
// MDL statements — the troubleshooting topics pair an error message with its fix
// as prose, and `oql` documents a CLI invocation whose quoted argument is OQL,
// not MDL.
//
// Keep this list short and justified: an entry added here stops being checked at
// all, which is the one way a stale example could still slip through.
var nonStatementTopics = map[string]bool{
	"errors":           true, // error/fix prose
	"errors.execution": true, // error/fix prose
	"errors.reference": true, // error/fix prose
	"errors.syntax":    true, // error/fix prose
	"oql":              true, // `mxcli oql "<OQL>"` — a shell command, and OQL is not MDL
}

// mdlBlocks splits an Example into blank-line-separated blocks, dropping shell
// commands, comment-only blocks, and leading prose.
func mdlBlocks(example string) []string {
	var blocks []string
	for _, raw := range strings.Split(stripNonMDL(example), "\n\n") {
		block := strings.TrimSpace(stripNonMDL(raw))
		if block == "" {
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// parsesInSomeContext reports the first context the example parses in.
func parsesInSomeContext(example string) (string, bool) {
	for _, c := range []struct {
		name string
		wrap func(string) string
	}{
		{"statement", func(s string) string { return s }},
		{"microflow activity", func(s string) string {
			return "CREATE MICROFLOW SyntaxDoc.Probe ()\nBEGIN\n" + s + "\nEND;"
		}},
		{"page widget", func(s string) string {
			return "CREATE PAGE SyntaxDoc.Probe (Title: 'Probe', Layout: 'Atlas_Core.Atlas_Default') {\n" + s + "\n};"
		}},
		{"workflow activity", func(s string) string {
			return "CREATE WORKFLOW SyntaxDoc.Probe PARAMETER $Ctx: SyntaxDoc.Ctx\nBEGIN\n" + s + "\nEND WORKFLOW;"
		}},
		// XPath topics illustrate a constraint on its own (`WHERE [...]`), which
		// is a clause rather than a statement. Hanging it off a RETRIEVE checks
		// the constraint itself without forcing the docs to repeat a full
		// statement around every example.
		{"retrieve clause", func(s string) string {
			return "CREATE MICROFLOW SyntaxDoc.Probe ()\nBEGIN\n" +
				"RETRIEVE $Probe FROM SyntaxDoc.Entity\n" + s + ";\nEND;"
		}},
	} {
		if _, errs := visitor.Build(c.wrap(example)); len(errs) == 0 {
			return c.name, true
		}
	}
	// A block may list several alternative clauses one per line (the xpath
	// function reference does this). Accept it when every line is a valid clause
	// on its own — the block is a menu, not one statement.
	if lines := nonEmptyLines(example); len(lines) > 1 {
		all := true
		for _, line := range lines {
			if _, ok := parsesInSomeContext(line); !ok {
				all = false
				break
			}
		}
		if all {
			return "per-line clauses", true
		}
	}
	return "", false
}

// stripTestAnnotations removes `/** … @test … */` blocks.
//
// DOC_COMMENT is a real token in MDL, not a hidden one, and the grammar admits
// it only ahead of a handful of declarations. The `@test`/`@expect` annotations
// in a .test.mdl file sit ahead of ordinary microflow statements instead, and
// are consumed by the test runner's own front end before the MDL is parsed.
// Removing them here keeps the statements underneath under test rather than
// exempting the whole entry.
func stripTestAnnotations(s string) string {
	for {
		start := strings.Index(s, "/**")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "*/")
		if end < 0 {
			break
		}
		end += start + len("*/")
		if !strings.Contains(s[start:end], "@test") && !strings.Contains(s[start:end], "@expect") {
			break
		}
		s = s[:start] + s[end:]
	}
	return s
}

// nonEmptyLines returns the block's lines with blanks and comments dropped.
func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		out = append(out, t)
	}
	return out
}

// stripNonMDL removes leading prose and any shell-command lines. Some entries
// deliberately show the `mxcli …` equivalent alongside the MDL; those lines are
// documentation, not statements to validate.
func stripNonMDL(s string) string {
	s = stripTestAnnotations(s)
	var kept []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "mxcli ") {
			continue
		}
		// A lone `/` is MDL's statement separator (used between blocks in
		// .test.mdl files). It carries no syntax to validate and would otherwise
		// land inside a wrapper's body.
		if t == "/" {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.Join(kept, "\n")

	// Drop a leading run of comment/blank lines so a prose preamble does not
	// decide the parse context.
	lines := strings.Split(out, "\n")
	i := 0
	for i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if t == "" || strings.HasPrefix(t, "--") {
			i++
			continue
		}
		break
	}
	return strings.Join(lines[i:], "\n")
}
