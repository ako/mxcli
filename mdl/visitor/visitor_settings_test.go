// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mustParseSettings builds a program and fails the test on any parse error.
func mustParseSettings(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		t.FailNow()
	}
	if len(prog.Statements) == 0 {
		t.Fatalf("no statements parsed from %q", src)
	}
	return prog
}

func TestAlterSettings_Model(t *testing.T) {
	input := `ALTER SETTINGS MODEL DefaultLanguage = 'en_US';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterSettingsStmt)
	if !ok {
		t.Fatalf("Expected AlterSettingsStmt, got %T", prog.Statements[0])
	}
	if stmt.Section != "MODEL" {
		t.Errorf("Got Section %q", stmt.Section)
	}
	if stmt.Properties["DefaultLanguage"] != "en_US" {
		t.Errorf("Got %v", stmt.Properties["DefaultLanguage"])
	}
}

func TestAlterSettings_Constant(t *testing.T) {
	input := `ALTER SETTINGS CONSTANT 'MyModule.APIEndpoint' VALUE 'https://prod.example.com';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterSettingsStmt)
	if !ok {
		t.Fatalf("Expected AlterSettingsStmt, got %T", prog.Statements[0])
	}
	if stmt.ConstantId != "MyModule.APIEndpoint" {
		t.Errorf("Got ConstantId %q", stmt.ConstantId)
	}
	if stmt.Value != "https://prod.example.com" {
		t.Errorf("Got Value %q", stmt.Value)
	}
}

