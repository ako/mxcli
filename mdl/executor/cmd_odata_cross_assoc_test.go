// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// crossAssocODataCtx builds the mendixlabs/mxcli#1335 layout: Op.Parent, and
// Od.Child with a cross-module Reference Od.Child_Parent (FROM Od.Child TO
// Op.Parent), stored in Od's domain model as a DomainModels$CrossAssociation.
// Od also holds Od.Note with a same-module Od.Note_Child as the control.
func crossAssocODataCtx(t *testing.T) *ExecContext {
	t.Helper()
	op, od := mkModule("Op"), mkModule("Od")
	parent := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Parent", Persistable: true}
	child := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Child", Persistable: true}
	note := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Note", Persistable: true}
	opDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: op.ID,
		Entities:    []*domainmodel.Entity{parent},
	}
	odDM := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: od.ID,
		Entities:    []*domainmodel.Entity{child, note},
		Associations: []*domainmodel.Association{{
			Name: "Note_Child", ParentID: note.ID, ChildID: child.ID, Type: domainmodel.AssociationTypeReference,
		}},
		CrossAssociations: []*domainmodel.CrossModuleAssociation{{
			Name: "Child_Parent", ParentID: child.ID, ChildRef: "Op.Parent", Type: domainmodel.AssociationTypeReference,
		}},
	}
	byModule := map[model.ID]*domainmodel.DomainModel{op.ID: opDM, od.ID: odDM}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{op, od}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{opDM, odDM}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return byModule[id], nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func publishOne(t *testing.T, ctx *ExecContext, entity ast.QualifiedName, member string) *model.PublishedMember {
	t.Helper()
	def := &ast.PublishedEntityDef{
		Entity:  entity,
		Members: []*ast.PublishedMemberDef{{Name: member, ExposedName: "nav"}},
	}
	et, _ := astEntityDefToModel(ctx, def)
	if len(et.Members) != 1 {
		t.Fatalf("got %d members, want 1", len(et.Members))
	}
	return et.Members[0]
}

// TestPublishCrossModuleAssociation_IsAssociationEnd is mendixlabs/mxcli#1335:
// exposing a cross-module association by bare name wrote a PublishedAttribute
// with Attribute 'Od.Child.Child_Parent', which mx check reports as CE1613
// "The selected attribute 'Od.Child.Child_Parent' no longer exists."
func TestPublishCrossModuleAssociation_IsAssociationEnd(t *testing.T) {
	ctx := crossAssocODataCtx(t)

	cases := []struct {
		name       string
		entity     ast.QualifiedName
		member     string
		wantAssoc  string
		wantTarget string
		wantMany   bool
	}{
		// Control: a same-module association already worked before the fix.
		{"same-module FROM side", ast.QualifiedName{Module: "Od", Name: "Note"}, "Note_Child", "Od.Note_Child", "Od.Child", false},
		{"cross-module FROM side", ast.QualifiedName{Module: "Od", Name: "Child"}, "Child_Parent", "Od.Child_Parent", "Op.Parent", false},
		// The TO entity's module does not hold the association; the writer must
		// still qualify it with Od, the association's module, not Op.
		{"cross-module TO side", ast.QualifiedName{Module: "Op", Name: "Parent"}, "Child_Parent", "Od.Child_Parent", "Od.Child", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := publishOne(t, ctx, tc.entity, tc.member)
			if m.Kind != "association" {
				t.Fatalf("Kind = %q, want association (written as a PublishedAttribute -> CE1613)", m.Kind)
			}
			// The writer prefixes a bare name with the published entity's module
			// (qualifyAssociationName); compare what lands in Association.
			written := m.Name
			if !strings.Contains(written, ".") {
				written = tc.entity.Module + "." + written
			}
			if written != tc.wantAssoc {
				t.Errorf("Association as written = %q, want %q", written, tc.wantAssoc)
			}
			if m.AssociationTargetEntity != tc.wantTarget {
				t.Errorf("Entity = %q, want %q", m.AssociationTargetEntity, tc.wantTarget)
			}
			if m.IsMany != tc.wantMany {
				t.Errorf("IsMany = %v, want %v", m.IsMany, tc.wantMany)
			}
			if m.ExposedAssociationName != tc.member {
				t.Errorf("ExposedAssociationName = %q, want %q", m.ExposedAssociationName, tc.member)
			}
		})
	}
}
