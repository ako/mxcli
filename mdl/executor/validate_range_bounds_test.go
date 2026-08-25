// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL068, from issue #966.
//
// Mendix's Range list operation requires at least one bound. The grammar makes
// both optional, so `range($List)` parsed, checked clean, and executed — and
// the build then failed:
//
//	[error] [CE6520] "Amount and offset are not specified. Either amount or
//	offset or both must be specified." at List operation activity 'Range'
//
// (measured on mxbuild 11.13.0; the control is the same project with a bound,
// which is 0 errors).
//
// That is the form describe was emitting for every paged range while the
// reader was dropping the bounds, which is what the issue meant by "mxcli check
// passes on the incomplete output". The reader is fixed, so describe no longer
// produces this — but `range($L)` remains reachable by hand, and it should be
// refused where the other unbuildable shapes are, not left to mxbuild.
func TestUnboundedRangeIsRefused(t *testing.T) {
	cases := []struct {
		name string
		op   string
		flag bool
	}{
		{"offset and amount", `range($All, $Offset, $Amount)`, false},
		{"offset only", `range($All, $Offset)`, false},
		{"literal bounds", `range($All, 0, 10)`, false},
		{"no bounds — CE6520", `range($All)`, true},
		// The control: a different list operation with one argument is not a
		// range and must not be swept up by the rule.
		{"head is not a range", `head($All)`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := `create microflow BX.M ($Offset: Integer, $Amount: Integer) returns list of BX.Item
begin
  retrieve $All from BX.Item;
  $Paged = ` + c.op + `;
  return $Paged;
end;`
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
			if !ok {
				t.Fatalf("got %T", prog.Statements[0])
			}
			got := false
			var msg string
			for _, v := range ValidateMicroflow(stmt) {
				if v.RuleID == "MDL068" {
					got, msg = true, v.Message
				}
			}
			if got != c.flag {
				t.Errorf("MDL068 fired = %v, want %v (for %s)\n  message: %s", got, c.flag, c.op, msg)
			}
			if c.flag && got && !strings.Contains(msg, "CE6520") {
				t.Errorf("message should name the build error it prevents: %s", msg)
			}
		})
	}
}
