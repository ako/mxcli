// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// mendixlabs/mxcli#1326: an operator inside an XPath function argument passed
// check and exec, then failed the build with CE0161 on mxbuild 11.14.0 — the
// measured pairs are on xpathArithmeticOperators. The bracketed `[…]` form
// does not parse an operator in a function argument at all, so only the
// unbracketed form reached the writer.
func TestValidateMicroflow_XPathFunctionArgumentOperator(t *testing.T) {
	cases := []struct {
		name    string
		where   string
		wantMDL bool
	}{
		{"concatenation in starts-with", "starts-with(Name, 'MS-' + $Key)", true},
		{"parenthesised argument", "contains(Name, ($Key + 'x'))", true},
		{"inside not()", "not(ends-with(Name, $Key + '-X'))", true},
		{"subtraction in contains", "contains(Name, $Key - 'a')", true},
		{"inside and", "Name != empty and ends-with(Name, '-' + $Key)", true},
		{"top-level concatenation is fine", "Name = 'X-' + $Key", false},
		{"operator beside a call is fine", "year-from-dateTime(Due) = $N - 1", false},
		{"pre-computed argument is fine", "starts-with(Name, $Key)", false},
		{"operator in a literal is text", "starts-with(Name, 'a + b')", false},
		{"comparison inside not() is fine", "not(Name = $Key)", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "create microflow M.F ($Key: String, $N: Integer)\nreturns list of M.Item\nbegin\n  retrieve $L from M.Item where " + tc.where + ";\n  return $L;\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			var msgs []string
			for _, vi := range ValidateMicroflow(mf) {
				if vi.RuleID == "MDL091" {
					msgs = append(msgs, vi.Message)
				}
			}
			if got := len(msgs) > 0; got != tc.wantMDL {
				t.Fatalf("MDL091 fired=%v, want %v (where: %q) %v", got, tc.wantMDL, tc.where, msgs)
			}
			if tc.wantMDL && !strings.Contains(strings.Join(msgs, "\n"), "CE0161") {
				t.Errorf("message must name CE0161: %v", msgs)
			}
		})
	}
}

// check and exec must agree: the report had both accepting the constraint.
func TestCheckExecAgree_RetrieveConstraintFunctionArgumentOperator(t *testing.T) {
	const head = `mdl 1;
create module ModG;
create persistent entity ModG.Item (Name: String(50));
create microflow ModG.SUB_PlusInFunc ($Key: String)
begin
  retrieve $L from ModG.Item where %s;
end;`
	exec, _, dir := openPedAppCopy(t)
	bad := strings.Replace(head, "%s", "starts-with(Name, 'MS-' + $Key)", 1)
	got := strings.Join(agreeCheck(t, exec, dir, bad), "\n")
	if !strings.Contains(got, "MDL091") || !strings.Contains(got, "starts-with()") {
		t.Errorf("check -p must report MDL091 for the operator, reported:\n%s", got)
	}
	if err := agreeExec(t, exec, bad); err == nil || !strings.Contains(err.Error(), "MDL091") {
		t.Errorf("exec must refuse the operator with MDL091, got: %v", err)
	}

	// The two shapes the report measured clean must stay accepted.
	for _, ok := range []string{"Name = 'X-' + $Key", "starts-with(Name, $Key)"} {
		exec2, _, dir2 := openPedAppCopy(t)
		assertAgree(t, exec2, dir2, strings.Replace(head, "%s", ok, 1), "")
	}
}
