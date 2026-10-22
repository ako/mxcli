// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// MDL-ASSOCDS02 — a page data source that follows a view entity's association
// from the entity it points at.
//
// A view entity's association (an `<alias>.ID as <Name>` column of its OQL) is
// one-way for a page: the persistent entity at its far end does not own it.
// mxbuild refuses a data source that follows it from there with CE8812
// "Entity 'M.Customer' has no association 'M.Stats_Customer'". `check` passed
// it and exec wrote it (mendixlabs/mxcli#1611 A).
//
// Measured on mxbuild 11.14.0, one page per shape:
//
//	DATABASE FROM $currentObject/M.Stats_Customer/M.Stats  list view, data grid, gallery → CE8812
//	$currentObject/M.Stats_Customer                        list view, data view           → CE8812
//	… the same hop second in a path (from an Item)                                        → CE8812
//	the same, followed from the view entity                                               → ok
//	DATABASE M.Stats WHERE [M.Stats_Customer = $currentObject]                            → ok
//	a microflow `retrieve $L from $Cust/M.Stats_Customer`                                 → ok (not judged)
//
// The walk follows each hop from the widget's context entity, so it judges only
// a path it can place: an unknown association or an entity it cannot follow
// ends the walk silently.
func (v *attributeScopeValidator) checkViewAssocReverse(w *ast.WidgetV3, enclosing dataContext) {
	if v.assocs == nil {
		return
	}
	ds := w.GetDataSource()
	if ds == nil {
		return
	}
	var path string
	switch {
	case ds.Type == "association":
		path = ds.Reference
	case ds.Type == "database" && ds.AssociationPath != "":
		path = ds.AssociationPath
	default:
		return
	}
	cur := v.dataSourceContextEntity(ds, enclosing)
	if cur == "" || path == "" {
		return
	}
	for _, seg := range strings.Split(path, "/") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return
		}
		qn := seg
		if !strings.Contains(qn, ".") {
			mod, _, _ := strings.Cut(cur, ".")
			qn = mod + "." + qn
		}
		shape, isAssoc := v.assocs[strings.ToLower(qn)]
		if !isAssoc {
			// An entity step (`/M.Stats` after the association) names where the
			// hop arrived; anything else ends what can be judged.
			if strings.EqualFold(qn, cur) {
				continue
			}
			return
		}
		switch {
		case strings.EqualFold(cur, shape.from):
			cur = shape.to
		case strings.EqualFold(cur, shape.to):
			if shape.view {
				v.errs = append(v.errs, fmt.Sprintf(
					"%s data source %s: %s is an association of view entity %s, and a page cannot follow it from %s — "+
						"mxbuild rejects this with CE8812 \"Entity '%s' has no association '%s'\". Query the view entity "+
						"instead: DATABASE %s WHERE [%s = $currentObject] [MDL-ASSOCDS02]",
					widgetLabel(w.Name, w.Type), dataSourceDisplay(ds), shape.qn, shape.from, cur,
					cur, shape.qn, shape.from, shape.qn))
				return
			}
			cur = shape.from
		default:
			return
		}
	}
}

// dataSourceContextEntity is the entity a context-relative data source starts
// from: the enclosing data container's for $currentObject, else the page or
// snippet parameter's. "" when it cannot be established.
func (v *attributeScopeValidator) dataSourceContextEntity(ds *ast.DataSourceV3, enclosing dataContext) string {
	switch cv := strings.ToLower(ds.ContextVariable); cv {
	case "", "currentobject":
		if enclosing.unresolved || len(enclosing.entities) == 0 {
			return ""
		}
		return enclosing.entities[len(enclosing.entities)-1]
	default:
		return v.pageParams[cv]
	}
}

func dataSourceDisplay(ds *ast.DataSourceV3) string {
	if ds.Type == "database" {
		return "DATABASE FROM $" + ds.ContextVariable + "/" + ds.AssociationPath
	}
	return dataSourceText(ds)
}
