// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func boolReturn() *ast.MicroflowReturnType {
	return &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}}
}

// A rule's return type is not a style preference: mxbuild rejects anything but
// Boolean or an enumeration with CE0103 + CE0139, measured on 11.13.0 by
// converting a microflow unit into a rule to get past this validator.
func TestRuleReturnTypeMustBeBooleanOrEnumeration(t *testing.T) {
	cases := []struct {
		name    string
		ret     *ast.MicroflowReturnType
		wantErr bool
	}{
		{"boolean", boolReturn(), false},
		{"enumeration", &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeEnumeration}}, false},
		{"absent", nil, true},
		{"void", &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeVoid}}, true},
		{"string", &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeString}}, true},
		{"integer", &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeInteger}}, true},
	}
	for _, c := range cases {
		msg := validateRuleReturnType(c.ret)
		if (msg != "") != c.wantErr {
			t.Errorf("%s: got %q, wantErr=%v", c.name, msg, c.wantErr)
		}
	}
}

// The activities Mendix forbids in a rule. mxbuild reports CE0009 "This action
// is not supported in rules." for each; a rule that reached the build carrying
// one is a document mxcli should never have written.
func TestRuleBodyRefusesDisallowedActivities(t *testing.T) {
	cases := []struct {
		name string
		stmt ast.MicroflowStatement
		want string
	}{
		{"create", &ast.CreateObjectStmt{}, "cannot create objects"},
		{"change", &ast.ChangeObjectStmt{}, "cannot change objects"},
		{"delete", &ast.DeleteObjectStmt{}, "cannot delete objects"},
		{"commit", &ast.MfCommitStmt{}, "cannot commit objects"},
		{"rollback", &ast.RollbackStmt{}, "cannot roll back objects"},
		{"show page", &ast.ShowPageStmt{}, "cannot show a page"},
		{"close page", &ast.ClosePageStmt{}, "cannot close a page"},
		{"show message", &ast.ShowMessageStmt{}, "cannot show a message"},
		{"validation feedback", &ast.ValidationFeedbackStmt{}, "cannot send validation feedback"},
		{"download", &ast.DownloadFileStmt{}, "cannot start a file download"},
		{"web service", &ast.CallWebServiceStmt{}, "cannot call a web service"},
	}
	for _, c := range cases {
		errs := validateRuleBody([]ast.MicroflowStatement{c.stmt})
		if len(errs) != 1 || !strings.Contains(errs[0], c.want) {
			t.Errorf("%s: got %v, want one error containing %q", c.name, errs, c.want)
		}
	}
}

// A rule may do everything a rule is FOR — retrieve, branch, call a microflow,
// compute. Without this control the denylist could reject the whole language and
// the test above would still pass.
func TestRuleBodyAllowsWhatARuleIsFor(t *testing.T) {
	allowed := []ast.MicroflowStatement{
		&ast.RetrieveStmt{},
		&ast.CallMicroflowStmt{},
		&ast.DeclareStmt{},
		&ast.ReturnStmt{},
	}
	if errs := validateRuleBody(allowed); len(errs) != 0 {
		t.Errorf("a rule must be allowed to retrieve, call and compute; got %v", errs)
	}
}

// A disallowed activity nested inside a branch or a loop is still disallowed —
// hiding a create inside an `if` must not get it past the check.
func TestRuleBodyWalksNestedBodies(t *testing.T) {
	nested := []ast.MicroflowStatement{
		&ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{&ast.CreateObjectStmt{}},
			ElseBody: []ast.MicroflowStatement{
				&ast.LoopStmt{Body: []ast.MicroflowStatement{&ast.ShowMessageStmt{}}},
			},
		},
	}
	errs := validateRuleBody(nested)
	if len(errs) != 2 {
		t.Fatalf("got %d errors %v, want 2 (one per nested body)", len(errs), errs)
	}
}

// validateRule is what both `check` and `exec` call, so the two cannot disagree
// about what a rule may contain. It reports every problem at once rather than
// the first.
func TestValidateRuleReportsEveryProblem(t *testing.T) {
	msg := validateRule("Mod.R",
		[]ast.MicroflowStatement{&ast.CreateObjectStmt{}, &ast.ShowPageStmt{}},
		&ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeString}})
	for _, want := range []string{"Mod.R", "not String", "cannot create objects", "cannot show a page"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q; got:\n%s", want, msg)
		}
	}
	if validateRule("Mod.R", []ast.MicroflowStatement{&ast.ReturnStmt{}}, boolReturn()) != "" {
		t.Error("a valid rule must produce no message")
	}
}
