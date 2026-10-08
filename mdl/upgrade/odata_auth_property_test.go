// SPDX-License-Identifier: Apache-2.0

package upgrade

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// The trailing clause becomes the last property of the list, in the order
// written, before an entity block as well as at the end of the statement.
func TestUpgrade_ODataAuthenticationProperty(t *testing.T) {
	src := "create published odata service M.A ( Path: 'a', Namespace: 'M' ) authentication basic, session;\n" +
		"CREATE OR MODIFY PUBLISHED ODATA SERVICE M.B (\n  Path: 'b',\n  Namespace: 'M'\n)\nAUTHENTICATION MICROFLOW M.Auth, SESSION\n{\n  PUBLISH ENTITY M.E AS 'Es';\n};\n"
	want := "create published odata service M.A ( Path: 'a', Namespace: 'M', Authentication: (basic, session) );\n" +
		"CREATE OR MODIFY PUBLISHED ODATA SERVICE M.B (\n  Path: 'b',\n  Namespace: 'M',\n  Authentication: (MICROFLOW M.Auth, SESSION)\n)\n{\n  PUBLISH ENTITY M.E AS 'Es';\n};\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != want {
		t.Fatalf("got:\n%s\nwant:\n%s", res.Source, want)
	}
	if res.Rewritten[deprecation.ODataAuthenticationClause] != 2 {
		t.Errorf("Rewritten = %v", res.Rewritten)
	}
	if again := mustUpgrade(t, res.Source, Options{}); again.Changed() {
		t.Errorf("second upgrade changed the script again: %v", again.Rewritten)
	}
}

// A method MDL has no keyword for (the clause took any identifier) has no
// property spelling: reported, not rewritten.
func TestUpgrade_ODataAuthenticationCustomIdentifierIsNotRewritten(t *testing.T) {
	src := "create published odata service M.A ( Path: 'a', Namespace: 'M' ) authentication Custom;\n"
	res := mustUpgrade(t, src, Options{})
	if res.Source != src {
		t.Errorf("rewrote it:\n%s", res.Source)
	}
	if len(res.Unrewritten) != 1 || res.Unrewritten[0].Code != deprecation.ODataAuthenticationClause {
		t.Errorf("Unrewritten = %+v", res.Unrewritten)
	}
}
