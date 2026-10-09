// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func systemAssocRetrieveViolations(t *testing.T, stmt *ast.CreateMicroflowStmt) []string {
	t.Helper()
	var out []string
	for _, v := range ValidateMicroflow(stmt) {
		if v.RuleID == retrieveSystemAssocRule {
			out = append(out, v.Message+" | "+v.Suggestion)
		}
	}
	return out
}

func retrieveByAssoc(variable, start, module, name string) *ast.RetrieveStmt {
	return &ast.RetrieveStmt{
		Variable:      variable,
		StartVariable: start,
		Source:        ast.QualifiedName{Module: module, Name: name},
	}
}

// TestRetrieveOverSystemOwnerAssociation is mendixlabs/mxcli#1358:
// `retrieve $u from $task/System.owner;` passed check and exec, and mxbuild then
// reported `[CE0136] "Retrieve object must specify the 'Entity' property."`.
// Measured on 11.12.2: mxbuild resolves the name (System.Owner is CE1613
// instead) but derives no entity from it, with or without `owner: AutoOwner`.
func TestRetrieveOverSystemOwnerAssociation(t *testing.T) {
	t.Run("association retrieve over System.owner is refused", func(t *testing.T) {
		got := systemAssocRetrieveViolations(t, mfWith(retrieveByAssoc("u", "task", "System", "owner")))
		if len(got) != 1 {
			t.Fatalf("want one %s violation, got %v", retrieveSystemAssocRule, got)
		}
		for _, want := range []string{"CE0136", "System.owner", "[id = $task/System.owner]"} {
			if !strings.Contains(got[0], want) {
				t.Errorf("violation does not mention %q: %s", want, got[0])
			}
		}
	})

	t.Run("System.changedBy, nested in a branch, is refused", func(t *testing.T) {
		got := systemAssocRetrieveViolations(t, mfWith(&ast.IfStmt{
			ThenBody: []ast.MicroflowStatement{retrieveByAssoc("u", "task", "System", "changedBy")},
		}))
		if len(got) != 1 || !strings.Contains(got[0], "[id = $task/System.changedBy]") {
			t.Fatalf("want one violation naming the System.changedBy rewrite, got %v", got)
		}
	})

	// Controls: the forms that build.
	t.Run("a modelled association to System.User is not flagged", func(t *testing.T) {
		if got := systemAssocRetrieveViolations(t, mfWith(retrieveByAssoc("u", "task", "MyFirstModule", "Task_User"))); len(got) != 0 {
			t.Fatalf("unexpected violation: %v", got)
		}
	})
	t.Run("the database rewrite is not flagged", func(t *testing.T) {
		db := &ast.RetrieveStmt{Variable: "u", Source: ast.QualifiedName{Module: "System", Name: "User"}, First: true}
		if got := systemAssocRetrieveViolations(t, mfWith(db)); len(got) != 0 {
			t.Fatalf("unexpected violation: %v", got)
		}
	})
	t.Run("System.UserRoles is a modelled System association and is not flagged", func(t *testing.T) {
		if got := systemAssocRetrieveViolations(t, mfWith(retrieveByAssoc("roles", "user", "System", "UserRoles"))); len(got) != 0 {
			t.Fatalf("unexpected violation: %v", got)
		}
	})

	t.Run("exec refuses it too", func(t *testing.T) {
		err := validateMicroflowRules(mfWith(retrieveByAssoc("u", "task", "System", "owner")))
		if err == nil || !strings.Contains(err.Error(), retrieveSystemAssocRule) {
			t.Fatalf("exec must refuse the write, got %v", err)
		}
	})
}
