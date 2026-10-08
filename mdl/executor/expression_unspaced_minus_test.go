// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestUnspacedMinusAfterMemberIsSubtraction is the regression test for
// describe output that did not parse: Studio Pro stores an expression exactly
// as the developer typed it, so a subtraction with no spaces around the minus
// is ordinary stored text — `$Pagination/PageSize*($Pagination/Offset-1)`,
// `(($I/OEE-$B/OEE) div $B/OEE)*100` (Evora Factory Management). The lexer's
// HYPHENATED_ID rule, which exists for names such as `starts-with`, also
// matched `Offset-1` and the trailing-hyphen `OEE-`, so the member and the
// operator became one token and the statement failed with `missing '('`.
//
// The test asserts the stored text, not just the parse: describe prints the
// stored expression verbatim, so for describe → exec to report Unchanged the
// writer must store back the same bytes it was given.
func TestUnspacedMinusAfterMemberIsSubtraction(t *testing.T) {
	cases := []struct{ name, expr string }{
		{"digit after the hyphen", `$P/PageSize*($P/Offset-1)`},
		{"variable after the hyphen", `(($I/OEE-$B/OEE) div $B/OEE)*100`},
		{"bare member then digit", `$P/Count-1`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fb := buildFlowFromMDL(t, "  declare $z Decimal = "+tc.expr+";")
			var got string
			found := false
			for _, o := range fb.objects {
				a, ok := o.(*microflows.ActionActivity)
				if !ok {
					continue
				}
				if cv, ok := a.Action.(*microflows.CreateVariableAction); ok {
					got, found = cv.InitialValue, true
				}
			}
			if !found {
				t.Fatal("no create-variable activity was built")
			}
			if got != tc.expr {
				t.Errorf("stored expression = %q, want %q (verbatim)", got, tc.expr)
			}
		})
	}
}
