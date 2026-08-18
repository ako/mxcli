// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func qn(module, name string) ast.QualifiedName {
	return ast.QualifiedName{Module: module, Name: name}
}

// TestUserRoleWithoutSystemRoleIsAnError is the regression test for CE0156. A
// user role built only from application module roles cannot sign in or read
// System entities, so the app is unusable for anyone holding it — and the whole
// thing is decidable from the MDL.
func TestUserRoleWithoutSystemRoleIsAnError(t *testing.T) {
	stmt := &ast.CreateUserRoleStmt{
		Name:        "Evaluator",
		ModuleRoles: []ast.QualifiedName{qn("ReplicationLab", "Evaluator")},
	}
	v := ValidateUserRoleSystemModuleRole(stmt)
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if v[0].RuleID != "MDL-SEC20" {
		t.Errorf("RuleID = %q", v[0].RuleID)
	}
	if !strings.Contains(v[0].Message, "System.User") {
		t.Errorf("message does not name the fix:\n%s", v[0].Message)
	}
	if !strings.Contains(v[0].Message, "CE0156") {
		t.Errorf("message does not cite the MxBuild code:\n%s", v[0].Message)
	}
}

func TestUserRoleWithSystemRoleIsAccepted(t *testing.T) {
	for _, sys := range []string{"System", "system", "SYSTEM"} {
		stmt := &ast.CreateUserRoleStmt{
			Name: "Evaluator",
			ModuleRoles: []ast.QualifiedName{
				qn("ReplicationLab", "Evaluator"), qn(sys, "User"),
			},
		}
		if v := ValidateUserRoleSystemModuleRole(stmt); len(v) != 0 {
			t.Errorf("%s.User rejected: %s", sys, v[0].Message)
		}
	}
}

// TestUserRoleWithNoModuleRolesIsNotFlagged — a role declared empty and extended
// later by ALTER USER ROLE is a legitimate shape, not a missing System role.
func TestUserRoleWithNoModuleRolesIsNotFlagged(t *testing.T) {
	if v := ValidateUserRoleSystemModuleRole(&ast.CreateUserRoleStmt{Name: "Placeholder"}); len(v) != 0 {
		t.Errorf("an empty role was flagged: %s", v[0].Message)
	}
}

// TestPageURLMissingParameterSegmentIsAnError is the regression test for CE5601.
// Mendix binds each page parameter from the URL, so one without a {Name} segment
// cannot be opened by link and fails the build.
func TestPageURLMissingParameterSegmentIsAnError(t *testing.T) {
	stmt := &ast.CreatePageStmtV3{
		Name:       ast.QualifiedName{Module: "Lab", Name: "ScenarioDetail"},
		URL:        "scenario",
		Parameters: []ast.PageParameter{{Name: "Scenario"}},
	}
	v := ValidatePageURLParameters(stmt)
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if v[0].RuleID != "MDL-PAGE20" {
		t.Errorf("RuleID = %q", v[0].RuleID)
	}
	for _, want := range []string{"CE5601", `"Scenario"`, "scenario/{Scenario}"} {
		if !strings.Contains(v[0].Message, want) {
			t.Errorf("message does not contain %s:\n%s", want, v[0].Message)
		}
	}
}

func TestPageURLWithEveryParameterIsAccepted(t *testing.T) {
	stmt := &ast.CreatePageStmtV3{
		Name:       ast.QualifiedName{Module: "Lab", Name: "P"},
		URL:        "run/{Scenario}/{Step}",
		Parameters: []ast.PageParameter{{Name: "Scenario"}, {Name: "Step"}},
	}
	if v := ValidatePageURLParameters(stmt); len(v) != 0 {
		t.Errorf("a complete URL was rejected: %s", v[0].Message)
	}
}

// TestPageWithParametersAndNoURLIsNotFlagged — parameters without a URL are
// entirely normal; the rule only applies once a deep link exists.
func TestPageWithParametersAndNoURLIsNotFlagged(t *testing.T) {
	stmt := &ast.CreatePageStmtV3{
		Name:       ast.QualifiedName{Module: "Lab", Name: "P"},
		Parameters: []ast.PageParameter{{Name: "Scenario"}},
	}
	if v := ValidatePageURLParameters(stmt); len(v) != 0 {
		t.Errorf("a URL-less page was flagged: %s", v[0].Message)
	}
}

// TestPageURLNamesEveryMissingParameter — reporting only the first would send
// the author round the loop once per parameter.
func TestPageURLNamesEveryMissingParameter(t *testing.T) {
	stmt := &ast.CreatePageStmtV3{
		Name:       ast.QualifiedName{Module: "Lab", Name: "P"},
		URL:        "run",
		Parameters: []ast.PageParameter{{Name: "Scenario"}, {Name: "Step"}},
	}
	v := ValidatePageURLParameters(stmt)
	if len(v) != 1 {
		t.Fatalf("violations = %d, want 1", len(v))
	}
	if !strings.Contains(v[0].Message, `"Scenario" and "Step"`) {
		t.Errorf("message does not name both:\n%s", v[0].Message)
	}
	if !strings.Contains(v[0].Message, "run/{Scenario}/{Step}") {
		t.Errorf("suggestion does not add both segments:\n%s", v[0].Message)
	}
}

// TestPageURLAcceptsAnAttributePathSegment. Mendix binds a parameter by one of
// its attributes — `url: 'p006_dataform/{Customer/Name}'` is the shape mxcli's
// own page examples use — so matching `{Customer}` exactly would flag correct
// URLs as errors. That false positive is worse than the missing rule was.
func TestPageURLAcceptsAnAttributePathSegment(t *testing.T) {
	stmt := &ast.CreatePageStmtV3{
		Name:       ast.QualifiedName{Module: "PgTest", Name: "P006_DataForm"},
		URL:        "p006_dataform/{Customer/Name}",
		Parameters: []ast.PageParameter{{Name: "Customer"}},
	}
	if v := ValidatePageURLParameters(stmt); len(v) != 0 {
		t.Errorf("an attribute-path URL was flagged: %s", v[0].Message)
	}
}

func TestURLBindsParameterFormats(t *testing.T) {
	cases := []struct {
		url, param string
		want       bool
	}{
		{"scenario/{Scenario}", "Scenario", true},
		{"p006/{Customer/Name}", "Customer", true},
		{"p006/{$Customer}", "Customer", true},
		{"p006/{customer}", "Customer", true},
		{"a/{Other}/b/{Scenario}", "Scenario", true},
		{"scenario", "Scenario", false},
		{"scenario/{Other}", "Scenario", false},
		{"scenario/{ScenarioExtra}", "Scenario", false},
		{"scenario/{unclosed", "Scenario", false},
	}
	for _, c := range cases {
		if got := urlBindsParameter(c.url, c.param); got != c.want {
			t.Errorf("urlBindsParameter(%q, %q) = %v, want %v", c.url, c.param, got, c.want)
		}
	}
}
