// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestAddRetrieveAction_AllowsAssociationPathSortAttribute(t *testing.T) {
	moduleID := model.ID("sample-module")
	parentID := model.ID("parent-entity")
	childID := model.ID("child-entity")
	fb := &flowBuilder{
		varTypes: map[string]string{},
		backend: &mock.MockBackend{
			GetModuleByNameFunc: func(name string) (*model.Module, error) {
				if name != "SampleApps" {
					return nil, nil
				}
				return &model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: name}, nil
			},
			GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) {
				if id != moduleID {
					return nil, nil
				}
				return &domainmodel.DomainModel{
					ContainerID: moduleID,
					Entities: []*domainmodel.Entity{
						{BaseElement: model.BaseElement{ID: parentID}, Name: "DeploymentTarget"},
						{BaseElement: model.BaseElement{ID: childID}, Name: "ApplicationView"},
					},
					Associations: []*domainmodel.Association{
						{
							Name:     "DeploymentTarget_ApplicationView",
							ParentID: parentID,
							ChildID:  childID,
							Type:     domainmodel.AssociationTypeReference,
						},
					},
				}, nil
			},
		},
	}

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable: "DeploymentTargetList",
		Source: ast.QualifiedName{
			Module: "SampleApps",
			Name:   "DeploymentTarget",
		},
		SortColumns: []ast.SortColumnDef{
			{Attribute: "SampleApps.ApplicationView.CreatedAt", Order: "DESC"},
			{Attribute: "Name", Order: "ASC"},
		},
	})

	if len(fb.errors) > 0 {
		t.Fatalf("unexpected builder errors: %v", fb.errors)
	}
	if len(fb.objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(fb.objects))
	}

	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("got object %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.RetrieveAction)
	if !ok {
		t.Fatalf("got action %T, want *microflows.RetrieveAction", activity.Action)
	}
	source, ok := action.Source.(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatalf("got source %T, want *microflows.DatabaseRetrieveSource", action.Source)
	}
	if len(source.Sorting) != 2 {
		t.Fatalf("got %d sort items, want 2", len(source.Sorting))
	}
	if got := source.Sorting[0].AttributeQualifiedName; got != "SampleApps.ApplicationView.CreatedAt" {
		t.Fatalf("first sort attribute = %q", got)
	}
	if got := source.Sorting[0].EntityRefSteps; len(got) != 1 || got[0].Association != "SampleApps.DeploymentTarget_ApplicationView" || got[0].DestinationEntity != "SampleApps.ApplicationView" {
		t.Fatalf("first sort entity ref steps = %#v", got)
	}
	if got := source.Sorting[0].Direction; got != microflows.SortDirectionDescending {
		t.Fatalf("first sort direction = %q, want %q", got, microflows.SortDirectionDescending)
	}
	if got := source.Sorting[1].AttributeQualifiedName; got != "SampleApps.DeploymentTarget.Name" {
		t.Fatalf("second sort attribute = %q", got)
	}
	if got := source.Sorting[1].EntityRefSteps; len(got) != 0 {
		t.Fatalf("second sort entity ref steps = %#v, want none", got)
	}
}

func TestAddRetrieveAction_PreservesMultipleXPathPredicates(t *testing.T) {
	fb := &flowBuilder{
		varTypes: map[string]string{},
	}
	where := "[CreatedAt > $Token/CreatedAt or (CreatedAt = $Token/CreatedAt and ItemId > $Token/ItemId)][CreatedAt < '[%CurrentDateTime%]'][ExternalId != empty]"

	fb.addRetrieveAction(&ast.RetrieveStmt{
		Variable: "Items",
		Source: ast.QualifiedName{
			Module: "Synthetic",
			Name:   "Item",
		},
		Where: &ast.SourceExpr{
			Expression: &ast.BinaryExpr{
				Left:     &ast.IdentifierExpr{Name: "CreatedAt"},
				Operator: ">",
				Right:    &ast.AttributePathExpr{Variable: "Token", Path: []string{"CreatedAt"}},
			},
			Source: where,
		},
	})

	if len(fb.errors) > 0 {
		t.Fatalf("unexpected builder errors: %v", fb.errors)
	}
	if len(fb.objects) != 1 {
		t.Fatalf("got %d objects, want 1", len(fb.objects))
	}
	activity, ok := fb.objects[0].(*microflows.ActionActivity)
	if !ok {
		t.Fatalf("got object %T, want *microflows.ActionActivity", fb.objects[0])
	}
	action, ok := activity.Action.(*microflows.RetrieveAction)
	if !ok {
		t.Fatalf("got action %T, want *microflows.RetrieveAction", activity.Action)
	}
	source, ok := action.Source.(*microflows.DatabaseRetrieveSource)
	if !ok {
		t.Fatalf("got source %T, want *microflows.DatabaseRetrieveSource", action.Source)
	}
	// This constraint is 129 characters, so it is now stored broken across lines
	// so it can be read in Studio Pro's editor (upstream #979). What #772 is about
	// — that all three predicate groups survive, not just the first — is asserted
	// on the flattened form, which is independent of how it was laid out.
	if got := visitor.FlattenXPathConstraint(source.XPathConstraint); got != where {
		t.Fatalf("XPathConstraint flattens to %q, want %q", got, where)
	}
	wantLayout := strings.Join([]string{
		"[",
		"  CreatedAt > $Token/CreatedAt",
		"  or (CreatedAt = $Token/CreatedAt and ItemId > $Token/ItemId)",
		"]",
		"[CreatedAt < '[%CurrentDateTime%]']",
		"[ExternalId != empty]",
	}, "\n")
	if source.XPathConstraint != wantLayout {
		t.Errorf("XPathConstraint layout\n got:\n%s\nwant:\n%s", source.XPathConstraint, wantLayout)
	}
}
