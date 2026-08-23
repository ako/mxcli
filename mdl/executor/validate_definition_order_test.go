// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// buildOrderProgram parses a script and fails the test on a syntax error — a
// test whose script does not parse silently asserts nothing, because
// ValidateScriptDefinitionOrder returns no violations for an empty program.
func buildOrderProgram(t *testing.T, script string) []string {
	t.Helper()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("script did not parse: %v", errs)
	}
	if prog == nil || len(prog.Statements) == 0 {
		t.Fatalf("script parsed to no statements")
	}
	var msgs []string
	for _, v := range ValidateScriptDefinitionOrder(prog) {
		if v.RuleID != "MDL-ORDER01" {
			t.Errorf("unexpected rule id %q", v.RuleID)
		}
		msgs = append(msgs, v.Message)
	}
	return msgs
}

// #955: `check --references` passed scripts `exec` refuses over in-script
// creation order. Each shape below was measured against a real project: exec
// fails on it, with the "defined later in this script" hint, after earlier
// statements have already been written.
func TestValidateScriptDefinitionOrder_FlagsForwardReferences(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string // substring the violation must name
	}{
		{
			name: "microflow parameter type",
			script: `create module R;
create microflow R.MF ($T: R.Thing) returns String as $O
begin declare $O String = 'x'; return $O; end;
create persistent entity R.Thing (Name: String(100));`,
			want: "R.Thing",
		},
		{
			name: "microflow parameter enumeration",
			script: `create module R;
create microflow R.MF ($S: Enumeration(R.Stat)) returns String as $O
begin declare $O String = 'x'; return $O; end;
create enumeration R.Stat (Open 'Open');`,
			want: "R.Stat",
		},
		{
			name: "microflow return type",
			script: `create module R;
create microflow R.MF () returns R.Thing as $O
begin $O = create R.Thing (Name = 'x'); return $O; end;
create persistent entity R.Thing (Name: String(100));`,
			want: "R.Thing",
		},
		{
			name: "entity attribute enumeration",
			script: `create module R;
create persistent entity R.Thing (Status: Enumeration(R.Stat));
create enumeration R.Stat (Open 'Open');`,
			want: "R.Stat",
		},
		{
			name: "association endpoint",
			script: `create module R;
create persistent entity R.A (Name: String(100));
create association R.A_B from R.A to R.B;
create persistent entity R.B (Name: String(100));`,
			want: "R.B",
		},
		{
			name: "call microflow",
			script: `create module R;
create microflow R.Caller () returns Boolean as $O
begin $X = call microflow R.Callee (); declare $O Boolean = true; return $O; end;
create microflow R.Callee () returns Boolean as $O
begin declare $O Boolean = true; return $O; end;`,
			want: "R.Callee",
		},
		{
			name: "call nanoflow",
			script: `create module R;
create nanoflow R.Caller () returns Boolean as $O
begin $X = call nanoflow R.Callee (); declare $O Boolean = true; return $O; end;
create nanoflow R.Callee () returns Boolean as $O
begin declare $O Boolean = true; return $O; end;`,
			want: "R.Callee",
		},
		{
			name: "rule parameter type",
			script: `create module R;
create rule R.Chk ($T: R.Thing) returns Boolean as $O
begin declare $O Boolean = true; return $O; end;
create persistent entity R.Thing (Name: String(100));`,
			want: "R.Thing",
		},
		{
			name: "grant on entity",
			script: `create module R;
create module role R.User;
grant R.User on R.Thing (read *);
create persistent entity R.Thing (Name: String(100));`,
			want: "R.Thing",
		},
		{
			name: "grant execute on microflow",
			script: `create module R;
create module role R.User;
grant execute on microflow R.MF to R.User;
create microflow R.MF () returns Boolean as $O
begin declare $O Boolean = true; return $O; end;`,
			want: "R.MF",
		},
		{
			name: "call inside a loop body is reached",
			script: `create module R;
create microflow R.Caller () returns Boolean as $O
begin
  retrieve $items from R.Seen;
  loop $i in $items begin
    $X = call microflow R.Callee ();
  end loop;
  declare $O Boolean = true;
  return $O;
end;
create persistent entity R.Seen (Name: String(100));
create microflow R.Callee () returns Boolean as $O
begin declare $O Boolean = true; return $O; end;`,
			want: "R.Callee",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgs := buildOrderProgram(t, tt.script)
			if len(msgs) == 0 {
				t.Fatalf("expected MDL-ORDER01, got none")
			}
			joined := strings.Join(msgs, "\n")
			if !strings.Contains(joined, tt.want) {
				t.Errorf("violation does not name %q:\n%s", tt.want, joined)
			}
		})
	}
}

