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

// TestRenameEntity_RepointsOwnMemberReferences guards the half of a RENAME ENTITY
// that lives INSIDE the renamed entity: its own access rules and validation rules
// name each member by qualified name (`Module.Entity.Attr`), and that name contains
// the entity name being changed.
//
// The mechanism is a clobber, and the reference report is what makes it visible.
// execRenameEntity reads the domain model, runs the project-wide RenameReferences
// sweep — which DOES rewrite those names, in the raw unit — and then persists the
// semantic model it read BEFORE the sweep, overwriting the unit with the stale
// names. Measured on a real 11.13 app: "Updated 3 reference(s) in 1 document(s)"
// followed by exactly three CE1613s naming those three members, at "Validation rule
// of entity" and "Access rule of entity" for the entity that had just been renamed.
//
// It is the sibling of the MOVE ENTITY defect in ako/mxcli#605 — same stale
// qualified names inside the moved/renamed entity, different command — and worse
// than a dangling string for the same reason: entityToGen's syncMemberAccesses
// matches existing entries BY qualified name, so a stale `Module.Old.Attr` never
// equals the rebuilt `Module.New.Attr` and it appends the new entry while keeping
// the old one.
//
// A mock is the right instrument: the defect is the executor handing
// UpdateDomainModel a semantic model whose self-references are stale, so what needs
// pinning is the model it passes. The end-to-end proof that mxbuild then accepts the
// project is mdl-examples/bug-tests/domainmodel-1169-whole-unit-storage-guids.mdl.
func TestRenameEntity_RepointsOwnMemberReferences(t *testing.T) {
	const (
		modName = "Ren"
		oldName = "Widget"
		newName = "Gadget"
	)

	mod := mkModule(modName)
	attr := &domainmodel.Attribute{Name: "WidgetName", Type: &domainmodel.StringAttributeType{Length: 100}}
	attr.ID = nextID("attr")

	ent := &domainmodel.Entity{Name: oldName, Persistable: true, Attributes: []*domainmodel.Attribute{attr}}
	ent.ID = nextID("entity")
	// An access rule naming the attribute, and — the case a fix that only covered
	// access rules would miss — a validation rule naming it too.
	ma := &domainmodel.MemberAccess{
		AttributeID:   attr.ID,
		AttributeName: modName + "." + oldName + "." + attr.Name,
		AccessRights:  domainmodel.MemberAccessRightsReadWrite,
	}
	ma.ID = nextID("ma")
	ar := &domainmodel.AccessRule{ContainerID: ent.ID, AllowRead: true, MemberAccesses: []*domainmodel.MemberAccess{ma}}
	ar.ID = nextID("ar")
	ent.AccessRules = []*domainmodel.AccessRule{ar}

	vr := &domainmodel.ValidationRule{
		ContainerID: ent.ID,
		AttributeID: model.ID(modName + "." + oldName + "." + attr.Name),
		Type:        "Required",
	}
	vr.ID = nextID("vr")
	ent.ValidationRules = []*domainmodel.ValidationRule{vr}

	// An association reference is NOT qualified by the entity name (it is
	// `Module.Association`), so renaming the entity must leave it exactly as it is.
	// A blanket prefix swap would corrupt it.
	assocRef := modName + ".Widget_Other"
	maAssoc := &domainmodel.MemberAccess{
		AssociationName: assocRef,
		AccessRights:    domainmodel.MemberAccessRightsReadWrite,
	}
	maAssoc.ID = nextID("ma")
	ar.MemberAccesses = append(ar.MemberAccesses, maAssoc)

	// A sibling entity whose own members must not be touched: the rename rewrites
	// what the RENAMED entity says about itself, nothing else in the unit.
	sibAttr := &domainmodel.Attribute{Name: "SibName", Type: &domainmodel.StringAttributeType{Length: 50}}
	sibAttr.ID = nextID("attr")
	sib := &domainmodel.Entity{Name: "Sibling", Persistable: true, Attributes: []*domainmodel.Attribute{sibAttr}}
	sib.ID = nextID("entity")
	sibMA := &domainmodel.MemberAccess{
		AttributeID:   sibAttr.ID,
		AttributeName: modName + ".Sibling." + sibAttr.Name,
		AccessRights:  domainmodel.MemberAccessRightsReadOnly,
	}
	sibMA.ID = nextID("ma")
	sibAR := &domainmodel.AccessRule{ContainerID: sib.ID, AllowRead: true, MemberAccesses: []*domainmodel.MemberAccess{sibMA}}
	sibAR.ID = nextID("ar")
	sib.AccessRules = []*domainmodel.AccessRule{sibAR}

	dm := &domainmodel.DomainModel{ContainerID: mod.ID, Entities: []*domainmodel.Entity{ent, sib}}
	dm.ID = nextID("dm")

	var persisted *domainmodel.DomainModel
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc: func(name string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) {
			return dm, nil
		},
		RenameReferencesFunc: func(_, _ string, _ bool) ([]types.RenameHit, error) { return nil, nil },
		UpdateDomainModelFunc: func(d *domainmodel.DomainModel) error {
			persisted = d
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	if err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "entity",
		Name:       ast.QualifiedName{Module: modName, Name: oldName},
		NewName:    newName,
	}); err != nil {
		t.Fatalf("execRename: %v", err)
	}
	if persisted == nil {
		t.Fatal("UpdateDomainModel was never called; the rename did not persist")
	}

	var renamed, sibling *domainmodel.Entity
	for _, e := range persisted.Entities {
		switch e.ID {
		case ent.ID:
			renamed = e
		case sib.ID:
			sibling = e
		}
	}
	if renamed == nil || sibling == nil {
		t.Fatalf("persisted model lost an entity: renamed=%v sibling=%v", renamed != nil, sibling != nil)
	}
	if renamed.Name != newName {
		t.Fatalf("entity not renamed: got %q, want %q", renamed.Name, newName)
	}

	wantAttrRef := modName + "." + newName + "." + attr.Name
	gotAttrRefs := 0
	for _, r := range renamed.AccessRules {
		for _, m := range r.MemberAccesses {
			if m.AttributeName != "" {
				gotAttrRefs++
				if m.AttributeName != wantAttrRef {
					t.Errorf("access rule still names %q, want %q", m.AttributeName, wantAttrRef)
				}
			}
			if m.AssociationName != "" && m.AssociationName != assocRef {
				t.Errorf("association reference was rewritten to %q; it is not qualified by the entity name and must stay %q",
					m.AssociationName, assocRef)
			}
		}
	}
	if gotAttrRefs != 1 {
		t.Errorf("expected 1 attribute member reference, got %d", gotAttrRefs)
	}

	for _, r := range renamed.ValidationRules {
		if got := string(r.AttributeID); got != wantAttrRef {
			t.Errorf("validation rule still names %q, want %q", got, wantAttrRef)
		}
	}

	// The sibling is the control: a fix that swapped the module prefix, or every
	// occurrence of the old name anywhere in the unit, would have moved this too.
	for _, r := range sibling.AccessRules {
		for _, m := range r.MemberAccesses {
			if want := modName + ".Sibling." + sibAttr.Name; m.AttributeName != want {
				t.Errorf("sibling entity's member reference changed to %q, want %q", m.AttributeName, want)
			}
		}
	}
}

