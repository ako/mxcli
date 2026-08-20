// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestBuildingBlockDataSourceRebindTargetsUnboundWidgets guards the regression
// #941's emitter fix exposed.
//
// `use building block X (datasource: …)` finds its binding point by re-rendering
// the block to MDL and re-parsing it. It used to look for a widget that already
// carried a DataSource property, on the stated assumption that "a block's
// outermost list/grid/dataview always emits one". That was only true because of
// the bug #941 fixed: an unbound datasource — which is what a reusable Atlas
// block has, since it is a template — rendered as the malformed
// `DataSource: database from ,`. Emitting nothing for it, which is correct, left
// the rebind with nothing to match.
//
// So the target is matched by widget TYPE, exactly as the action override
// already does. That case learned this lesson first: its comment notes Atlas
// blocks "ship placeholder buttons with no action", which is the same fact about
// the same blocks.
func TestBuildingBlockDataSourceRebindTargetsUnboundWidgets(t *testing.T) {
	for _, widgetType := range []string{"gallery", "listview", "datagrid", "dataview", "DATAGRID2"} {
		t.Run(widgetType, func(t *testing.T) {
			if !isDataSourceWidget(&ast.WidgetV3{Type: widgetType}) {
				t.Errorf("%s is not recognised as a datasource-carrying widget", widgetType)
			}
		})
	}
	for _, widgetType := range []string{"container", "text", "actionbutton", "layoutgrid"} {
		t.Run("not/"+widgetType, func(t *testing.T) {
			if isDataSourceWidget(&ast.WidgetV3{Type: widgetType}) {
				t.Errorf("%s must not be treated as a datasource target", widgetType)
			}
		})
	}
}

// TestBuildingBlockRebindPrefersTheOutermost pins that the override still lands
// on the block's outermost datasource widget when the tree nests several — the
// binding-point rule the feature documents.
func TestBuildingBlockRebindPrefersTheOutermost(t *testing.T) {
	inner := &ast.WidgetV3{Type: "listview", Name: "inner"}
	outer := &ast.WidgetV3{Type: "gallery", Name: "outer", Children: []*ast.WidgetV3{inner}}
	var got string
	rebindFirst([]*ast.WidgetV3{outer}, isDataSourceWidget, func(w *ast.WidgetV3) { got = w.Name })
	if got != "outer" {
		t.Errorf("rebound %q, want the outermost datasource widget", got)
	}
}
