// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func ruleFired(src, rule string) (bool, string) {
	prog, errs := visitor.Build(src)
	if len(errs) > 0 || prog == nil {
		return false, "parse errors"
	}
	for _, v := range ValidateProgram(prog, "") {
		if v.RuleID == rule {
			return true, v.Message
		}
	}
	return false, ""
}

// `create association Order_Probe from …` passed `check --references` and was
// then refused by `exec` with "module name is required". exec is not
// transactional, so the statements before it were already applied and re-running
// hit "already exists" on them (mendixlabs/mxcli#1050).
func TestValidateCreateIsQualified(t *testing.T) {
	fired, msg := ruleFired(`create association Order_Probe from A.Order to A.Customer type reference;`, "MDL074")
	if !fired {
		t.Fatal("an unqualified CREATE was accepted")
	}
	for _, want := range []string{"Order_Probe", "module"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message should mention %q: %s", want, msg)
		}
	}

	// It is not association-specific: the same gap existed for every document.
	for _, src := range []string{
		`create entity Thing ( Code: String(10) );`,
		`create microflow ACT_Thing () begin log info 'x'; end;`,
		`create enumeration Colours ( Red 'Red' );`,
	} {
		if ok, _ := ruleFired(src, "MDL074"); !ok {
			t.Errorf("unqualified CREATE accepted: %s", src)
		}
	}
}

// CONTROL: a qualified CREATE passes, and a MODULE — which has nothing to be
// qualified by — must never be reported.
func TestValidateCreateIsQualified_Controls(t *testing.T) {
	for _, src := range []string{
		`create module MyModule;`,
		`create entity A.Thing ( Code: String(10) );`,
		`create association A.Order_Customer from A.Order to A.Customer type reference;`,
	} {
		if ok, msg := ruleFired(src, "MDL074"); ok {
			t.Errorf("reported a valid statement: %s -> %s", src, msg)
		}
	}
}

// `RETURNS void AS $result` passed check and wrote `return $result` into a flow
// with no such variable: [CE0109] "Undefined variable 'result'." at End event
// (mendixlabs/mxcli#1041).
func TestValidateVoidReturnAlias(t *testing.T) {
	fired, msg := ruleFired(
		`create microflow A.P () returns void as $result begin log info 'x'; end;`, "MDL075")
	if !fired {
		t.Fatal("`RETURNS void AS $result` was accepted")
	}
	for _, want := range []string{"result", "CE0109"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message should mention %q: %s", want, msg)
		}
	}
}

// CONTROL: the two spellings that are correct. `RETURNS void` alone is the fix
// the report itself identified as round-tripping cleanly, and a typed return
// with an alias is the ordinary shape — a rule that flagged either would break
// far more than it fixed.
func TestValidateVoidReturnAlias_Controls(t *testing.T) {
	for _, src := range []string{
		`create microflow A.P () returns void begin log info 'x'; end;`,
		`create microflow A.P () begin log info 'x'; end;`,
		`create microflow A.P () returns string as $out begin set $out = 'x'; return $out; end;`,
	} {
		if ok, msg := ruleFired(src, "MDL075"); ok {
			t.Errorf("reported a valid flow: %s -> %s", src, msg)
		}
	}
}

// Both rules need NO project: a divergence that only `-p` catches is still a
// divergence, and these are answerable from the statement alone.
func TestCreateShapeRules_NeedNoProject(t *testing.T) {
	prog, _ := visitor.Build(`create entity Thing ( Code: String(10) );`)
	if len(ValidateCreateIsQualified(prog)) == 0 {
		t.Error("MDL074 did not fire without a project")
	}
	prog, _ = visitor.Build(`create microflow A.P () returns void as $r begin log info 'x'; end;`)
	if len(ValidateVoidReturnAlias(prog)) == 0 {
		t.Error("MDL075 did not fire without a project")
	}
	// CONTROL: a nil program must not panic.
	if len(ValidateCreateIsQualified(nil))+len(ValidateVoidReturnAlias(nil)) != 0 {
		t.Error("a nil program produced violations")
	}
	_ = ast.Program{}
}
