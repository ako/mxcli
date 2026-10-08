// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// An enumeration value may start with an underscore — Studio Pro names the
// values of a True/False/Unknown enumeration `_True` and `_False`, since the
// bare words are reserved (GenAICommons.ENUM_ModelSupport). XPath compares the
// attribute with the value's name, so `[A = M.E._True]` is stored as
// `[A = '_True']`, which is what Evora's AgentCommons.DS_Agent_GetDeployedModels
// stores and describe prints back as the qualified name.
//
// The stored-form rewrite matched only parts starting with a letter, so the
// value was written verbatim: `[Supports = Probe.Support._True]`, CE0161 on
// mxbuild 10.24.15 (measured), and `check -p` reported the name as "neither an
// attribute nor an association". The writer and the check share
// retrieveXPathConstraint, so the check was telling the truth about the write.
func TestRetrieveXPathConstraint_UnderscoreEnumValue(t *testing.T) {
	cases := map[string]string{
		"Unknown": "[Supports = 'Unknown']", // control: an ordinary value
		"_True":   "[Supports = '_True']",
		"_False":  "[Supports = '_False']",
	}
	for value, want := range cases {
		prog, errs := visitor.Build("create microflow M.F () begin retrieve $L from M.Model where [Supports = M.Support." + value + "]; end;")
		if len(errs) > 0 {
			t.Fatalf("parse: %v", errs)
		}
		r := prog.Statements[0].(*ast.CreateMicroflowStmt).Body[0].(*ast.RetrieveStmt)
		if got := retrieveXPathConstraint(r.Where, "M.Model"); got != want {
			t.Errorf("%s: stored %q, want %q", value, got, want)
		}
	}
	if got := normalizeXPathEnumRefs("[A = M.E._True or B = M.E.Open]"); got != "[A = '_True' or B = 'Open']" {
		t.Errorf("normalizeXPathEnumRefs: got %q", got)
	}
}
