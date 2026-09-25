// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestIssue605_MoveEntitySweepsQualifiedNames guards the call site, because the
// call site is what was missing.
//
// execMove returns early for ENTITY — before the doctype switch and before the
// isCrossModuleMove sweep every other doctype gets — so a cross-module entity move
// rewrote no references at all: 33 CE1613s in a blank 11.13 app, reported as
// success. The backend contract (which association changed module, and to what) is
// covered in mdl/backend/modelsdk/issue605_move_refs_test.go; what this asserts is
// that moveEntity actually drives the sweep from it.
//
// A mock is the right instrument here: the real sweep is already exercised by the
// doctypes that never lost it, and what needs pinning is which (old, new) pairs
// reach it — in particular that an association whose module did NOT change is not
// swept, since renaming it would corrupt references that are still correct.
func TestIssue605_MoveEntitySweepsQualifiedNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		// converted is what the backend reports for the move.
		converted []types.MovedAssociation
		// wantSweeps lists the (old, new) pairs the sweep must be called with, in
		// any order.
		wantSweeps map[string]string
	}{
		{
			name: "ParentMoved_EntityAndAssociationSwept",
			converted: []types.MovedAssociation{{
				Name:             "Child_Parent",
				OldQualifiedName: "Source.Child_Parent",
				NewQualifiedName: "Target.Child_Parent",
			}},
			wantSweeps: map[string]string{
				"Source.Subject":      "Target.Subject",
				"Source.Child_Parent": "Target.Child_Parent",
			},
		},
		{
			// The cross-association stayed in the source module, so its qualified
			// name is unchanged and must NOT be swept.
			name: "ChildMoved_OnlyEntitySwept",
			converted: []types.MovedAssociation{{
				Name:             "Child_Parent",
				OldQualifiedName: "Source.Child_Parent",
				NewQualifiedName: "Source.Child_Parent",
			}},
			wantSweeps: map[string]string{
				"Source.Subject": "Target.Subject",
			},
		},
		{
			name:       "NoAssociations_EntityStillSwept",
			converted:  nil,
			wantSweeps: map[string]string{"Source.Subject": "Target.Subject"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := mkModule("Source")
			dst := mkModule("Target")
			subject := &domainmodel.Entity{Name: "Subject", Persistable: true}
			subject.ID = nextID("entity")

			gotSweeps := map[string]string{}
			mb := &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{src, dst}, nil },
				GetModuleByNameFunc: func(name string) (*model.Module, error) {
					switch name {
					case src.Name:
						return src, nil
					case dst.Name:
						return dst, nil
					}
					return nil, nil
				},
				GetDomainModelFunc: func(moduleID model.ID) (*domainmodel.DomainModel, error) {
					dm := &domainmodel.DomainModel{ContainerID: moduleID}
					dm.ID = nextID("dm")
					if moduleID == src.ID {
						dm.Entities = []*domainmodel.Entity{subject}
					}
					return dm, nil
				},
				MoveEntityFunc: func(_ *domainmodel.Entity, _, _ model.ID, _, _ string) ([]types.MovedAssociation, error) {
					return tc.converted, nil
				},
				UpdateOqlQueriesForMovedEntityFunc: func(_, _ string) (int, error) { return 0, nil },
				UpdateQualifiedNameInAllUnitsFunc: func(oldName, newName string) (int, error) {
					if prev, dup := gotSweeps[oldName]; dup {
						t.Errorf("swept %s twice (%s then %s)", oldName, prev, newName)
					}
					gotSweeps[oldName] = newName
					return 1, nil
				},
			}

			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(src, dst)))
			err := execMove(ctx, &ast.MoveStmt{
				DocumentType: ast.DocumentTypeEntity,
				Name:         ast.QualifiedName{Module: src.Name, Name: subject.Name},
				TargetModule: dst.Name,
			})
			if err != nil {
				t.Fatalf("execMove: %v", err)
			}

			for old, want := range tc.wantSweeps {
				got, ok := gotSweeps[old]
				if !ok {
					t.Errorf("no reference sweep for %s → %s", old, want)
					continue
				}
				if got != want {
					t.Errorf("sweep for %s went to %s, want %s", old, got, want)
				}
			}
			for old := range gotSweeps {
				if _, want := tc.wantSweeps[old]; !want {
					t.Errorf("unexpected reference sweep for %s → %s; renaming a name that did not change corrupts references that were still correct", old, gotSweeps[old])
				}
			}
		})
	}
}