// The rule's other half. Every case here executes correctly with the definition
// coming afterwards, or cannot be judged without a project — flagging any of
// them would reject scripts that work today. Measured against Mendix 11.6.6;
// the executable ones are also in
// mdl-examples/bug-tests/955-forward-definition-order-tolerated.mdl.
func TestValidateScriptDefinitionOrder_LeavesToleratedReferencesAlone(t *testing.T) {
	tests := []struct {
		name   string
		script string
	}{
		{
			name: "entity EXTENDS generalization is resolved lazily",
			script: `create module R;
create persistent entity R.Child extends R.Parent (Name: String(100));
create persistent entity R.Parent (Code: String(50));`,
		},
		{
			name: "retrieve source",
			script: `create module R;
create microflow R.MF () returns Boolean as $O
begin retrieve $l from R.Thing; declare $O Boolean = true; return $O; end;
create persistent entity R.Thing (Name: String(100));`,
		},
		{
			name: "create object in a body",
			script: `create module R;
create microflow R.MF () returns Boolean as $O
begin $x = create R.Thing (Name = 'a'); declare $O Boolean = true; return $O; end;
create persistent entity R.Thing (Name: String(100));`,
		},
		{
			name: "call java action",
			script: `create module R;
create microflow R.MF () returns Boolean as $O
begin $X = call java action R.JA (); declare $O Boolean = true; return $O; end;
create java action R.JA () returns Boolean as $$
public Boolean executeAction() { return true; }
$$;`,
		},
		{
			// A later CREATE OR MODIFY asserts nothing about the document being
			// absent, so the reference may resolve against the project. Unsound
			// to flag with no project to look at; left to --references.
			name: "later definition is CREATE OR MODIFY",
			script: `create module R;
create microflow R.MF ($T: R.Thing) returns String as $O
begin declare $O String = 'x'; return $O; end;
create or modify persistent entity R.Thing (Name: String(100));`,
		},
		{
			name: "correct order is not flagged",
			script: `create module R;
create persistent entity R.Thing (Name: String(100));
create microflow R.MF ($T: R.Thing) returns String as $O
begin declare $O String = 'x'; return $O; end;`,
		},
		{
			// The executor holds the document it is writing, so a flow calling
			// itself is not an ordering problem.
			name: "self reference (recursion)",
			script: `create module R;
create microflow R.MF () returns Boolean as $O
begin $X = call microflow R.MF (); declare $O Boolean = true; return $O; end;`,
		},
		{
			name: "reference to a document not created in this script at all",
			script: `create module R;
create microflow R.MF ($T: Other.Thing) returns String as $O
begin declare $O String = 'x'; return $O; end;`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if msgs := buildOrderProgram(t, tt.script); len(msgs) > 0 {
				t.Errorf("expected no violation, got:\n%s", strings.Join(msgs, "\n"))
			}
		})
	}
}

// The rule reports every forward reference in the script, not just the first —
// a script fixed one statement at a time otherwise needs one run per mistake.
func TestValidateScriptDefinitionOrder_ReportsEveryOccurrence(t *testing.T) {
	msgs := buildOrderProgram(t, `create module R;
create microflow R.A ($T: R.Thing) returns String as $O
begin declare $O String = 'x'; return $O; end;
create microflow R.B ($T: R.Thing) returns String as $O
begin declare $O String = 'x'; return $O; end;
create persistent entity R.Thing (Name: String(100));`)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 violations, got %d:\n%s", len(msgs), strings.Join(msgs, "\n"))
	}
}

// A statement naming a document created earlier in the script is fine even when
// a LATER statement creates something of the same name in another namespace.
// The rule keys on kind as well as name, so an entity and a microflow sharing a
// qualified name do not shadow each other.
func TestValidateScriptDefinitionOrder_KindsAreSeparateNamespaces(t *testing.T) {
	msgs := buildOrderProgram(t, `create module R;
create persistent entity R.Thing (Name: String(100));
create microflow R.MF ($T: R.Thing) returns String as $O
begin declare $O String = 'x'; return $O; end;
create microflow R.Thing () returns Boolean as $O
begin declare $O Boolean = true; return $O; end;`)
	if len(msgs) > 0 {
		t.Errorf("entity R.Thing exists by then; the later microflow R.Thing is a different namespace:\n%s",
			strings.Join(msgs, "\n"))
	}
}
