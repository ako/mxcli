// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// ako/mxcli#662: a bare association name is qualified with the module that
// DECLARES it, found by walking the context entity's generalization chain — not
// with the module of whichever entity happens to be the context.
//
// Administration.Account extends System.User, and UserRoles is declared on
// System.User. `DESCRIBE PAGE Administration.Account_New` emits
// `combobox (Attribute: UserRoles, DataSource: database from System.UserRole)`,
// and exec qualified the bare name with the page entity's module, writing
// `Administration.UserRoles`:
//
//	[CE1613] "The selected association 'Administration.UserRoles' no longer exists."
//
// Before f0d1aea80 (issuetracker #19) the name was qualified with the OPTION
// LIST's module and came out right here only by accident — System.UserRole
// happens to live where UserRoles is declared. Both rules guess from a module
// name; the fixture below holds cases each of them gets wrong.
func pageInheritedAssocFixture(entityContext string) *pageBuilder {
	const (
		sysID     = model.ID("mod-system")
		adminID   = model.ID("mod-admin")
		itID      = model.ID("mod-it")
		userID    = model.ID("e-user")
		roleID    = model.ID("e-userrole")
		langID    = model.ID("e-language")
		accountID = model.ID("e-account")
		issueID   = model.ID("e-issue")
		noteID    = model.ID("e-note")
	)
	return &pageBuilder{
		entityContext:    entityContext,
		paramEntityNames: map[string]string{},
		widgetScope:      map[string]model.ID{},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{
				sysID:   "System",
				adminID: "Administration",
				itID:    "IT",
			}},
			domainModels: []*domainmodel.DomainModel{
				{
					ContainerID: sysID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: userID}, Name: "User"},
						{BaseElement: model.BaseElement{ID: roleID}, Name: "UserRole"},
						{BaseElement: model.BaseElement{ID: langID}, Name: "Language"},
					},
					Associations: []*domainmodel.Association{
						{Name: "UserRoles", ParentID: userID, ChildID: roleID, Type: domainmodel.AssociationTypeReferenceSet},
						{Name: "User_Language", ParentID: userID, ChildID: langID, Type: domainmodel.AssociationTypeReference},
					},
				},
				{
					ContainerID: adminID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: accountID}, Name: "Account", GeneralizationRef: "System.User"},
					},
				},
				{
					ContainerID: itID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: issueID}, Name: "Issue"},
						{BaseElement: model.BaseElement{ID: noteID}, Name: "Note"},
					},
					Associations: []*domainmodel.Association{
						{Name: "Note_Issue", ParentID: noteID, ChildID: issueID, Type: domainmodel.AssociationTypeReference},
					},
					CrossAssociations: []*domainmodel.CrossModuleAssociation{
						{Name: "Issue_Assignee", ParentID: issueID, ChildRef: "System.User", Type: domainmodel.AssociationTypeReference},
						// Declared in IT, navigated FROM the System side.
						{Name: "Issue_Reporter", ParentID: issueID, ChildRef: "System.User", Type: domainmodel.AssociationTypeReference},
					},
				},
			},
		},
	}
}

func TestResolveAssociationPathIn_DeclaringModule(t *testing.T) {
	tests := []struct {
		name, assoc, context, want string
	}{
		{"inherited from a System parent (#662)", "UserRoles", "Administration.Account", "System.UserRoles"},
		{"inherited, second association (#662)", "User_Language", "Administration.Account", "System.User_Language"},
		{"declared on the context itself", "UserRoles", "System.User", "System.UserRoles"},
		{"cross-module FROM end (issuetracker #19)", "Issue_Assignee", "IT.Issue", "IT.Issue_Assignee"},
		{"reverse navigation from the other module's end", "Issue_Reporter", "Administration.Account", "IT.Issue_Reporter"},
		{"same-module TO end", "Note_Issue", "IT.Issue", "IT.Note_Issue"},
		// Unknown to the model: keep the historical guess rather than refuse,
		// so the reference validator reports it by the name the author wrote.
		{"unknown name falls back to context module", "Nope", "Administration.Account", "Administration.Nope"},
		{"already qualified is untouched", "Other.UserRoles", "Administration.Account", "Other.UserRoles"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pb := pageInheritedAssocFixture(tc.context)
			if got := pb.resolveAssociationPathIn(tc.assoc, tc.context); got != tc.want {
				t.Errorf("resolveAssociationPathIn(%q, %q) = %q, want %q", tc.assoc, tc.context, got, tc.want)
			}
		})
	}
}

// The reported shape end to end through the widget engine: a ComboBox whose
// DataSource mapping has already moved entityContext to its option list, inside
// a data view over the specialization.
func TestResolveMapping_Association_InheritedFromGeneralization(t *testing.T) {
	pb := pageInheritedAssocFixture("System.UserRole") // moved by the DataSource mapping
	engine := &PluggableWidgetEngine{pageBuilder: pb, outerEntityContext: "Administration.Account"}

	mapping := PropertyMapping{PropertyKey: "attributeAssociation", Source: "Association", Operation: "association"}
	w := &ast.WidgetV3{Properties: map[string]any{"Attribute": "UserRoles"}}

	ctx, err := engine.resolveMapping(mapping, w)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := "System.UserRoles"; ctx.AssocPath != want {
		t.Errorf("AssocPath = %q, want %q (Administration.UserRoles is CE1613 \"no longer exists\")", ctx.AssocPath, want)
	}
}

// A multi-hop attribute path qualifies EACH hop against the entity that hop
// starts from, not against the path's starting entity.
func TestResolveAssociationAttributePath_HopQualifiedPerStep(t *testing.T) {
	pb := pageInheritedAssocFixture("IT.Note")
	finalQN, steps, ok := pb.resolveAssociationAttributePath("Note_Issue/Issue_Assignee/UserRoles/Name")
	if !ok {
		t.Fatalf("path dropped (ok=false) — the third hop starts at System.User, and qualifying it against the path's start (IT.Note) gives IT.UserRoles")
	}
	if len(steps) != 3 || steps[0].Association != "IT.Note_Issue" ||
		steps[1].Association != "IT.Issue_Assignee" || steps[2].Association != "System.UserRoles" {
		t.Errorf("steps = %+v", steps)
	}
	if finalQN != "System.UserRole.Name" {
		t.Errorf("finalQN = %q, want System.UserRole.Name", finalQN)
	}

	// Inherited association on the first hop.
	pb = pageInheritedAssocFixture("Administration.Account")
	_, steps, ok = pb.resolveAssociationAttributePath("User_Language/Code")
	if !ok || len(steps) != 1 || steps[0].Association != "System.User_Language" {
		t.Errorf("inherited hop: ok=%v steps=%+v, want System.User_Language", ok, steps)
	}
}
