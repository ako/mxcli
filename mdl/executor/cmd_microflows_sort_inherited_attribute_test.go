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

// CapTrackV2 FINDINGS §13 — sorting a list on an attribute the entity INHERITS
// is not expressible, in either direction:
//
//	RETRIEVE $U FROM CapTrack.CapTrackUser SORT BY Name ASC;
//	  -> mxcli check and exec both pass, then mxbuild fails
//	     CE1613 "The selected attribute 'CapTrack.CapTrackUser.Name' no longer
//	     exists." — mxcli qualifies with the LIST's entity, mxbuild wants the
//	     DECLARING one.
//
//	RETRIEVE … SORT BY System.User.Name ASC;
//	  -> mxcli refuses: "sort by attribute 'System.User.Name' does not belong to
//	     entity 'CapTrack.CapTrackUser'".
//
// So the spelling that writes is wrong and the spelling that is right is
// refused. Reading the same attribute works — `TEXTBOX (Attribute: Name)`
// resolves — which is the tell: the page builder walks the generalization chain
// (`declaringEntityFor`) and this path did not, though `flowBuilder` has carried
// `resolveAttributeInEntityHierarchy` and `entityIsSubtypeOf` all along.

// inheritedSortBackend is an entity that inherits `Name` from a base in another
// module, and declares `FullName` itself.
func inheritedSortBackend() *mock.MockBackend {
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
				return &domainmodel.DomainModel{
					ContainerID: appModuleID,
					Entities: []*domainmodel.Entity{{
						Name:              "AppUser",
						GeneralizationRef: "SyntheticBase.User",
						Attributes: []*domainmodel.Attribute{
							{Name: "FullName", Type: &domainmodel.StringAttributeType{}},
						},
					}},
				}, nil
			case baseModuleID:
				return &domainmodel.DomainModel{
					ContainerID: baseModuleID,
					Entities: []*domainmodel.Entity{{
						Name: "User",
						Attributes: []*domainmodel.Attribute{
							{Name: "Name", Type: &domainmodel.StringAttributeType{}},
						},
					}},
				}, nil
			}
			return nil, nil
		},
	}
}

// sortAttrOf builds the retrieve and returns the stored sort attribute paths and
// any build errors.
func sortAttrOf(t *testing.T, attr string) (paths []string, errs []string) {
	t.Helper()
	fb := &flowBuilder{backend: inheritedSortBackend(), spacing: 100}
	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable:    "Users",
		Source:      ast.QualifiedName{Module: "SyntheticApp", Name: "AppUser"},
		SortColumns: []ast.SortColumnDef{{Attribute: attr, Order: "ASC"}},
	})
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
		for _, s := range ds.Sorting {
			paths = append(paths, s.AttributeQualifiedName)
		}
	}
	return paths, fb.errors
}

// A bare inherited name must be stored against the DECLARING entity, which is
// what mxbuild resolves.
func TestSortBy_BareInheritedAttributeUsesTheDeclaringEntity(t *testing.T) {
	paths, errs := sortAttrOf(t, "Name")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(paths) != 1 {
		t.Fatalf("got %d sort columns, want 1: %v", len(paths), paths)
	}
	if paths[0] != "SyntheticBase.User.Name" {
		t.Errorf("stored %q, want %q — the list's own entity does not declare Name, "+
			"and mxbuild rejects that reference with CE1613", paths[0], "SyntheticBase.User.Name")
	}
}

// CONTROL 1: an attribute the entity declares ITSELF is still qualified with it.
// A fix that always walked to the base would break every ordinary sort.
func TestSortBy_OwnAttributeIsUnchanged(t *testing.T) {
	paths, errs := sortAttrOf(t, "FullName")
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(paths) != 1 || paths[0] != "SyntheticApp.AppUser.FullName" {
		t.Errorf("stored %v, want [SyntheticApp.AppUser.FullName]", paths)
	}
}

// The other half of §13: naming the declaring entity explicitly is the CORRECT
// reference and was refused outright.
func TestSortBy_QualifiedAncestorAttributeIsAccepted(t *testing.T) {
	paths, errs := sortAttrOf(t, "SyntheticBase.User.Name")
	if len(errs) > 0 {
		t.Fatalf("naming the declaring entity is the reference mxbuild wants, "+
			"and it was refused: %v", errs)
	}
	if len(paths) != 1 || paths[0] != "SyntheticBase.User.Name" {
		t.Errorf("stored %v, want [SyntheticBase.User.Name]", paths)
	}
}

// CONTROL 2: an attribute on an entity that is NOT in the chain and not reachable
// by association is still refused. Accepting anything qualified would turn a
// diagnosable mistake into a CE1613 at the far end of a build.
func TestSortBy_UnrelatedEntityIsStillRefused(t *testing.T) {
	_, errs := sortAttrOf(t, "SyntheticBase.Other.Name")
	if len(errs) == 0 {
		t.Fatal("an attribute of an unrelated entity must still be refused")
	}
	if !strings.Contains(strings.Join(errs, " "), "does not belong to entity") {
		t.Errorf("unexpected message: %v", errs)
	}
}
