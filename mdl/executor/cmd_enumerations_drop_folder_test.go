// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// upstream #976: DROP ENUMERATION could not resolve an enumeration that visibly
// exists.
//
//	DROP ENUMERATION NCRManagement.ENUM_DisciplineMethod;
//	Error: enumeration not found: NCRManagement.ENUM_DisciplineMethod
//
// while SHOW ENUMERATIONS listed it and `mxcli refs` resolved it in the same
// session. The difference is a FOLDER: an enumeration inside one has the
// folder's ID as its ContainerID, and DROP resolved the owning module with
// findModuleByID, which matches module IDs only. SHOW, DESCRIBE and ALTER all
// use the container hierarchy, which walks folders up to the module — so DROP
// was the one command of the four that could not see inside one.
//
// The control is in the fixture: two identical enumerations, one at the module
// root and one in a folder. Before the fix only the foldered one failed, which
// is what makes this a container-resolution bug rather than "drop is broken".

// dropFolderFixture builds a module with `Loose` at the root and `Filed` inside
// a folder, and reports which enumeration ID was deleted.
func dropFolderFixture(t *testing.T) (*ExecContext, *model.Enumeration, *model.Enumeration, *string) {
	t.Helper()
	mod := mkModule("NCR")
	folderID := model.ID("folder-enums")

	loose := mkEnumeration(mod.ID, "Loose", "A", "B")
	filed := mkEnumeration(folderID, "Filed", "A", "B")

	h := mkHierarchy(mod)
	withContainer(h, loose.ContainerID, mod.ID)
	// `filed.ContainerID` IS the folder, so the only link to register is the
	// folder's own parent. Registering the enumeration's container as a child of
	// the folder would write folder → folder and hide the module behind a cycle.
	withContainer(h, folderID, mod.ID)

	deleted := ""
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) {
			return []*model.Enumeration{loose, filed}, nil
		},
		DeleteEnumerationFunc: func(id model.ID) error {
			deleted = string(id)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, loose, filed, &deleted
}

func TestDropEnumeration_InsideAFolder(t *testing.T) {
	ctx, _, filed, deleted := dropFolderFixture(t)

	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "NCR", Name: "Filed"},
	})
	if err != nil {
		t.Fatalf("an enumeration in a folder must be droppable; SHOW and ALTER both find it. got: %v", err)
	}
	if *deleted != string(filed.ID) {
		t.Errorf("deleted %q, want the foldered enumeration %q", *deleted, filed.ID)
	}
}

// The control. This one passed before the fix too — it is here so the test above
// cannot be satisfied by a change that drops the module check altogether.
func TestDropEnumeration_AtTheModuleRoot(t *testing.T) {
	ctx, loose, _, deleted := dropFolderFixture(t)

	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "NCR", Name: "Loose"},
	})
	if err != nil {
		t.Fatalf("dropping an enumeration at the module root must keep working, got: %v", err)
	}
	if *deleted != string(loose.ID) {
		t.Errorf("deleted %q, want %q", *deleted, loose.ID)
	}
}

// Resolving through the hierarchy must not make the module filter meaningless:
// a foldered enumeration still belongs to its module, and naming another one
// must not find it.
func TestDropEnumeration_FolderedButWrongModule(t *testing.T) {
	ctx, _, _, deleted := dropFolderFixture(t)

	err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "SomeOtherModule", Name: "Filed"},
	})
	if err == nil {
		t.Fatal("NCR.Filed must not answer to SomeOtherModule.Filed")
	}
	if *deleted != "" {
		t.Errorf("nothing should have been deleted, but %q was", *deleted)
	}
}

// An enumeration whose container cannot be walked to a module is skipped rather
// than reported under an empty module name.
func TestDropEnumeration_UnresolvableContainer(t *testing.T) {
	mod := mkModule("NCR")
	orphan := mkEnumeration(model.ID("nowhere"), "Orphan", "A")

	deleted := ""
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListEnumerationsFunc: func() ([]*model.Enumeration, error) { return []*model.Enumeration{orphan}, nil },
		DeleteEnumerationFunc: func(id model.ID) error {
			deleted = string(id)
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	if err := execDropEnumeration(ctx, &ast.DropEnumerationStmt{
		Name: ast.QualifiedName{Module: "NCR", Name: "Orphan"},
	}); err == nil {
		t.Fatal("an enumeration whose module cannot be resolved must not be dropped under a guessed name")
	}
	if deleted != "" {
		t.Errorf("nothing should have been deleted, but %q was", deleted)
	}
}