// TestRenameAssociation_RepointsEntityMemberReferences is the same defect one
// document type over: an entity access rule names an association by qualified name,
// and a RENAME ASSOCIATION left that name stale for exactly the same reason — the
// sweep fixes the raw unit, the UpdateDomainModel that follows puts the old name back.
//
// It is the longest-lived of the four: the stale name is then re-read by every later
// statement that loads the unit, so a RENAME ASSOCIATION followed by an unrelated
// RENAME ENTITY carried it forward into the second write too. On the real 11.13 app
// it was the one CE1613 that survived fixing the entity half.
func TestRenameAssociation_RepointsEntityMemberReferences(t *testing.T) {
	const (
		modName = "Ren"
		oldName = "Child_Parent"
		newName = "Kid_Parent"
	)
	mod := mkModule(modName)

	child := &domainmodel.Entity{Name: "Child", Persistable: true}
	child.ID = nextID("entity")
	parent := &domainmodel.Entity{Name: "Parent", Persistable: true}
	parent.ID = nextID("entity")

	// The MemberAccess sits on the FROM (child) entity — Mendix stores it there and
	// only there (a MemberAccess for an association on the TO entity is CE0066).
	ma := &domainmodel.MemberAccess{
		AssociationName: modName + "." + oldName,
		AccessRights:    domainmodel.MemberAccessRightsReadWrite,
	}
	ma.ID = nextID("ma")
	// A similarly-named association that is NOT the one being renamed: the control
	// for a prefix match, which would rewrite this too.
	other := &domainmodel.MemberAccess{
		AssociationName: modName + "." + oldName + "_Extra",
		AccessRights:    domainmodel.MemberAccessRightsReadOnly,
	}
	other.ID = nextID("ma")
	ar := &domainmodel.AccessRule{
		ContainerID:    child.ID,
		AllowRead:      true,
		MemberAccesses: []*domainmodel.MemberAccess{ma, other},
	}
	ar.ID = nextID("ar")
	child.AccessRules = []*domainmodel.AccessRule{ar}

	assoc := &domainmodel.Association{
		Name:     oldName,
		ParentID: child.ID,
		ChildID:  parent.ID,
		Type:     domainmodel.AssociationTypeReference,
		Owner:    domainmodel.AssociationOwnerDefault,
	}
	assoc.ID = nextID("assoc")

	dm := &domainmodel.DomainModel{
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{child, parent},
		Associations: []*domainmodel.Association{assoc},
	}
	dm.ID = nextID("dm")

	var persisted *domainmodel.DomainModel
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:  func(string) (*model.Module, error) { return mod, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		RenameReferencesFunc: func(_, _ string, _ bool) ([]types.RenameHit, error) { return nil, nil },
		UpdateDomainModelFunc: func(d *domainmodel.DomainModel) error {
			persisted = d
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	if err := execRename(ctx, &ast.RenameStmt{
		ObjectType: "association",
		Name:       ast.QualifiedName{Module: modName, Name: oldName},
		NewName:    newName,
	}); err != nil {
		t.Fatalf("execRename: %v", err)
	}
	if persisted == nil {
		t.Fatal("UpdateDomainModel was never called; the rename did not persist")
	}

	if got := persisted.Associations[0].Name; got != newName {
		t.Fatalf("association not renamed: got %q, want %q", got, newName)
	}
	want := modName + "." + newName
	wantOther := modName + "." + oldName + "_Extra"
	for _, e := range persisted.Entities {
		for _, r := range e.AccessRules {
			for _, m := range r.MemberAccesses {
				switch m.ID {
				case ma.ID:
					if m.AssociationName != want {
						t.Errorf("access rule still names %q, want %q", m.AssociationName, want)
					}
				case other.ID:
					if m.AssociationName != wantOther {
						t.Errorf("a differently-named association was rewritten to %q, want %q "+
							"(the match must be exact, not a prefix)", m.AssociationName, wantOther)
					}
				}
			}
		}
	}
}
