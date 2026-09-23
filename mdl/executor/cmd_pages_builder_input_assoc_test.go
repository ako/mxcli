// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#529 (write half) — an input widget could not bind an attribute over
// an association. Every input builder resolved its `attribute:` with
// resolveAttributePath, which knows nothing about associations, so the slashes
// survived into a flat path that resolved to nothing and the build failed
// CE1613.
//
// Studio Pro DOES store this shape on a plain text box. Measured on
// ako/TestApp, page Rules.RuleAction_NewEdit, textBox4:
//
//	Attribute: "Rules.BusinessRule.Name"
//	EntityRef: IndirectEntityRef{ Steps: [ EntityRefStep{
//	    Association: "Rules.RuleAction_BusinessRule",
//	    DestinationEntity: "Rules.BusinessRule" } ] }
//
// which is the same structure DataGrid2 columns and DynamicText parameters
// already produced — so one page could bind an associated attribute in a grid
// column and fail on the text box beside it.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ruleActionPB mirrors TestApp's Rules module: RuleAction has no Name of its
// own, and reaches BusinessRule.Name over RuleAction_BusinessRule. The missing
// Name is the point — it is why the old behaviour produced a binding to
// something that does not exist rather than merely a differently-spelled one.
func ruleActionPB(t *testing.T) *pageBuilder {
	t.Helper()
	const modID = model.ID("mod")
	const actionID = model.ID("e-action")
	const ruleID = model.ID("e-rule")
	return &pageBuilder{
		entityContext: "Rules.RuleAction",
		widgetScope:   map[string]model.ID{},
		execCache: &executorCache{
			hierarchy: &ContainerHierarchy{moduleNames: map[model.ID]string{modID: "Rules"}},
			domainModels: []*domainmodel.DomainModel{{
				ContainerID: modID,
				Entities: []*domainmodel.Entity{
					{BaseElement: model.BaseElement{ID: actionID}, Name: "RuleAction",
						Attributes: []*domainmodel.Attribute{{Name: "ActionType"}}},
					{BaseElement: model.BaseElement{ID: ruleID}, Name: "BusinessRule",
						Attributes: []*domainmodel.Attribute{{Name: "Name"}}},
				},
				Associations: []*domainmodel.Association{
					{Name: "RuleAction_BusinessRule", ParentID: actionID, ChildID: ruleID,
						Type: domainmodel.AssociationTypeReference},
				},
			}},
		},
	}
}

func TestResolveInputAttribute(t *testing.T) {
	pb := ruleActionPB(t)

	t.Run("association path resolves to the final attribute plus one hop", func(t *testing.T) {
		qn, steps := pb.resolveInputAttribute("RuleAction_BusinessRule/Name")
		if qn != "Rules.BusinessRule.Name" {
			t.Errorf("attribute = %q, want Rules.BusinessRule.Name", qn)
		}
		if len(steps) != 1 {
			t.Fatalf("got %d steps, want 1: %+v", len(steps), steps)
		}
		if steps[0].Association != "Rules.RuleAction_BusinessRule" {
			t.Errorf("association = %q, want Rules.RuleAction_BusinessRule", steps[0].Association)
		}
		if steps[0].DestinationEntity != "Rules.BusinessRule" {
			t.Errorf("destination = %q, want Rules.BusinessRule", steps[0].DestinationEntity)
		}
	})

	// The control: an own attribute must still resolve against the context and
	// carry NO steps. Without it a resolver that returned a step for everything
	// would pass the case above.
	t.Run("own attribute carries no steps", func(t *testing.T) {
		qn, steps := pb.resolveInputAttribute("ActionType")
		if qn != "Rules.RuleAction.ActionType" {
			t.Errorf("attribute = %q, want Rules.RuleAction.ActionType", qn)
		}
		if len(steps) != 0 {
			t.Errorf("own attribute gained %d association steps: %+v", len(steps), steps)
		}
	})

	// A path naming an association the model does not have falls back rather
	// than erroring — the reference checker reports the unknown member, and
	// failing here would reject paths whose context this pass cannot see.
	t.Run("unknown association falls back to the flat path", func(t *testing.T) {
		qn, steps := pb.resolveInputAttribute("NotAnAssociation/Name")
		if len(steps) != 0 {
			t.Errorf("unresolvable path produced steps: %+v", steps)
		}
		if qn == "" {
			t.Error("unresolvable path produced an empty attribute")
		}
	})
}

// Every input widget that takes a single `attribute:` must go through the
// resolver — a builder left on resolveAttributePath is exactly the defect, and
// there were six of them. This drives the real builders rather than the helper,
// because the helper being right says nothing about who calls it.
func TestInputWidgetsResolveAssociationAttributes(t *testing.T) {
	for _, widgetType := range []string{"textbox", "textarea", "datepicker", "dropdown", "checkbox", "radiobuttons"} {
		t.Run(widgetType, func(t *testing.T) {
			pb := ruleActionPB(t)
			built, err := pb.buildWidgetV3(&ast.WidgetV3{
				Type: widgetType,
				Name: "w1",
				Properties: map[string]any{
					"Attribute": "RuleAction_BusinessRule/Name",
				},
			})
			if err != nil {
				t.Fatalf("build %s: %v", widgetType, err)
			}
			path, steps := inputAttributeBinding(t, built)
			if path != "Rules.BusinessRule.Name" {
				t.Errorf("%s attribute = %q, want Rules.BusinessRule.Name", widgetType, path)
			}
			if len(steps) != 1 || steps[0].Association != "Rules.RuleAction_BusinessRule" {
				t.Errorf("%s lost the association hop: %+v", widgetType, steps)
			}
		})
	}
}

// inputAttributeBinding reads back the two fields under test from whichever
// input widget the builder produced.
func inputAttributeBinding(t *testing.T, w pages.Widget) (string, []pages.AttributeRefStep) {
	t.Helper()
	switch x := w.(type) {
	case *pages.TextBox:
		return x.AttributePath, x.AttributeRefSteps
	case *pages.TextArea:
		return x.AttributePath, x.AttributeRefSteps
	case *pages.DatePicker:
		return x.AttributePath, x.AttributeRefSteps
	case *pages.DropDown:
		return x.AttributePath, x.AttributeRefSteps
	case *pages.CheckBox:
		return x.AttributePath, x.AttributeRefSteps
	case *pages.RadioButtons:
		return x.AttributePath, x.AttributeRefSteps
	default:
		t.Fatalf("unexpected widget type %T — add it to this switch, and to the builder list above", w)
		return "", nil
	}
}
