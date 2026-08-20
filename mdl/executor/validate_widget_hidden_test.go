// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// accordionLikeRegistry mirrors the shape that matters for #931: a widget with an
// object list whose item sub-properties are hidden by two different conditions —
// one read off the WIDGET (collapsible), one off the ITEM (initialCollapsedState).
func accordionLikeRegistry() *WidgetRegistry {
	def := &WidgetDefinition{
		WidgetID: "com.mendix.widget.web.accordion.Accordion",
		MDLName:  "ACCORDION",
		PropertyMappings: []PropertyMapping{
			{PropertyKey: "collapsible", Operation: "primitive", Value: "true"},
			{PropertyKey: "expandBehavior", Operation: "primitive", Value: "singleExpanded"},
		},
		ObjectLists: []ObjectListMapping{{
			PropertyKey:  "groups",
			MDLContainer: "GROUP",
			ItemProperties: []ItemPropertyMapping{
				{PropertyKey: "initialCollapsedState", Operation: "primitive", Value: "collapsed"},
				{PropertyKey: "initiallyCollapsed", Operation: "expression", Default: "true"},
			},
		}},
		PropertyVisibility: []types.WidgetVisibilityRule{
			{PropertyKey: "expandBehavior", HiddenWhen: &types.WidgetVisibilityCondition{
				PropertyKey: "collapsible", Operator: "falsy",
			}},
			{PropertyKey: "initialCollapsedState", ListPropertyKey: "groups",
				HiddenWhen: &types.WidgetVisibilityCondition{PropertyKey: "collapsible", Operator: "falsy"}},
			{PropertyKey: "initiallyCollapsed", ListPropertyKey: "groups",
				HiddenWhen: &types.WidgetVisibilityCondition{
					PropertyKey: "initialCollapsedState", Operator: "ne", Value: "dynamic",
					Scope: types.ConditionScopeItem,
				}},
		},
	}
	return &WidgetRegistry{byMDLName: map[string]*WidgetDefinition{"ACCORDION": def}}
}

func accordionWidget(collapsible string, groupProps map[string]any) (*ast.WidgetV3, *ast.WidgetV3) {
	group := &ast.WidgetV3{Type: "GROUP", Name: "g1", Properties: groupProps}
	return &ast.WidgetV3{
		Type:       "accordion",
		Name:       "acc1",
		Properties: map[string]any{"collapsible": collapsible},
		Children:   []*ast.WidgetV3{group},
	}, group
}

func severities(vs []linter.Violation) (errors, warnings int) {
	for _, v := range vs {
		switch v.Severity {
		case linter.SeverityError:
			errors++
		case linter.SeverityWarning:
			warnings++
		}
	}
	return errors, warnings
}

// The value decides the severity, not the property. Measured on mxbuild 11.13:
// `collapsible: false` with the group's initialCollapsedState left at "collapsed"
// checks clean, and with "expanded" it is CE0463 — so the same hidden property is
// a warning in one script and a build failure in the other.
func TestHiddenItemPropertySeverityFollowsTheValue(t *testing.T) {
	registry := accordionLikeRegistry()
	mapping := &registry.byMDLName["ACCORDION"].ObjectLists[0]

	nonDefault, group := accordionWidget("false", map[string]any{"InitialCollapsedState": "expanded"})
	vs := validateWidgetItemVisibility(nonDefault, group, mapping, registry, "page P")
	errs, warns := severities(vs)
	if errs != 1 || warns != 0 {
		t.Fatalf("non-default on a hidden item property → %d errors %d warnings %+v, want 1 error", errs, warns, vs)
	}
	if !strings.Contains(vs[0].Message, "CE0463") || !strings.Contains(vs[0].Message, `"collapsed"`) {
		t.Errorf("message should name the consequence and the default, got %q", vs[0].Message)
	}
	if !strings.Contains(vs[0].Message, "group `g1`") {
		t.Errorf("message should name the item it fired on, got %q", vs[0].Message)
	}

	atDefault, group := accordionWidget("false", map[string]any{"InitialCollapsedState": "collapsed"})
	vs = validateWidgetItemVisibility(atDefault, group, mapping, registry, "page P")
	errs, warns = severities(vs)
	if errs != 0 || warns != 1 {
		t.Fatalf("default value on a hidden item property → %d errors %d warnings %+v, want 1 warning", errs, warns, vs)
	}
}

