// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// upstream #901, the half of it the visitor fix does not reach.
//
// Mendix's DeletingBehavior admits exactly three values (generated/metamodel
// enums.go). The CREATE path converts through an explicit switch and lands on
// them; ALTER ASSOCIATION SET built the stored value as
// DeleteBehaviorType(s.DeleteBehavior.String()), and ast.DeleteBehavior.String()
// returns "DeleteIfNoReferences" where Mendix writes "DeleteMeIfNoReferences".
//
// That mattered only once the visitor could produce DeleteIfNoReferences at all,
// which is why it hid behind the reported bug: measured by patching
// buildDeleteBehavior alone and running `ALTER ASSOCIATION ... SET DELETE_BEHAVIOR
// PREVENT` against a real 11.6.6 project, the string "DeleteIfNoReferences"
// reached the .mxunit on disk. An out-of-domain enum is worse than the wrong
// behaviour it replaces — mxbuild tolerates unknown property VALUES about as far
// as Studio Pro does, which is not far (CLAUDE.md, MprProperty.cs).
//
// So the assertion is on the STORED value, not on the AST and not on the command
// output. "Altered association: ..." was printed in every broken case.
func TestAssociationDeleteBehaviorReachesStorageAsAMendixValue(t *testing.T) {
	cases := []struct {
		name string
		in   ast.DeleteBehavior
		want domainmodel.DeleteBehaviorType
	}{
		{"keep", ast.DeleteKeepReferences, domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences},
		{"cascade", ast.DeleteCascade, domainmodel.DeleteBehaviorTypeDeleteMeAndReferences},
		{"prevent", ast.DeleteIfNoReferences, domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences},
	}

	for _, tc := range cases {
		t.Run("create-or-modify/"+tc.name, func(t *testing.T) {
			ctx, assoc := assocFixture(t)
			assertNoError(t, execCreateAssociation(ctx, &ast.CreateAssociationStmt{
				Name:           ast.QualifiedName{Module: "M", Name: "Child_Parent"},
				Parent:         ast.QualifiedName{Module: "M", Name: "Child"},
				Child:          ast.QualifiedName{Module: "M", Name: "Parent"},
				Type:           ast.AssocReference,
				DeleteBehavior: tc.in,
				CreateOrModify: true,
			}))
			assertStoredBehavior(t, assoc, tc.want)
		})

		t.Run("alter-set/"+tc.name, func(t *testing.T) {
			ctx, assoc := assocFixture(t)
			assertNoError(t, execAlterAssociation(ctx, &ast.AlterAssociationStmt{
				Name:           ast.QualifiedName{Module: "M", Name: "Child_Parent"},
				Operation:      ast.AlterAssociationSetDeleteBehavior,
				DeleteBehavior: tc.in,
			}))
			assertStoredBehavior(t, assoc, tc.want)
		})
	}
}

// DESCRIBE emitted `delete_behavior DELETE_CASCADE;` for a cascading association,
// and DELETE_CASCADE is not a token — the parser rejects it with "missing
// {DELETE_AND_REFERENCES, DELETE_BUT_KEEP_REFERENCES, DELETE_IF_NO_REFERENCES,
// CASCADE, PREVENT}". It is the very line the #901 reporter pasted as their
// starting state, so a user following the obvious describe → edit → exec loop hit
// a parse error on cascade and silent data loss on everything else.
//
// Feeding DESCRIBE's own output back through the parser is the only check that
// proves the two sides agree; asserting on a string literal would pass against a
// formatter emitting something nothing can read.
func TestDescribeAssociationDeleteBehaviorRoundTripsThroughTheParser(t *testing.T) {
	for _, want := range []ast.DeleteBehavior{
		ast.DeleteKeepReferences, ast.DeleteCascade, ast.DeleteIfNoReferences,
	} {
		t.Run(want.String(), func(t *testing.T) {
			ctx, assoc := assocFixture(t)
			assertNoError(t, execCreateAssociation(ctx, &ast.CreateAssociationStmt{
				Name:           ast.QualifiedName{Module: "M", Name: "Child_Parent"},
				Parent:         ast.QualifiedName{Module: "M", Name: "Child"},
				Child:          ast.QualifiedName{Module: "M", Name: "Parent"},
				Type:           ast.AssocReference,
				DeleteBehavior: want,
				CreateOrModify: true,
			}))
			_ = assoc

			var buf bytes.Buffer
			ctx.Output = &buf
			assertNoError(t, describeAssociation(ctx, ast.QualifiedName{Module: "M", Name: "Child_Parent"}))

			prog, errs := visitor.Build(buf.String())
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE emitted MDL the parser rejects: %v\n--- output ---\n%s", errs, buf.String())
			}
			stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
			if !ok {
				t.Fatalf("got %T, want *ast.CreateAssociationStmt", prog.Statements[0])
			}
			if stmt.DeleteBehavior != want {
				t.Errorf("round-tripped delete behaviour = %v, want %v\n--- output ---\n%s",
					stmt.DeleteBehavior, want, buf.String())
			}
		})
	}
}

