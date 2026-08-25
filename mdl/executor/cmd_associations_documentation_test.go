// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// An association was the one domain-model element with NO way to document it on
// create: `comment 'text'` was accepted and dropped, and a `/** … */` doc comment
// was dropped too. Measured on 11.13.0, both engines — the text appeared nowhere
// in the written project, and `mx check` passed, because an undocumented
// association is a valid association.
//
// Both halves were upstream of the writer, which has emitted `Documentation` all
// along:
//
//   - the visitor never captured the doc comment into stmt.Documentation, the way
//     the entity and enumeration visitors do; and
//   - the plain-CREATE branches of execCreateAssociation built the association
//     without Documentation at all. The OR MODIFY branches beside them set it,
//     which is why the field looked wired when read casually.
//
// This is why `create ... comment` was left on associations when it was removed
// from entity, enumeration, module, microflow, nanoflow and rule: there, the doc
// comment already worked, so the dead option was redundant. Here neither worked,
// so removing the option would have left nothing at all.

// assocCtx wires two entities in one module and captures the association the
// executor creates.
func assocCtx(t *testing.T) (*ExecContext, **domainmodel.Association) {
	t.Helper()
	mod := mkModule("As")

	parent := &domainmodel.Entity{Name: "Parent"}
	parent.ID = nextID("ent")
	child := &domainmodel.Entity{Name: "Child"}
	child.ID = nextID("ent")

	dm := &domainmodel.DomainModel{ContainerID: mod.ID, Entities: []*domainmodel.Entity{parent, child}}
	dm.ID = nextID("dm")

	var created *domainmodel.Association
	h := mkHierarchy(mod)
	withContainer(h, dm.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		CreateAssociationFunc: func(_ model.ID, a *domainmodel.Association) error {
			created = a
			return nil
		},
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &created
}

func createAssocStmt() *ast.CreateAssociationStmt {
	return &ast.CreateAssociationStmt{
		Name:   ast.QualifiedName{Module: "As", Name: "Child_Parent"},
		Parent: ast.QualifiedName{Module: "As", Name: "Child"},
		Child:  ast.QualifiedName{Module: "As", Name: "Parent"},
	}
}

// The `/** … */` doc comment, which is the spelling every other document type
// uses and the one the skills tell people to write.
func TestCreateAssociationStoresTheDocComment(t *testing.T) {
	ctx, created := assocCtx(t)

	s := createAssocStmt()
	s.Documentation = "Links a child to its parent."
	assertNoError(t, execCreateAssociation(ctx, s))

	got := *created
	if got == nil {
		t.Fatal("CreateAssociation was never called")
	}
	if got.Documentation != s.Documentation {
		t.Errorf("Documentation = %q, want %q — the doc comment was dropped on create",
			got.Documentation, s.Documentation)
	}
}

// `comment 'text'` still works here. It is the only inline spelling an
// association has, which is why it survived the removal that took it off every
// other CREATE.
func TestCreateAssociationStoresTheCommentOption(t *testing.T) {
	ctx, created := assocCtx(t)

	s := createAssocStmt()
	s.Comment = "Links a child to its parent."
	assertNoError(t, execCreateAssociation(ctx, s))

	got := *created
	if got == nil {
		t.Fatal("CreateAssociation was never called")
	}
	if got.Documentation != s.Comment {
		t.Errorf("Documentation = %q, want %q — the comment option was dropped on create",
			got.Documentation, s.Comment)
	}
}

// When a statement carries both, the doc comment wins — the same precedence the
// entity path uses ("Documentation if available, fall back to Comment").
func TestCreateAssociationPrefersTheDocCommentOverTheOption(t *testing.T) {
	ctx, created := assocCtx(t)

	s := createAssocStmt()
	s.Documentation = "From the doc comment."
	s.Comment = "From the option."
	assertNoError(t, execCreateAssociation(ctx, s))

	if got := (*created).Documentation; got != "From the doc comment." {
		t.Errorf("Documentation = %q, want the doc comment to win", got)
	}
}

// The control: an association with neither is created with empty documentation,
// not with something invented. Without this, "documentation is stored" could be
// satisfied by writing a placeholder.
func TestCreateAssociationWithoutDocumentationStoresNone(t *testing.T) {
	ctx, created := assocCtx(t)

	assertNoError(t, execCreateAssociation(ctx, createAssocStmt()))

	if got := (*created).Documentation; got != "" {
		t.Errorf("Documentation = %q, want empty", got)
	}
}