// A visible property is not flagged at all: with `collapsible` on, the whole
// State group is shown and any value is legal (measured — 0 errors on mxbuild).
func TestVisibleItemPropertyNotFlagged(t *testing.T) {
	registry := accordionLikeRegistry()
	mapping := &registry.byMDLName["ACCORDION"].ObjectLists[0]

	widget, group := accordionWidget("true", map[string]any{"InitialCollapsedState": "expanded"})
	for _, v := range validateWidgetItemVisibility(widget, group, mapping, registry, "page P") {
		if strings.Contains(v.Message, "initialCollapsedState") {
			t.Errorf("collapsible on → initialCollapsedState is visible, got %q", v.Message)
		}
	}
}

// The item-scoped condition reads the GROUP, not the widget. Evaluating it
// against the widget finds no `initialCollapsedState` key and reports nothing —
// which is how this CE0463 stayed silent even once the nested rules existed.
func TestItemScopedConditionReadsTheItem(t *testing.T) {
	registry := accordionLikeRegistry()
	mapping := &registry.byMDLName["ACCORDION"].ObjectLists[0]

	widget, group := accordionWidget("true", map[string]any{
		"InitialCollapsedState": "expanded", // ≠ dynamic ⇒ initiallyCollapsed hidden
		"InitiallyCollapsed":    "false",    // ≠ its default "true"
	})
	vs := validateWidgetItemVisibility(widget, group, mapping, registry, "page P")
	errs, _ := severities(vs)
	if errs != 1 {
		t.Fatalf("got %d errors %+v, want 1 for initiallyCollapsed", errs, vs)
	}
	if !strings.Contains(vs[0].Message, "initiallyCollapsed") ||
		!strings.Contains(vs[0].Message, "its own `initialCollapsedState`") {
		t.Errorf("message should attribute the condition to the item, got %q", vs[0].Message)
	}

	// dynamic ⇒ the property is shown, so the same value is fine.
	ok, okGroup := accordionWidget("true", map[string]any{
		"InitialCollapsedState": "dynamic",
		"InitiallyCollapsed":    "false",
	})
	if vs := validateWidgetItemVisibility(ok, okGroup, mapping, registry, "page P"); len(vs) != 0 {
		t.Errorf("initialCollapsedState:dynamic → initiallyCollapsed is visible, got %+v", vs)
	}
}

// A top-level hidden property splits the same way — this is the pre-existing
// MDL-WIDGET10 path, which reported every case as a warning. `collapsible: false`
// with a non-default expandBehavior is CE0463 on mxbuild.
func TestHiddenTopLevelPropertySeverityFollowsTheValue(t *testing.T) {
	registry := accordionLikeRegistry()

	nonDefault := &ast.WidgetV3{Type: "accordion", Name: "acc1", Properties: map[string]any{
		"collapsible":    "false",
		"expandBehavior": "multipleExpanded",
	}}
	errs, warns := severities(validateWidgetVisibility(nonDefault, registry, "page P"))
	if errs != 1 || warns != 0 {
		t.Errorf("non-default hidden top-level property → %d errors %d warnings, want 1 error", errs, warns)
	}

	atDefault := &ast.WidgetV3{Type: "accordion", Name: "acc1", Properties: map[string]any{
		"collapsible":    "false",
		"expandBehavior": "singleExpanded",
	}}
	errs, warns = severities(validateWidgetVisibility(atDefault, registry, "page P"))
	if errs != 0 || warns != 1 {
		t.Errorf("default hidden top-level property → %d errors %d warnings, want 1 warning", errs, warns)
	}
}
