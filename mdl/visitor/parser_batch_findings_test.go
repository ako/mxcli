// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestRetrieveSortByQuotedQualified guards FINDINGS #13: a quoted, qualified
// `sort by` attribute in a RETRIEVE must store the bare dotted form. Keeping the
// quotes produced a reference that only failed on write ("attribute does not
// belong to entity").
func TestRetrieveSortByQuotedQualified(t *testing.T) {
	input := `create microflow M.DS () returns list of M.Thing as $rows
begin
  retrieve $rows from M.Thing sort by "M"."Thing"."Code" asc;
  return $rows;
end;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	var got string
	for _, s := range mf.Body {
		if r, ok := s.(*ast.RetrieveStmt); ok && len(r.SortColumns) > 0 {
			got = r.SortColumns[0].Attribute
		}
	}
	if got != "M.Thing.Code" {
		t.Errorf("sort attribute = %q, want %q (unquoted dotted)", got, "M.Thing.Code")
	}
}

// TestUserRoleQuotingConsistency guards FINDINGS #5: DESCRIBE and DROP USER ROLE
// both accept bare and quoted names, so the two commands are consistent.
func TestUserRoleQuotingConsistency(t *testing.T) {
	cases := []struct {
		input    string
		wantName string
	}{
		{"describe user role Administrator;", "Administrator"},
		{"describe user role 'Administrator';", "Administrator"},
		{"drop user role User;", "User"},
		{"drop user role 'User';", "User"},
	}
	for _, tc := range cases {
		prog, errs := Build(tc.input)
		if len(errs) > 0 {
			t.Errorf("%q: unexpected parse errors: %v", tc.input, errs)
			continue
		}
		switch s := prog.Statements[0].(type) {
		case *ast.DescribeStmt:
			if s.Name.Name != tc.wantName {
				t.Errorf("%q: describe role name = %q, want %q", tc.input, s.Name.Name, tc.wantName)
			}
		case *ast.DropUserRoleStmt:
			if s.Name != tc.wantName {
				t.Errorf("%q: drop role name = %q, want %q", tc.input, s.Name, tc.wantName)
			}
		default:
			t.Errorf("%q: unexpected statement type %T", tc.input, prog.Statements[0])
		}
	}
}

// TestLogEmptyTemplateParamsIsAnErrorNotAPanic guards FINDINGS §55: `log … with ()`
// crashed mxcli with a nil dereference in buildTemplateParams.
//
// The grammar requires at least one templateParam, so ANTLR error-recovers by
// producing a TemplateParamContext with no NUMBER_LITERAL — which the builder
// dereferenced anyway. Every visitor that walks an error-recovered tree has this
// shape available to it, so the guard belongs at the dereference, not in the
// grammar: a malformed statement must come back as a parse error.
func TestLogEmptyTemplateParamsIsAnErrorNotAPanic(t *testing.T) {
	input := `create microflow M.LogEmpty ()
begin
  log 'hello' with ();
end;`

	_, errs := Build(input) // panicked before the fix
	if len(errs) == 0 {
		t.Fatal("expected a parse error for an empty `with ()` list, got none")
	}
}

// TestLogTemplateParamsStillBuild is the control for the guard above: a well-formed
// `with` list must still produce its parameters. A guard that skipped every param
// would pass the panic test and silently drop the message's arguments.
func TestLogTemplateParamsStillBuild(t *testing.T) {
	input := `create microflow M.LogOne ()
begin
  log 'hello {1} and {2}' with ({1} = 'world', {2} = 'again');
end;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	log, ok := mf.Body[0].(*ast.LogStmt)
	if !ok {
		t.Fatalf("first statement is %T, want *ast.LogStmt", mf.Body[0])
	}
	if len(log.Template) != 2 {
		t.Fatalf("template params = %d, want 2", len(log.Template))
	}
	for i, want := range []int{1, 2} {
		if log.Template[i].Index != want {
			t.Errorf("param %d index = %d, want %d", i, log.Template[i].Index, want)
		}
	}
}