func TestAlterSettings_DropConstant(t *testing.T) {
	input := `ALTER SETTINGS DROP CONSTANT 'MyModule.OldConst';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.AlterSettingsStmt)
	if !ok {
		t.Fatalf("Expected AlterSettingsStmt, got %T", prog.Statements[0])
	}
	if !stmt.DropConstant {
		t.Error("Expected DropConstant true")
	}
}

func TestCreateConfiguration(t *testing.T) {
	input := `CREATE CONFIGURATION 'Acceptance' DatabaseHost = 'db.acc.example.com', DatabaseName = 'myapp_acc';`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreateConfigurationStmt)
	if !ok {
		t.Fatalf("Expected CreateConfigurationStmt, got %T", prog.Statements[0])
	}
	if stmt.Name != "Acceptance" {
		t.Errorf("Got Name %q", stmt.Name)
	}
	if stmt.Properties["DatabaseHost"] != "db.acc.example.com" {
		t.Errorf("Got %v", stmt.Properties["DatabaseHost"])
	}
}

// TestAlterSettings_WorkflowGroup covers the four GROUP verbs and, with them, the
// one thing that made the first draft of this grammar unusable: `Description` is
// an MDL keyword (DESCRIPTION, from the security statements), so an option key
// typed as IDENTIFIER made the feature's only option a parse error — "mismatched
// input 'Description' expecting IDENTIFIER" — on the statement the feature exists
// for. The option key is identifierOrKeyword for that reason.
func TestAlterSettings_WorkflowGroup(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		check func(*testing.T, *ast.AlterSettingsStmt)
	}{
		{
			"add with description",
			"alter settings workflows add group 'Approvers' (Description: 'Primary approval group');",
			func(t *testing.T, s *ast.AlterSettingsStmt) {
				if !s.AddGroup || s.UpsertGroup || s.ModifyGroup || s.RemoveGroup {
					t.Errorf("verbs = add:%t upsert:%t modify:%t remove:%t", s.AddGroup, s.UpsertGroup, s.ModifyGroup, s.RemoveGroup)
				}
				if s.Properties["Description"] != "Primary approval group" {
					t.Errorf("Properties = %#v", s.Properties)
				}
			},
		},
		{
			"add without options",
			"alter settings workflows add group 'Reviewers';",
			func(t *testing.T, s *ast.AlterSettingsStmt) {
				if !s.AddGroup || len(s.Properties) != 0 {
					t.Errorf("stmt = %+v", s)
				}
			},
		},
		{
			"add or modify",
			"alter settings workflows add or modify group 'Approvers' (Description: 'd');",
			func(t *testing.T, s *ast.AlterSettingsStmt) {
				if !s.UpsertGroup || s.AddGroup || s.ModifyGroup {
					t.Errorf("verbs = add:%t upsert:%t modify:%t", s.AddGroup, s.UpsertGroup, s.ModifyGroup)
				}
			},
		},
		{
			"modify",
			"alter settings workflows modify group 'Approvers' (Description: 'd');",
			func(t *testing.T, s *ast.AlterSettingsStmt) {
				if !s.ModifyGroup || s.UpsertGroup {
					t.Errorf("verbs = upsert:%t modify:%t", s.UpsertGroup, s.ModifyGroup)
				}
			},
		},
		{
			"remove",
			"alter settings workflows remove group 'Approvers';",
			func(t *testing.T, s *ast.AlterSettingsStmt) {
				if !s.RemoveGroup {
					t.Errorf("stmt = %+v", s)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog := mustParseSettings(t, tc.src)
			stmt, ok := prog.Statements[0].(*ast.AlterSettingsStmt)
			if !ok {
				t.Fatalf("Expected AlterSettingsStmt, got %T", prog.Statements[0])
			}
			if !strings.EqualFold(stmt.Section, "workflows") {
				t.Errorf("Section = %q, want workflows", stmt.Section)
			}
			if stmt.GroupName != "Approvers" && stmt.GroupName != "Reviewers" {
				t.Errorf("GroupName = %q", stmt.GroupName)
			}
			// The GROUP forms must not be mistaken for the LANGUAGE ones: they
			// share the clause, and a stray AddLanguage would send the statement
			// to the language handler.
			if stmt.AddLanguage || stmt.ModifyLanguage || stmt.UpsertLanguage || stmt.RemoveLanguage {
				t.Errorf("a GROUP statement set a LANGUAGE verb: %+v", stmt)
			}
			tc.check(t, stmt)
		})
	}
}

// The LANGUAGE forms share the clause and must keep working unchanged — the
// options rule they use was renamed and generalised for the GROUP forms.
func TestAlterSettings_LanguageStillParsesAfterGroupForms(t *testing.T) {
	prog := mustParseSettings(t, "alter settings LANGUAGE add or modify 'ar_SD' (CheckCompleteness: true);")
	stmt := prog.Statements[0].(*ast.AlterSettingsStmt)
	if !stmt.UpsertLanguage || stmt.LanguageCode != "ar_SD" {
		t.Fatalf("stmt = %+v", stmt)
	}
	if stmt.Properties["CheckCompleteness"] != "true" {
		t.Errorf("Properties = %#v", stmt.Properties)
	}
	if stmt.AddGroup || stmt.UpsertGroup || stmt.GroupName != "" {
		t.Errorf("a LANGUAGE statement set a GROUP field: %+v", stmt)
	}
}

func TestShowWorkflowGroups_Parses(t *testing.T) {
	prog := mustParseSettings(t, "show workflow groups;")
	stmt, ok := prog.Statements[0].(*ast.ShowStmt)
	if !ok {
		t.Fatalf("Expected ShowStmt, got %T", prog.Statements[0])
	}
	if stmt.ObjectType != ast.ShowWorkflowGroups {
		t.Errorf("ObjectType = %v, want ShowWorkflowGroups", stmt.ObjectType)
	}
}

// `show workflows` and `show workflow groups` are different statements sharing a
// prefix; the listing one must not be captured by the new alternative.
func TestShowWorkflows_StillListsWorkflows(t *testing.T) {
	prog := mustParseSettings(t, "show workflows;")
	stmt := prog.Statements[0].(*ast.ShowStmt)
	if stmt.ObjectType != ast.ShowWorkflows {
		t.Errorf("ObjectType = %v, want ShowWorkflows", stmt.ObjectType)
	}
}
