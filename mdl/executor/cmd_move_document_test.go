// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// moveMockBackend wires a project with one document of the given type, found by
// name through the type-agnostic lookup.
func moveMockBackend(t *testing.T, mod *model.Module, doc *types.DocumentUnit, probe *placementProbe) (*mock.MockBackend, *ContainerHierarchy) {
	t.Helper()
	mb, h := folderMockBackend(t, mod, probe)
	mb.FindDocumentUnitFunc = func(moduleName, name string) (*types.DocumentUnit, error) {
		if doc != nil && moduleName == mod.Name && name == doc.Name {
			return doc, nil
		}
		return nil, nil
	}
	return mb, h
}

// TestMoveImportMappingReachesStorage is the issue's second report: MOVE IMPORT
// MAPPING was a parse error, and the backend method that would have performed it
// existed on the interface, both engines and the mock — with nothing calling it.
func TestMoveImportMappingReachesStorage(t *testing.T) {
	mod := mkModule("Module")
	doc := &types.DocumentUnit{
		ID:          nextID("unit"),
		ContainerID: mod.ID,
		Name:        "IMM_Example",
		Type:        "ImportMappings$ImportMapping",
		Kind:        "import mapping",
	}
	var probe placementProbe
	mb, h := moveMockBackend(t, mod, doc, &probe)

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeImportMapping,
		Name:         ast.QualifiedName{Module: "Module", Name: "IMM_Example"},
		Folder:       "Private/Import mappings",
	})
	assertNoError(t, err)

	if !probe.moved {
		t.Fatal("the mapping was not reparented")
	}
	if probe.unitID != doc.ID {
		t.Errorf("reparented %q, want %q", probe.unitID, doc.ID)
	}
	if probe.containerID == mod.ID {
		t.Error("reparented to the module root rather than the resolved folder")
	}
	assertContainsStr(t, buf.String(), "Moved import mapping Module.IMM_Example")
}

// TestMoveReportsTheKindItFound pins that the output names what actually moved
// rather than echoing the statement. The lookup is by name, so the two can
// differ — and a line that repeats the user's word for it would be a
// confident-sounding lie.
func TestMoveReportsTheKindItFound(t *testing.T) {
	mod := mkModule("Module")
	// A kind mxcli has no MOVE spelling for: the statement is accepted (the
	// name resolved to exactly one document) but must be reported truthfully.
	doc := &types.DocumentUnit{
		ID:          nextID("unit"),
		ContainerID: mod.ID,
		Name:        "Thing",
		Type:        "Forms$PageTemplate",
		Kind:        "page template",
	}
	var probe placementProbe
	mb, h := moveMockBackend(t, mod, doc, &probe)

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeJsonStructure,
		Name:         ast.QualifiedName{Module: "Module", Name: "Thing"},
		Folder:       "Private",
	})
	assertNoError(t, err)
	assertContainsStr(t, buf.String(), "Moved page template Module.Thing")
	assertNotContainsStr(t, buf.String(), "json structure")
}

// TestMoveRefusesAMistypedDoctype is the other half of that: when the derived
// kind IS a doctype MOVE can spell, the mismatch is a typo worth refusing, and
// the error must name the right statement to run instead.
func TestMoveRefusesAMistypedDoctype(t *testing.T) {
	mod := mkModule("Module")
	doc := &types.DocumentUnit{
		ID:          nextID("unit"),
		ContainerID: mod.ID,
		Name:        "JSON_Example",
		Type:        "JsonStructures$JsonStructure",
		Kind:        "json structure",
	}
	var probe placementProbe
	mb, h := moveMockBackend(t, mod, doc, &probe)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeQueue,
		Name:         ast.QualifiedName{Module: "Module", Name: "JSON_Example"},
		Folder:       "Private",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "is a json structure, not a queue")
	if probe.moved {
		t.Error("a refused statement still reparented the document")
	}
}

// TestMoveUnknownDocumentIsNotFound pins that an absent name is reported as
// absent. The lookup returns nil for "no such document", and treating that as
// anything other than not-found would make MOVE silently succeed.
func TestMoveUnknownDocumentIsNotFound(t *testing.T) {
	mod := mkModule("Module")
	var probe placementProbe
	mb, h := moveMockBackend(t, mod, nil, &probe)

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	err := execMove(ctx, &ast.MoveStmt{
		DocumentType: ast.DocumentTypeJsonStructure,
		Name:         ast.QualifiedName{Module: "Module", Name: "Nope"},
		Folder:       "Private",
	})
	assertError(t, err)
	assertContainsStr(t, err.Error(), "not found")
	if probe.moved {
		t.Error("a document that was not found was still reparented")
	}
}

// TestMoveEveryRegisteredDoctypeHasAPath walks the whole registry through
// execMove, so a doctype that parses but has no working handler fails here
// rather than in a user's project.
//
// Entity is excluded: it is not a unit and has its own handler, which needs a
// domain model rather than a document.
func TestMoveEveryRegisteredDoctypeHasAPath(t *testing.T) {
	for _, docType := range ast.MoveDocumentTypeByKeyword {
		t.Run(string(docType), func(t *testing.T) {
			mod := mkModule("Module")
			// Give the document the kind the statement names, so this test is
			// about routing rather than about the mistyped-doctype check.
			doc := &types.DocumentUnit{
				ID:          nextID("unit"),
				ContainerID: mod.ID,
				Name:        "Thing",
				Type:        "Some$Type",
				Kind:        strings.ToLower(string(docType)),
			}
			var probe placementProbe
			mb, h := moveMockBackend(t, mod, doc, &probe)
			// The typed handlers reach for their own list calls; give each an
			// empty project so they report "not found" rather than panicking,
			// and assert only that no doctype produces "unsupported".
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
			err := execMove(ctx, &ast.MoveStmt{
				DocumentType: docType,
				Name:         ast.QualifiedName{Module: "Module", Name: "Thing"},
				Folder:       "Private",
			})
			if err != nil && strings.Contains(err.Error(), "unsupported document type") {
				t.Errorf("%s parses but has no handler: %v", docType, err)
			}
		})
	}
}
