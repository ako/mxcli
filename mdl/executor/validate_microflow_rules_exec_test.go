// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestValidateMicroflowRules_ReachedFromExec guards issue #833, which is the
// same shape as #836: a rule that exists and fires in `mxcli check` but is
// never reached from the exec path, so `exec` writes the very construct
// `check` rejects.
//
// ValidateMicroflow (the MDL0xx rule set) was wired only into cmd_check.go and
// the LSP. The exec path called ValidateMicroflowBody, a different function
// with a different rule set, so all 17 error-severity microflow rules were
// check-only. #833 reported it through MDL048 (`[id = $StringVar]`), but the
// gap was never specific to that rule.
//
// Only a VERIFIED subset is promoted (execEnforcedMicroflowRules). Blanket
// promotion was tried and rejected — MDL009 is a false positive, so making the
// whole set a write barrier would refuse valid MDL. Warnings are never promoted.
func TestValidateMicroflowRules_ReachedFromExec(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantErr string // substring; "" means exec-validation must accept
	}{
		{
			// MDL048: comparing the object id against a String value.
			name: "id compared to a string variable is rejected",
			src: `create microflow M.ACT ($GuidText: String) returns M.Item
begin
  retrieve $Found from M.Item where [id = $GuidText] limit 1;
  return $Found;
end;`,
			wantErr: "MDL048",
		},
		{
			// MDL055: two-hop traversal off a variable.
			name: "variable association traversal is rejected",
			src: `create microflow M.ACT ($P: M.Product) returns list of M.Category
begin
  retrieve $L from M.Category where [Name = $P/M.Product_Category/Name];
  return $L;
end;`,
			wantErr: "MDL055",
		},
		{
			// The valid counterpart of the same statement must still pass.
			name: "one-hop traversal is accepted",
			src: `create microflow M.ACT ($P: M.Product) returns list of M.Category
begin
  retrieve $L from M.Category where [Name = $P/Code];
  return $L;
end;`,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build(c.src)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
			err := validateMicroflowRules(stmt)
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("valid microflow rejected by exec validation: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected exec validation to reject this (%s); `check` already does", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error should name the rule %s so it matches what check prints, got: %v", c.wantErr, err)
			}
		})
	}
}

// A warning-severity rule must not block exec — only errors do. MDL001/MDL002
// and friends are advisory, and turning them into hard exec failures would
// break scripts that check reports as passing.
func TestValidateMicroflowRules_WarningsDoNotBlockExec(t *testing.T) {
	// MDL006 (warning): a loop with no body statements.
	src := `create microflow M.ACT () returns Boolean
begin
  declare $x integer = 1;
  return true;
end;`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	// Whatever warnings this trips, none may become an exec error.
	if err := validateMicroflowRules(stmt); err != nil {
		t.Errorf("warning-only microflow must not fail exec validation, got: %v", err)
	}
}

// TestValidateMicroflowRules_UnverifiedRulesNotPromoted pins the deliberate
// narrowness of execEnforcedMicroflowRules.
//
// MDL008 is a CORRECT rule (mxbuild rejects `else` on an enum split with CE0079
// per uncovered value plus CE0773) that is nonetheless not on the allowlist:
// membership requires a verified construct, and correctness alone is not the
// bar — every promoted rule becomes a hard write barrier. This test fails the
// moment someone widens the allowlist wholesale.
func TestValidateMicroflowRules_UnverifiedRulesNotPromoted(t *testing.T) {
	src := `create microflow M.ACT ($S: Enumeration(M.Status)) returns String
begin
  case $S
    when Open then
      return 'a';
    when (empty) then
      return 'b';
    else
      return 'c';
  end case;
end;`
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	sawInCheck := false
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == "MDL008" {
			sawInCheck = true
		}
	}
	if !sawInCheck {
		t.Fatal("expected check to report MDL008 for an else branch on an enum split")
	}
	if err := validateMicroflowRules(stmt); err != nil {
		t.Errorf("MDL008 is not on the verified allowlist and must not block exec, got: %v", err)
	}
}
