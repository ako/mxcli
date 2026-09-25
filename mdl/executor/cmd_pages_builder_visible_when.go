// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// emptyConditionValue is how Studio Pro stores an enumeration attribute's
// "(empty)" choice; MDL spells it `empty`.
const emptyConditionValue = "(empty)"

// applyVisibleWhen writes `Visible: Attr in (v1, …)` — Studio Pro's "Visible:
// based on attribute value".
//
// Studio Pro stores ONE condition PER VALUE of the attribute, flagged visible or
// not: every enumeration value in declaration order plus "(empty)", or "true"
// then "false" for a boolean. Measured on the 12 attribute-based settings in a
// stock Administration v4.3.2 + Feedback v4.0.2 project at 11.13.0 (e.g.
// Administration.ScheduledEvents: Running true; Completed, Error, Stopped,
// (empty) false). MDL names only the values that show the widget, and the full
// list is filled from the domain model.
//
// Without this the setting had no MDL spelling, so describe → exec dropped it
// and the widget became ALWAYS visible — with mx check clean.
func (pb *pageBuilder) applyVisibleWhen(widget pages.Widget, w *ast.WidgetV3) error {
	vw, ok := w.Properties["VisibleWhen"].(*ast.VisibleWhenV3)
	if !ok || vw == nil {
		return nil
	}
	type baseWidgetGetter interface {
		GetBaseWidget() *pages.BaseWidget
	}
	bwg, ok := widget.(baseWidgetGetter)
	if !ok {
		return mdlerrors.NewValidationf("%s %s: `Visible: %s in (…)` is not supported on this widget", w.Type, w.Name, vw.Attribute)
	}
	where := fmt.Sprintf("%s %s: Visible: %s in (…)", w.Type, w.Name, vw.Attribute)
	if pb.entityContext == "" {
		return mdlerrors.NewValidationf("%s: the attribute is read from the enclosing data container's object — place the widget inside a data container", where)
	}
	if strings.ContainsAny(vw.Attribute, "/.") {
		return mdlerrors.NewValidationf("%s: name an attribute of the data container's own entity (%s); association paths are not supported", where, pb.entityContext)
	}

	declaring, ok := pb.declaringEntityFor(pb.entityContext, vw.Attribute)
	if !ok {
		return mdlerrors.NewValidationf("%s: %s has no attribute %s", where, pb.entityContext, vw.Attribute)
	}
	attrQN := declaring + "." + vw.Attribute

	var all []string
	switch t := pb.findAttributeType(attrQN).(type) {
	case *domainmodel.BooleanAttributeType:
		all = []string{"true", "false"}
	case *domainmodel.EnumerationAttributeType:
		values, err := pb.enumerationValueNames(t.EnumerationRef)
		if err != nil {
			return mdlerrors.NewValidationf("%s: %v", where, err)
		}
		all = append(values, emptyConditionValue)
	default:
		return mdlerrors.NewValidationf("%s: %s is not a Boolean or enumeration attribute — use an expression instead: `Visible: [...]`", where, attrQN)
	}

	visible := map[string]bool{}
	for _, v := range vw.Values {
		stored := v
		if strings.EqualFold(v, "empty") {
			stored = emptyConditionValue
		}
		idx := indexFold(all, stored)
		if idx < 0 {
			return mdlerrors.NewValidationf("%s: %q is not a value of %s (values: %s)", where, v, attrQN,
				strings.Join(mdlValueNames(all), ", "))
		}
		visible[all[idx]] = true
	}

	conds := make([]pages.ValueCondition, 0, len(all))
	for _, v := range all {
		conds = append(conds, pages.ValueCondition{Value: v, Visible: visible[v]})
	}
	bwg.GetBaseWidget().ConditionalVisibility = &pages.ConditionalVisibilitySettings{
		BaseElement: model.BaseElement{
			ID:       model.ID(types.GenerateID()),
			TypeName: "Forms$ConditionalVisibilitySettings",
		},
		Attribute:  attrQN,
		Conditions: conds,
	}
	return nil
}

func indexFold(values []string, v string) int {
	for i, x := range values {
		if strings.EqualFold(x, v) {
			return i
		}
	}
	return -1
}

// mdlValueNames lists condition values as MDL spells them.
func mdlValueNames(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		if v == emptyConditionValue {
			v = "empty"
		}
		out[i] = v
	}
	return out
}

// enumerationValueNames returns an enumeration's value names in declaration
// order.
func (pb *pageBuilder) enumerationValueNames(enumQN string) ([]string, error) {
	enums, err := pb.getEnumerations()
	if err != nil {
		return nil, err
	}
	h, err := pb.getHierarchy()
	if err != nil {
		return nil, err
	}
	for _, e := range enums {
		if h.GetModuleName(h.FindModuleID(e.ContainerID))+"."+e.Name != enumQN {
			continue
		}
		names := make([]string, 0, len(e.Values))
		for _, v := range e.Values {
			names = append(names, v.Name)
		}
		return names, nil
	}
	return nil, fmt.Errorf("enumeration %s not found", enumQN)
}

// getEnumerations returns cached enumerations or loads them.
func (pb *pageBuilder) getEnumerations() ([]*model.Enumeration, error) {
	if pb.execCache != nil && pb.execCache.enumerations != nil {
		return pb.execCache.enumerations, nil
	}
	if pb.backend == nil {
		return nil, fmt.Errorf("no project loaded")
	}
	enums, err := pb.backend.ListEnumerations()
	if err != nil {
		return nil, err
	}
	if pb.execCache != nil {
		pb.execCache.enumerations = enums
	}
	return enums, nil
}