// assocFixture builds M.Child_Parent stored as DELETE_CASCADE — the reporter's
// starting state — and returns the association the executor will mutate in place.
// Starting from cascade rather than the default is what makes a silent fallback
// to DeleteMeButKeepReferences visible instead of indistinguishable from success.
func assocFixture(t *testing.T) (*ExecContext, *domainmodel.Association) {
	t.Helper()
	mod := mkModule("M")
	child := mkEntity(mod.ID, "Child")
	parent := mkEntity(mod.ID, "Parent")
	assoc := mkAssociation(mod.ID, "Child_Parent", child.ID, parent.ID)
	assoc.ChildDeleteBehavior = &domainmodel.DeleteBehavior{
		Type: domainmodel.DeleteBehaviorTypeDeleteMeAndReferences,
	}

	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{child, parent},
		Associations: []*domainmodel.Association{assoc},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, child.ContainerID, dm.ID)
	withContainer(h, parent.ContainerID, dm.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:             func() bool { return true },
		ListModulesFunc:             func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc:        func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:          func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateDomainModelFunc:       func(*domainmodel.DomainModel) error { return nil },
		ReconcileMemberAccessesFunc: func(model.ID, string) (int, error) { return 0, nil },
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, assoc
}

func assertStoredBehavior(t *testing.T, assoc *domainmodel.Association, want domainmodel.DeleteBehaviorType) {
	t.Helper()
	if assoc.ChildDeleteBehavior == nil {
		t.Fatalf("no delete behaviour stored, want %q", want)
	}
	got := assoc.ChildDeleteBehavior.Type
	switch got {
	case domainmodel.DeleteBehaviorTypeDeleteMeAndReferences,
		domainmodel.DeleteBehaviorTypeDeleteMeButKeepReferences,
		domainmodel.DeleteBehaviorTypeDeleteMeIfNoReferences:
	default:
		t.Fatalf("stored %q, which is not one of Mendix's three DeletingBehavior values — "+
			"this is an unopenable model, not merely a wrong one", got)
	}
	if got != want {
		t.Errorf("stored %q, want %q", got, want)
	}
}

// The message must survive DESCRIBE, or a describe -> exec round trip writes an
// association whose RUNTIME does not start — and regenerating scripts that way
// is exactly how these models are maintained (CapTrackV2 §1).
func TestDescribeAssociation_RoundTripsTheDeleteErrorMessage(t *testing.T) {
	const msg = "A customer with orders cannot be deleted"
	ctx, _ := assocFixture(t)
	assertNoError(t, execCreateAssociation(ctx, &ast.CreateAssociationStmt{
		Name:               ast.QualifiedName{Module: "M", Name: "Child_Parent"},
		Parent:             ast.QualifiedName{Module: "M", Name: "Child"},
		Child:              ast.QualifiedName{Module: "M", Name: "Parent"},
		Type:               ast.AssocReference,
		DeleteBehavior:     ast.DeleteIfNoReferences,
		DeleteErrorMessage: msg,
		CreateOrModify:     true,
	}))

	var buf bytes.Buffer
	ctx.Output = &buf
	assertNoError(t, describeAssociation(ctx, ast.QualifiedName{Module: "M", Name: "Child_Parent"}))

	prog, errs := visitor.Build(buf.String())
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE emitted MDL the parser rejects: %v\n--- output ---\n%s", errs, buf.String())
	}
	stmt := prog.Statements[0].(*ast.CreateAssociationStmt)
	if stmt.DeleteBehavior != ast.DeleteIfNoReferences {
		t.Errorf("behaviour = %v, want DeleteIfNoReferences", stmt.DeleteBehavior)
	}
	if stmt.DeleteErrorMessage != msg {
		t.Errorf("message = %q, want %q\n--- output ---\n%s", stmt.DeleteErrorMessage, msg, buf.String())
	}
}

// CONTROL: a behaviour with no message emits no ERROR_MESSAGE clause. An empty
// one would re-execute into a message that is not what the author wrote.
func TestDescribeAssociation_NoMessageEmitsNoClause(t *testing.T) {
	ctx, _ := assocFixture(t)
	assertNoError(t, execCreateAssociation(ctx, &ast.CreateAssociationStmt{
		Name:           ast.QualifiedName{Module: "M", Name: "Child_Parent"},
		Parent:         ast.QualifiedName{Module: "M", Name: "Child"},
		Child:          ast.QualifiedName{Module: "M", Name: "Parent"},
		Type:           ast.AssocReference,
		DeleteBehavior: ast.DeleteCascade,
		CreateOrModify: true,
	}))

	var buf bytes.Buffer
	ctx.Output = &buf
	assertNoError(t, describeAssociation(ctx, ast.QualifiedName{Module: "M", Name: "Child_Parent"}))
	if strings.Contains(strings.ToLower(buf.String()), "error_message") {
		t.Errorf("emitted an ERROR_MESSAGE clause for a cascade:\n%s", buf.String())
	}
}
