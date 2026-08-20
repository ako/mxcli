// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// folderOf returns the Folder field of whatever create statement src produced,
// so one table can cover doctypes with unrelated AST types.
func folderOf(t *testing.T, src string) (string, bool) {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors for %q: %v", src, errs)
	}
	for _, s := range prog.Statements {
		switch v := s.(type) {
		case *ast.CreateImportMappingStmt:
			return v.Folder, true
		case *ast.CreateExportMappingStmt:
			return v.Folder, true
		case *ast.CreateQueueStmt:
			return v.Folder, true
		case *ast.CreateRegularExpressionStmt:
			return v.Folder, true
		case *ast.CreateScheduledEventStmt:
			return v.Folder, true
		case *ast.CreateImageCollectionStmt:
			return v.Folder, true
		case *ast.CreateMenuStmt:
			return v.Folder, true
		case *ast.CreateJavaActionStmt:
			return v.Folder, true
		case *ast.CreateJavaScriptActionStmt:
			return v.Folder, true
		case *ast.CreateDatabaseConnectionStmt:
			return v.Folder, true
		case *ast.CreateDataTransformerStmt:
			return v.Folder, true
		case *ast.CreateWorkflowStmt:
			return v.Folder, true
		case *ast.CreateModelStmt:
			return v.Folder, true
		case *ast.CreateKnowledgeBaseStmt:
			return v.Folder, true
		case *ast.CreateConsumedMCPServiceStmt:
			return v.Folder, true
		case *ast.CreateAgentStmt:
			return v.Folder, true
		}
	}
	return "", false
}

// TestCreateStatementsAcceptAFolderClause covers the doctypes that had no way
// to be placed at all: #932 reported mappings, but a dozen more documents could
// only ever be created at the module root.
func TestCreateStatementsAcceptAFolderClause(t *testing.T) {
	const want = "Private/Filed"
	cases := map[string]string{
		"import mapping": `create import mapping M.IMM folder 'Private/Filed'
			with json structure M.JS { create M.E { A = a } };`,
		"export mapping": `create export mapping M.EXM folder 'Private/Filed'
			with json structure M.JS { M.E { a = A } };`,
		"queue":              `create queue M.Q folder 'Private/Filed' ( Parallelism: 3 );`,
		"regular expression": `create regular expression M.RE folder 'Private/Filed' ( Expression: '\d+' );`,
		"scheduled event":    `create scheduled event M.SE folder 'Private/Filed' ( Microflow: M.ACT_Run, Repeat: Day );`,
		"image collection":   `create image collection M.IC folder 'Private/Filed';`,
		"menu":               `create menu M.Menu folder 'Private/Filed' ( menu item 'Home' page M.Home; );`,
		"java action":        "create java action M.JA folder 'Private/Filed' () returns string as $$return null;$$;",
		"javascript action":  "create javascript action M.JSA folder 'Private/Filed' () returns string as $$return null;$$;",
		"database connection": `create database connection M.DB folder 'Private/Filed'
			type 'PostgreSQL' connection string 'jdbc:postgresql://h/db';`,
		"data transformer": `create data transformer M.DT folder 'Private/Filed'
			source json '{"a": 1}' { JSLT '{ "b": .a }'; };`,
		"workflow": `create workflow M.WF folder 'Private/Filed'
			begin
			  user task Approve 'Approve the order';
			end workflow;`,
		"model":                `create model M.Model folder 'Private/Filed' ( Provider: MxCloudGenAI );`,
		"knowledge base":       `create knowledge base M.KB folder 'Private/Filed' ( Provider: MxCloudGenAI );`,
		"consumed mcp service": `create consumed mcp service M.MCP folder 'Private/Filed' ( Version: '0.0.1' );`,
		"agent":                `create agent M.Agent folder 'Private/Filed' ( UsageType: Task );`,
	}
	for kind, src := range cases {
		t.Run(kind, func(t *testing.T) {
			got, found := folderOf(t, src)
			if !found {
				t.Fatalf("no create statement produced for %s", kind)
			}
			if got != want {
				t.Errorf("Folder = %q, want %q", got, want)
			}
		})
	}
}

// TestCreateStatementsWithoutAFolderClauseAreSilent is the control. An absent
// clause must leave Folder empty, because the executor reads emptiness as
// "leave placement alone" — a visitor that defaulted it to anything would unfile
// every foldered document on the next CREATE OR MODIFY.
func TestCreateStatementsWithoutAFolderClauseAreSilent(t *testing.T) {
	cases := map[string]string{
		"import mapping": `create import mapping M.IMM with json structure M.JS { create M.E { A = a } };`,
		"queue":          `create queue M.Q ( Parallelism: 3 );`,
		"java action":    "create java action M.JA () returns string as $$return null;$$;",
		"model":          `create model M.Model ( Provider: MxCloudGenAI );`,
	}
	for kind, src := range cases {
		t.Run(kind, func(t *testing.T) {
			got, found := folderOf(t, src)
			if !found {
				t.Fatalf("no create statement produced for %s", kind)
			}
			if got != "" {
				t.Errorf("Folder = %q, want empty for a statement with no folder clause", got)
			}
		})
	}
}

// TestWorkflowHeaderClausesAreReadByLabel pins the trap that adding a FOLDER
// clause to the workflow rule created. The header's optional strings used to be
// read by counting STRING_LITERALs in order, so a folder path — now the first
// string in the rule — would have been picked up as the display name.
func TestWorkflowHeaderClausesAreReadByLabel(t *testing.T) {
	src := `create workflow M.WF
		folder 'Private/Filed'
		display 'Approve Order'
		description 'Two-step approval'
		due date '[%DayLength%]'
		begin
		  user task Approve 'Approve the order';
		end workflow;`
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var wf *ast.CreateWorkflowStmt
	for _, s := range prog.Statements {
		if w, ok := s.(*ast.CreateWorkflowStmt); ok {
			wf = w
		}
	}
	if wf == nil {
		t.Fatal("no CreateWorkflowStmt produced")
	}
	if wf.Folder != "Private/Filed" {
		t.Errorf("Folder = %q", wf.Folder)
	}
	if wf.DisplayName != "Approve Order" {
		t.Errorf("DisplayName = %q — the folder path leaked into it", wf.DisplayName)
	}
	if wf.Description != "Two-step approval" {
		t.Errorf("Description = %q", wf.Description)
	}
	if wf.DueDate != "[%DayLength%]" {
		t.Errorf("DueDate = %q", wf.DueDate)
	}
}

// TestDataTransformerSourceIsReadByLabel is the same trap in the other rule that
// gained a second direct STRING_LITERAL.
func TestDataTransformerSourceIsReadByLabel(t *testing.T) {
	src := `create data transformer M.DT folder 'Private/Filed'
		source json '{"a": 1}' { JSLT '{ "b": .a }'; };`
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	for _, s := range prog.Statements {
		if dt, ok := s.(*ast.CreateDataTransformerStmt); ok {
			if dt.Folder != "Private/Filed" {
				t.Errorf("Folder = %q", dt.Folder)
			}
			if dt.SourceJSON != `{"a": 1}` {
				t.Errorf("SourceJSON = %q — the folder path was read as the source", dt.SourceJSON)
			}
			return
		}
	}
	t.Fatal("no CreateDataTransformerStmt produced")
}
