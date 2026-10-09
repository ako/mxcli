// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// describe writes a variable's default bare (R5), and re-executing that
// output stores the same default. A default that is itself a string, or
// empty, has no bare spelling yet (MDL-DEPR086) and keeps the string form.
func TestDescribePageVariableDefault_RoundTrip(t *testing.T) {
	for _, tc := range []struct{ stored, want string }{
		{"true", "$v: Boolean = true"},
		{"if (3 < 4) then true else false", "$v: Boolean = if (3 < 4) then true else false"},
		{"[%CurrentDateTime%]", "$v: Boolean = [%CurrentDateTime%]"},
		{"'abc'", "$v: Boolean = '''abc'''"},
		{"", "$v: Boolean = ''"},
	} {
		got := pageVariableMDL(&ExecContext{Output: &bytes.Buffer{}}, "v", "Boolean", tc.stored)
		if got != tc.want {
			t.Errorf("stored %q: describe wrote %q, want %q", tc.stored, got, tc.want)
		}
		prog := parseMDL(t, "create page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default, Variables: ( "+got+" )) { };")
		vars := prog.Statements[0].(*ast.CreatePageStmtV3).Variables
		if len(vars) != 1 || vars[0].DefaultValue != tc.stored {
			t.Errorf("re-exec of %q stores %+v, want default %q", got, vars, tc.stored)
		}
	}
}
