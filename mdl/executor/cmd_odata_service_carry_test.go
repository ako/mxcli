// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// Running a published service's describe output as `create or modify` over the
// Studio Pro-authored ako/TestApp (Services.OrderODataApi and two more) changed
// every entity set's PageSize from 10000 to 0 (describe prints PageSize only
// with UsePaging), reordered the entity sets into entity-type order, and
// recomputed each member's CanBeEmpty (#743). None of the three has a spelling
// in the statement, so the rewrite carries the stored value.
func TestModifyODataService_CarriesPageSizeOrderAndCanBeEmpty(t *testing.T) {
	svc, mb, h := existingPublishedService()
	no := false
	svc.EntityTypes = append(svc.EntityTypes, &model.PublishedEntityType{
		ExposedName: "Line", Entity: "MyModule.Line",
		Members: []*model.PublishedMember{{Kind: "attribute", Name: "MyModule.Line.Amount", ExposedName: "Amount", CanBeEmpty: &no}},
	})
	svc.EntityTypes[0].Members[0].CanBeEmpty = &no
	// Stored set order is Lines, Orders: not the entity-type order.
	svc.EntitySets = []*model.PublishedEntitySet{
		{ExposedName: "Lines", EntityTypeName: "MyModule.Line", PageSize: 10000},
		{ExposedName: "Orders", EntityTypeName: "MyModule.Order", PageSize: 10000},
	}
	var updated *model.PublishedODataService
	mb.UpdatePublishedODataServiceFunc = func(s *model.PublishedODataService) error { updated = s; return nil }
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	stmt := &ast.CreateODataServiceStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "CatalogService"},
		CreateOrModify: true,
		Entities: []*ast.PublishedEntityDef{
			{Entity: ast.QualifiedName{Module: "MyModule", Name: "Order"}, ExposedName: "Orders",
				Members: []*ast.PublishedMemberDef{{Name: "Label", ExposedName: "label"}}},
			{Entity: ast.QualifiedName{Module: "MyModule", Name: "Line"}, ExposedName: "Lines",
				Members: []*ast.PublishedMemberDef{{Name: "Amount", ExposedName: "Amount"}, {Name: "Added", ExposedName: "Added"}}},
			{Entity: ast.QualifiedName{Module: "MyModule", Name: "New"}, ExposedName: "News", UsePaging: true, PageSize: 50},
		},
	}
	assertNoError(t, createODataService(ctx, stmt))
	if updated == nil {
		t.Fatal("the service was never updated")
	}

	var order []string
	for _, es := range updated.EntitySets {
		order = append(order, es.ExposedName)
	}
	if len(order) != 3 || order[0] != "Lines" || order[1] != "Orders" || order[2] != "News" {
		t.Errorf("entity set order = %v, want the stored [Lines Orders] then the new News", order)
	}
	for _, es := range updated.EntitySets {
		want := 10000
		if es.ExposedName == "News" {
			want = 50 // declared: the statement wins
		}
		if es.PageSize != want {
			t.Errorf("%s PageSize = %d, want %d", es.ExposedName, es.PageSize, want)
		}
	}
	if m := findPublishedMember(t, updated, "Label"); m.CanBeEmpty == nil || *m.CanBeEmpty {
		t.Errorf("Label CanBeEmpty = %v, want the stored false", m.CanBeEmpty)
	}
	if m := findPublishedMember(t, updated, "Amount"); m.CanBeEmpty == nil || *m.CanBeEmpty {
		t.Errorf("Amount CanBeEmpty = %v, want the stored false", m.CanBeEmpty)
	}
	// Control: a member the stored service did not publish carries nothing.
	if m := findPublishedMember(t, updated, "Added"); m.CanBeEmpty != nil {
		t.Errorf("new member Added got CanBeEmpty %v", *m.CanBeEmpty)
	}
}

// The CanBeEmpty carry above is keyed on the member's name, so it also carried
// across a change of key status. Exposing a member without (KEY) stores
// CanBeEmpty true; re-running the statement with `Email (KEY)` then wrote true
// onto a key member, and mxbuild refused it: CE0309 "Exposed attribute 'Email'
// of entity 'Customer' that is part of the key cannot be marked as 'Can be
// empty'". A fresh create of the same statement built clean. The stored value
// is the member's state only while its key status is the one it was stored
// with; when that changes, the derived value (!IsPartOfKey) applies.
func TestModifyODataService_CanBeEmptyNotCarriedAcrossKeyChange(t *testing.T) {
	svc, mb, h := existingPublishedService()
	yes, no := true, false
	svc.EntityTypes[0].Members = []*model.PublishedMember{
		{Kind: "attribute", Name: "Label", ExposedName: "label", CanBeEmpty: &yes},                 // becomes a key
		{Kind: "attribute", Name: "Code", ExposedName: "code", IsPartOfKey: true, CanBeEmpty: &no}, // stops being one
		{Kind: "attribute", Name: "Note", ExposedName: "note", CanBeEmpty: &yes},                   // control: unchanged
	}
	var updated *model.PublishedODataService
	mb.UpdatePublishedODataServiceFunc = func(s *model.PublishedODataService) error { updated = s; return nil }
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	stmt := &ast.CreateODataServiceStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "CatalogService"},
		CreateOrModify: true,
		Entities: []*ast.PublishedEntityDef{
			{Entity: ast.QualifiedName{Module: "MyModule", Name: "Order"}, ExposedName: "Orders",
				Members: []*ast.PublishedMemberDef{
					{Name: "Label", ExposedName: "label", IsPartOfKey: true},
					{Name: "Code", ExposedName: "code"},
					{Name: "Note", ExposedName: "note"},
				}},
		},
	}
	assertNoError(t, createODataService(ctx, stmt))
	if updated == nil {
		t.Fatal("the service was never updated")
	}

	if m := findPublishedMember(t, updated, "Label"); m.CanBeEmpty != nil && *m.CanBeEmpty {
		t.Errorf("Label is now part of the key but carried CanBeEmpty true (CE0309)")
	}
	if m := findPublishedMember(t, updated, "Code"); m.CanBeEmpty != nil && !*m.CanBeEmpty {
		t.Errorf("Code is no longer part of the key but carried the key's CanBeEmpty false")
	}
	// Control: a member whose key status did not change still carries.
	if m := findPublishedMember(t, updated, "Note"); m.CanBeEmpty == nil || !*m.CanBeEmpty {
		t.Errorf("Note CanBeEmpty = %v, want the stored true", m.CanBeEmpty)
	}
}
