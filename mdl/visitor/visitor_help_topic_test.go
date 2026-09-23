// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `mxcli syntax` prints its topics as dotted paths — `workflow.user-task` —
// and the REPL's HELP reads the same registry. Copying a printed path into
// HELP was a parse error: `helpStatement: IDENTIFIER (identifierOrKeyword)*`
// took neither the hyphen (a separate HYPHENATED_ID token) nor the dot.
// mendixlabs/mxcli#1025.
func TestHelpTopicSpellings(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{"words", `help workflow user task targeting;`,
			[]string{"workflow", "user", "task", "targeting"}},
		{"hyphenated", `help workflow user-task targeting;`,
			[]string{"workflow", "user-task", "targeting"}},
		{"dotted", `help workflow.user-task.targeting;`,
			[]string{"workflow", "user-task", "targeting"}},
		{"dotted, no hyphen", `help domain-model.entity;`,
			[]string{"domain-model", "entity"}},
		{"single topic", `help workflow;`, []string{"workflow"}},
		{"mixed", `help security entity-access;`,
			[]string{"security", "entity-access"}},
		// A keyword as a topic word still works — `page` and `security` are
		// keywords, and the topics are named after them.
		{"keyword topic", `help page widgets;`, []string{"page", "widgets"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(tt.input)
			if len(errs) > 0 {
				t.Fatalf("%s: %v", tt.input, errs)
			}
			stmt, ok := prog.Statements[0].(*ast.HelpStmt)
			if !ok {
				t.Fatalf("Expected HelpStmt, got %T", prog.Statements[0])
			}
			if strings.Join(stmt.Topic, "|") != strings.Join(tt.want, "|") {
				t.Errorf("%s: Topic = %q, want %q", tt.input, stmt.Topic, tt.want)
			}
		})
	}
}

// HELP with no topic, and EXIT/QUIT, share the rule; widening it must not
// change them.
func TestHelpAndExitUnchanged(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  ast.Statement
	}{
		{`help;`, &ast.HelpStmt{}},
		{`exit;`, &ast.ExitStmt{}},
		{`quit;`, &ast.ExitStmt{}},
	} {
		prog, errs := Build(tc.input)
		if len(errs) > 0 {
			t.Errorf("%s: %v", tc.input, errs)
			continue
		}
		if got, want := prog.Statements[0], tc.want; !sameType(got, want) {
			t.Errorf("%s: got %T, want %T", tc.input, got, want)
		}
	}
}

func sameType(a, b ast.Statement) bool {
	switch a.(type) {
	case *ast.HelpStmt:
		_, ok := b.(*ast.HelpStmt)
		return ok
	case *ast.ExitStmt:
		_, ok := b.(*ast.ExitStmt)
		return ok
	}
	return false
}

// helpStatement is the grammar's catch-all — an IDENTIFIER and some words —
// so whatever it can swallow, it swallows from the statement that should have
// had it. Widening it with a leading `DOT?` made `Sec.ApiUser` a complete
// statement of its own, and these two then parsed as a shorter statement
// followed by a "help topic": two statements, the wrong types, and no parse
// error to show for it. The topic word before the dot is what keeps them
// whole; these are the cases that caught it.
func TestHelpRuleDoesNotSwallowATrailingQualifiedName(t *testing.T) {
	tests := []struct {
		input string
		want  ast.Statement
	}{
		{`create module role Sec.ApiUser;`, &ast.CreateModuleRoleStmt{}},
		{`REVOKE EXECUTE ON MICROFLOW MyModule.ProcessOrder FROM MyModule.Admin;`,
			&ast.RevokeMicroflowAccessStmt{}},
	}
	for _, tt := range tests {
		prog, errs := Build(tt.input)
		if len(errs) > 0 {
			t.Errorf("%s: %v", tt.input, errs)
			continue
		}
		if len(prog.Statements) != 1 {
			t.Errorf("%s: parsed as %d statements, want 1", tt.input, len(prog.Statements))
			continue
		}
		if got := prog.Statements[0]; reflect.TypeOf(got) != reflect.TypeOf(tt.want) {
			t.Errorf("%s: got %T, want %T", tt.input, got, tt.want)
		}
	}
}
