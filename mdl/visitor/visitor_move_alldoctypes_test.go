// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// parseMove parses one MOVE statement and returns it.
func parseMove(t *testing.T, src string) *ast.MoveStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors for %q: %v", src, errs)
	}
	for _, s := range prog.Statements {
		if m, ok := s.(*ast.MoveStmt); ok {
			return m
		}
	}
	t.Fatalf("no MoveStmt produced for %q", src)
	return nil
}

// TestMoveStatement_AcceptsEveryDocumentType is #932's second half: MOVE listed
// nine doctypes, so import mappings, JSON structures and twenty others could not
// be foldered from MDL at all ("no viable alternative at input 'MOVEIMPORT'").
//
// Driven from ast.MoveDocumentTypeByKeyword rather than a list written out here,
// so a doctype added to the grammar and the registry is covered without anyone
// remembering to extend this test — and a doctype added to the registry but not
// the grammar fails it.
func TestMoveStatement_AcceptsEveryDocumentType(t *testing.T) {
	for keyword, want := range ast.MoveDocumentTypeByKeyword {
		t.Run(keyword, func(t *testing.T) {
			// The MDL spelling is the DocumentType value, which is the keyword
			// with its word breaks restored.
			src := "move " + strings.ToLower(string(want)) + " Mv.Thing to folder 'Private';"
			got := parseMove(t, src)
			if got.DocumentType != want {
				t.Errorf("%q gave DocumentType %q, want %q", src, got.DocumentType, want)
			}
			if got.Name.Module != "Mv" || got.Name.Name != "Thing" {
				t.Errorf("%q gave name %q", src, got.Name.String())
			}
			if got.Folder != "Private" {
				t.Errorf("%q gave folder %q, want Private", src, got.Folder)
			}
		})
	}
}

// TestMoveStatement_ReportedStatementsParse is the issue's own text, verbatim.
func TestMoveStatement_ReportedStatementsParse(t *testing.T) {
	got := parseMove(t, "MOVE IMPORT MAPPING Module.IMM_Example TO FOLDER 'Private/Import mappings';")
	if got.DocumentType != ast.DocumentTypeImportMapping {
		t.Errorf("DocumentType = %q, want IMPORT MAPPING", got.DocumentType)
	}
	if got.Folder != "Private/Import mappings" {
		t.Errorf("Folder = %q", got.Folder)
	}
}

// TestMoveStatement_ToModuleWithoutFolder pins the second alternative for the
// new doctypes: moving to a module root rather than into a folder.
func TestMoveStatement_ToModuleWithoutFolder(t *testing.T) {
	got := parseMove(t, "move json structure Mv.JSON_Order to OtherModule;")
	if got.DocumentType != ast.DocumentTypeJsonStructure {
		t.Errorf("DocumentType = %q, want JSON STRUCTURE", got.DocumentType)
	}
	if got.Folder != "" {
		t.Errorf("Folder = %q, want empty for a move to a module root", got.Folder)
	}
	if got.TargetModule != "OtherModule" {
		t.Errorf("TargetModule = %q, want OtherModule", got.TargetModule)
	}
}

// TestMoveStatement_FolderMoveSurvivesEveryDoctype is the regression this
// restructure exists to prevent, and the reason moveDocumentType is a grammar
// rule instead of an inline alternation.
//
// MOVE FOLDER is identified by the ABSENCE of a doctype. That used to be a
// hand-written negation of every doctype keyword, so each keyword added to MOVE
// had to be added to the discriminator too — and with twenty-two more keywords
// the odds of that staying in step were nil. The discriminator is now one check
// against the sub-rule, which cannot go stale; this test is the proof.
func TestMoveStatement_FolderMoveSurvivesEveryDoctype(t *testing.T) {
	for _, src := range []string{
		"move folder Mv.Old to folder 'New';",
		"move folder Mv.Old to OtherModule;",
		"move folder Mv.Old to folder 'New' in OtherModule;",
	} {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse errors for %q: %v", src, errs)
		}
		sawFolderMove := false
		for _, s := range prog.Statements {
			if m, ok := s.(*ast.MoveStmt); ok {
				t.Fatalf("%q produced a document MoveStmt (type %q)", src, m.DocumentType)
			}
			if _, ok := s.(*ast.MoveFolderStmt); ok {
				sawFolderMove = true
			}
		}
		if !sawFolderMove {
			t.Errorf("%q produced no MoveFolderStmt", src)
		}
	}
}

// TestMoveStatement_EntityIsStillItsOwnThing guards the one doctype that is not
// a unit: an entity lives inside a domain model, and its move converts
// associations rather than reparenting a row, so it must not be swept into the
// generic document path.
func TestMoveStatement_EntityIsStillItsOwnThing(t *testing.T) {
	got := parseMove(t, "move entity Mv.Customer to OtherModule;")
	if got.DocumentType != ast.DocumentTypeEntity {
		t.Errorf("DocumentType = %q, want ENTITY", got.DocumentType)
	}
	if got.TargetModule != "OtherModule" {
		t.Errorf("TargetModule = %q", got.TargetModule)
	}
}
