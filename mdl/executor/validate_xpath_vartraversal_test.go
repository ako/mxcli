// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestXPathVariableTraversal covers issue #831: a retrieve constraint whose
// right-hand side traverses an association FROM a variable
//
//	where [Name = $RefProduct/Mod.Product_Category/Name]
//
// passed `mxcli check` and `mxcli exec`, and the build then failed CE0161.
//
// The valid/invalid boundary was established against mxbuild 11.6.6, and it is
// narrower than "a qualified name after a variable":
//
//	$Var/Attr                 VALID  — a parameter's own attribute
//	$Var/Mod.Assoc            VALID  — one hop to the associated object
//	$Var/Mod.Assoc/Attr       CE0161 — two or more hops
//
// so the rule must key on the number of segments, not on the presence of a
// module-qualified one. Flagging the middle form would reject valid MDL.
func TestXPathVariableTraversal(t *testing.T) {
	cases := []struct {
		name  string
		where string
		flag  bool
	}{
		{"attribute of a parameter", `[Name = $P/Code]`, false},
		{"one hop to the associated object", `[BX.Product_Category = $P/BX.Product_Category]`, false},
		{"two hops — the reported form", `[Name = $P/BX.Product_Category/Name]`, true},
		{"three hops", `[Name = $P/BX.A_B/BX.B_C/Name]`, true},
		// An entity-rooted traversal is not variable-rooted and is valid XPath.
		{"entity-rooted traversal", `[BX.Product_Category/BX.Product = $P]`, false},
		{"bare attribute compare", `[Name = 'x']`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src := `create microflow BX.M ($P: BX.Product) returns list of BX.Category
begin
  retrieve $L from BX.Category where ` + c.where + `;
  return $L;
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
				if v.RuleID == "MDL055" {
					got, msg = true, v.Message
				}
			}
			if got != c.flag {
				t.Errorf("MDL055 fired = %v, want %v (where %s)\n  message: %s", got, c.flag, c.where, msg)
			}
			if c.flag && got && !strings.Contains(msg, "$P") {
				t.Errorf("message should name the offending variable path: %s", msg)
			}
		})
	}
}
