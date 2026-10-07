// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// grammarRenameTargets reads the renameTarget rule out of the committed
// grammar, plus MODULE (renameStatement's second alternative), as the words a
// user types: "JAVA ACTION", not two tokens.
func grammarRenameTargets(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "mdl", "grammar", "MDLParser.g4")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rule := regexp.MustCompile(`(?s)\nrenameTarget\s*\n\s*:(.*?)\n\s*;`).FindStringSubmatch(string(b))
	if rule == nil {
		t.Fatal("renameTarget rule not found — re-point this guard rather than deleting it")
	}
	var out []string
	for _, alt := range strings.Split(rule[1], "|") {
		if alt = strings.Join(strings.Fields(alt), " "); alt != "" {
			out = append(out, alt)
		}
	}
	return append(out, "MODULE")
}

// `mxcli rename` offered eight of the ten targets RENAME accepts: JAVA ACTION
// and WORKFLOW were MDL-only, because the subcommand kept its own switch of
// types and nothing tied it to the grammar (follow-up to mendixlabs/mxcli#1318).
// Every grammar target must be reachable from the shell, spelled as the
// keyword in lower case with '-' for the space, and must build a statement
// that parses to the RENAME it names.
func TestRenameSubcommandCoversEveryGrammarTarget(t *testing.T) {
	for _, target := range grammarRenameTargets(t) {
		arg := strings.ToLower(strings.ReplaceAll(target, " ", "-"))
		t.Run(arg, func(t *testing.T) {
			name := "Sales.Old"
			if target == "MODULE" {
				name = "Sales"
			}
			stmt, keyword, err := renameStatement(arg, name, "New", false)
			if err != nil {
				t.Fatalf("mxcli rename %s: %v", arg, err)
			}
			if keyword != target {
				t.Errorf("type %q maps to %q, want %q", arg, keyword, target)
			}
			prog, errs := visitor.Build(stmt)
			if len(errs) > 0 {
				t.Fatalf("%q does not parse: %v", stmt, errs[0])
			}
			rs, ok := prog.Statements[0].(*ast.RenameStmt)
			if !ok {
				t.Fatalf("%q parsed as %T, not a RENAME", stmt, prog.Statements[0])
			}
			if want := strings.ToLower(strings.ReplaceAll(target, " ", "")); rs.ObjectType != want {
				t.Errorf("%q renames a %q, want %q", stmt, rs.ObjectType, want)
			}
		})
	}
}

func TestRenameStatementSpellings(t *testing.T) {
	for _, tc := range []struct {
		arg  string
		want string
	}{
		{"java-action", "RENAME JAVA ACTION Sales.Old TO New DRY RUN"},
		{"javaaction", "RENAME JAVA ACTION Sales.Old TO New DRY RUN"},
		{"java_action", "RENAME JAVA ACTION Sales.Old TO New DRY RUN"},
		{"Workflow", "RENAME WORKFLOW Sales.Old TO New DRY RUN"},
		{"entity", "RENAME ENTITY Sales.Old TO New DRY RUN"},
	} {
		got, _, err := renameStatement(tc.arg, "Sales.Old", "New", true)
		if err != nil || got != tc.want {
			t.Errorf("renameStatement(%q) = %q, %v; want %q", tc.arg, got, err, tc.want)
		}
	}
	if _, _, err := renameStatement("folder", "Sales.Old", "New", false); err == nil {
		t.Error("an unknown type must be refused")
	}
}
