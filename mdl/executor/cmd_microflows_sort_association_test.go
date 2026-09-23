// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1152 — "roundtrip for retrieve DB with sorting over association does
// not work". `mxcli describe` emitted
//
//	retrieve $AccountList from Administration.Account
//	sort by System.Language.Code asc;
//
// `mxcli check` reported no errors and `mxcli exec` refused it:
//
//	Error: microflow 'ExamplesModule.ACT_sort_test ' has validation errors:
//	sort by attribute 'System.Language.Code' does not belong to entity
//	'Administration.Account'
//
// The hop that reaches System.Language is System.User_Language, which is
// declared on the ANCESTOR System.User and stored in the SYSTEM module's domain
// model. inferSortEntityRefSteps looked only in the retrieved entity's own
// module (Administration) and only at associations whose parent was the
// retrieved entity itself, so it found nothing and the sort was refused.
//
// This backend is that shape with synthetic names: SyntheticApp.AppUser
// generalizes SyntheticBase.User, and the association to SyntheticBase.Language
// lives on the base, in the base's module.
func associationSortBackend() *mock.MockBackend {
	appModuleID := model.ID("synthetic-app-module")
	baseModuleID := model.ID("synthetic-base-module")
	return &mock.MockBackend{
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			switch name {
			case "SyntheticApp":
				return &model.Module{BaseElement: model.BaseElement{ID: appModuleID}, Name: name}, nil
			case "SyntheticBase":
				return &model.Module{BaseElement: model.BaseElement{ID: baseModuleID}, Name: name}, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
			switch id {
			case appModuleID:
				profile := &domainmodel.Entity{Name: "Profile"}
				profile.ID = model.ID("SyntheticApp.Profile")
				appUser := &domainmodel.Entity{
					Name:              "AppUser",
					GeneralizationRef: "SyntheticBase.User",
					Attributes: []*domainmodel.Attribute{
						{Name: "FullName", Type: &domainmodel.StringAttributeType{}},
					},
				}
				appUser.ID = model.ID("SyntheticApp.AppUser")
				own := &domainmodel.Association{
					Name:     "AppUser_Profile",
					ParentID: appUser.ID,
					ChildID:  profile.ID,
					Type:     domainmodel.AssociationTypeReference,
				}
				own.ID = model.ID("SyntheticApp.AppUser_Profile")
				return &domainmodel.DomainModel{
					ContainerID:  appModuleID,
					Entities:     []*domainmodel.Entity{appUser, profile},
					Associations: []*domainmodel.Association{own},
				}, nil
			case baseModuleID:
				user := &domainmodel.Entity{
					Name: "User",
					Attributes: []*domainmodel.Attribute{
						{Name: "Name", Type: &domainmodel.StringAttributeType{}},
					},
				}
				user.ID = model.ID("SyntheticBase.User")
				language := &domainmodel.Entity{
					Name: "Language",
					Attributes: []*domainmodel.Attribute{
						{Name: "Code", Type: &domainmodel.StringAttributeType{}},
					},
				}
				language.ID = model.ID("SyntheticBase.Language")
				other := &domainmodel.Entity{Name: "Other"}
				other.ID = model.ID("SyntheticBase.Other")
				assoc := &domainmodel.Association{
					Name:     "User_Language",
					ParentID: user.ID,
					ChildID:  language.ID,
					Type:     domainmodel.AssociationTypeReference,
				}
				assoc.ID = model.ID("SyntheticBase.User_Language")
				return &domainmodel.DomainModel{
					ContainerID:  baseModuleID,
					Entities:     []*domainmodel.Entity{user, language, other},
					Associations: []*domainmodel.Association{assoc},
				}, nil
			}
			return nil, nil
		},
	}
}

// assocSortOf builds the retrieve against associationSortBackend and returns the
// stored sort attribute path, its EntityRefSteps, and any build errors.
func assocSortOf(t *testing.T, attr string) (path string, steps []microflows.EntityRefStep, errs []string) {
	t.Helper()
	fb := &flowBuilder{backend: associationSortBackend(), spacing: 100}
	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:    "Users",
		Source:      ast.QualifiedName{Module: "SyntheticApp", Name: "AppUser"},
		SortColumns: []ast.SortColumnDef{{Attribute: attr, Order: "ASC"}},
	})
	var items []*microflows.SortItem
	for _, obj := range fb.objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		ra, ok := act.Action.(*microflows.RetrieveAction)
		if !ok {
			continue
		}
		ds, ok := ra.Source.(*microflows.DatabaseRetrieveSource)
		if !ok {
			continue
		}
		items = append(items, ds.Sorting...)
	}
	if len(items) > 1 {
		t.Fatalf("got %d sort columns, want at most 1", len(items))
	}
	if len(items) == 1 {
		path, steps = items[0].AttributeQualifiedName, items[0].EntityRefSteps
	}
	return path, steps, fb.errors
}

// The reported case: the association is on an ancestor, in the ancestor's
// module. Before the fix this produced "does not belong to entity".
func TestSortBy_AssociationOnAncestorInAnotherModule(t *testing.T) {
	path, steps, errs := assocSortOf(t, "SyntheticBase.Language.Code")
	if len(errs) > 0 {
		t.Fatalf("describe emitted this sort and exec refused it (mendixlabs/mxcli#1152): %v", errs)
	}
	if path != "SyntheticBase.Language.Code" {
		t.Errorf("stored attribute %q, want %q", path, "SyntheticBase.Language.Code")
	}
	if len(steps) != 1 {
		t.Fatalf("got %d entity ref steps, want 1: %+v", len(steps), steps)
	}
	// The association is qualified with the module that STORES it, not with the
	// retrieved entity's module — the two differ exactly in this case.
	if steps[0].Association != "SyntheticBase.User_Language" {
		t.Errorf("association %q, want %q", steps[0].Association, "SyntheticBase.User_Language")
	}
	if steps[0].DestinationEntity != "SyntheticBase.Language" {
		t.Errorf("destination %q, want %q", steps[0].DestinationEntity, "SyntheticBase.Language")
	}
}

// CONTROL 1: an association declared on the retrieved entity itself, in its own
// module, still resolves. A fix that only looked at ancestors would break the
// case that already worked.
func TestSortBy_AssociationOnTheRetrievedEntityStillResolves(t *testing.T) {
	_, steps, errs := assocSortOf(t, "SyntheticApp.Profile.Label")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(steps) != 1 || steps[0].Association != "SyntheticApp.AppUser_Profile" ||
		steps[0].DestinationEntity != "SyntheticApp.Profile" {
		t.Errorf("got %+v, want one hop SyntheticApp.AppUser_Profile -> SyntheticApp.Profile", steps)
	}
}

// CONTROL 2: an entity that is neither in the generalization chain nor reachable
// by one association hop is still refused. Accepting anything qualified would
// turn a diagnosable mistake into a CE1613 at the far end of a build.
func TestSortBy_UnreachableEntityIsStillRefused(t *testing.T) {
	_, _, errs := assocSortOf(t, "SyntheticBase.Other.Name")
	if len(errs) == 0 {
		t.Fatal("an attribute of an unreachable entity must still be refused")
	}
	if !strings.Contains(strings.Join(errs, " "), "does not belong to entity") {
		t.Errorf("unexpected message: %v", errs)
	}
}
