// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestValidateMicroflow_DeclareListIsRejected guards issue #607: a list-typed
// `declare` parses and passed `mxcli check`, but Studio Pro rejects it with
// CE0053 ("type not allowed") and CE0038 ("value required") because `declare`
// maps to a Create Variable activity, which cannot produce a list. Lists must
// come from a microflow parameter, a `retrieve`, or a `create list`.
func TestValidateMicroflow_DeclareListIsRejected(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name: "empty initializer (the reported case)",
			input: `create microflow Synthetic.MF_DeclareEmptyList ()
begin
  declare $Items list of Synthetic.Item = empty;
end;`,
		},
		{
			name: "non-empty initializer is still a Create Variable on a list",
			input: `create microflow Synthetic.MF_DeclareList ()
begin
  declare $Items list of Synthetic.Item = $Other;
end;`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := visitor.Build(tc.input)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

			violations := ValidateMicroflow(stmt)
			var found *linter.Violation
			for i := range violations {
				if violations[i].RuleID == "MDL040" {
					found = &violations[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("expected MDL040 for list-typed declare, got %#v", violations)
			}
			if found.Severity != linter.SeverityError {
				t.Errorf("MDL040 severity = %v, want Error (Studio Pro rejects this with CE0053/CE0038)", found.Severity)
			}
			if !strings.Contains(found.Message, "Items") {
				t.Errorf("MDL040 message should name the offending variable, got %q", found.Message)
			}
		})
	}
}

// TestValidateMicroflow_DeclarePrimitiveIsAllowed ensures the declare rules are
// scoped to lists (MDL040) and objects (MDL043) only — a primitive declare, and
// an explicit enumeration declare, must NOT be flagged.
func TestValidateMicroflow_DeclarePrimitiveIsAllowed(t *testing.T) {
	input := `create microflow Synthetic.MF_DeclareScalar ()
begin
  declare $Count Integer = 0;
  declare $Name String = '';
  declare $Status Enumeration(Synthetic.Status) = Synthetic.Status.Open;
end;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL040" || v.RuleID == "MDL043" {
			t.Fatalf("%s must not fire on primitive/explicit-enum declares, got: %q", v.RuleID, v.Message)
		}
	}
}

// TestValidateMicroflow_DeclareObjectIsRejected guards the declare-entity trap: an
// object-typed `declare` parsed and passed `mxcli check`, but mxbuild rejects it
// with CE0053 ("Selected type is not allowed") because `declare` maps to a Create
// Variable activity, which only holds primitives — bare or initialized. Object
// variables must come from a parameter, a retrieve, a create object, or a loop
// iterator. A bare `Module.X` declare type parses as the ambiguous
// TypeEnumeration (EnumRef, ExplicitEnum=false); an explicit `Enumeration(...)`
// is not flagged.
func TestValidateMicroflow_DeclareObjectIsRejected(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			name: "bare object declare with object initializer (the trap)",
			input: `create microflow Synthetic.MF_DeclareObjInit ($In: Synthetic.Customer)
begin
  declare $c Synthetic.Customer = $In;
  change $c (Name = 'x');
end;`,
		},
		{
			name: "bare object declare with no initializer",
			input: `create microflow Synthetic.MF_DeclareObjBare ()
begin
  declare $c Synthetic.Customer;
end;`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog, errs := visitor.Build(tc.input)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

			violations := ValidateMicroflow(stmt)
			var found *linter.Violation
			for i := range violations {
				if violations[i].RuleID == "MDL043" {
					found = &violations[i]
					break
				}
			}
			if found == nil {
				t.Fatalf("expected MDL043 for object-typed declare, got %#v", violations)
			}
			if found.Severity != linter.SeverityError {
				t.Errorf("MDL043 severity = %v, want Error (mxbuild rejects with CE0053)", found.Severity)
			}
			if !strings.Contains(found.Message, "Customer") {
				t.Errorf("MDL043 message should name the offending type, got %q", found.Message)
			}
		})
	}
}
