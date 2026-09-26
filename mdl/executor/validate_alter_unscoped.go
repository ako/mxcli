// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// A bare binding in a widget that ALTER PAGE / ALTER SNIPPET inserts where no
// entity is in scope passed `check --references` and was only caught — or not
// caught — at exec.
//
// The builder qualifies a bare `Attribute: X` or `{1} = X` with the entity of
// the insertion point, which exec reads from the STORED document: the nearest
// enclosing data source, or for a flow source the flow's return type. When there
// is none — the flow is missing, as FeedbackModule.DS_FeedbackForm is from
// Feedback v4.0.2's ShareFeedback_Logo, or the widget goes outside every data
// container — the binding is written bare. Measured on Mendix 11.13.0: an image
// URL parameter written that way left a project `mx check` could not LOAD
// (ArgumentNullException setting 'Attribute'); a text box's `Attribute:` was
// silently dropped and the build failed with CE7005 "No value selection has
// been made".
//
// CREATE PAGE catches the same shape at check time (relaxExcludedWidgetRefs,
// via unscopedBindings). ALTER could not, because its scope is not in the
// statement: it is in the document. So this pass opens the stored document and
// asks it the question exec asks, through the same function (alterEntityContext)
// — check and exec cannot disagree about which entity is in scope.

// validateAlterUnscopedBindings reports the bare bindings of widgets an
// INSERT or REPLACE places where the stored document puts no entity in scope.
func validateAlterUnscopedBindings(ctx *ExecContext, prog *ast.Program, sc *scriptContext) []error {
	if prog == nil || !ctx.Connected() {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return nil
	}
	opened := map[model.ID]backend.PageMutator{}

	var errs []error
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.AlterPageStmt)
		if !ok || !hasWidgetBuildingOp(s) || alterTargetComesFromScript(sc, s) {
			continue
		}
		unitID, containerID, containerType, err := resolveAlterPageUnit(ctx, s, h)
		if err != nil {
			continue // validateAlterTarget's finding
		}
		mutator, seen := opened[unitID]
		if !seen {
			// Read-only: the mutator is never saved. A document that will not
			// open is silence, never a finding.
			mutator, _ = ctx.Backend.OpenPageForMutation(unitID)
			opened[unitID] = mutator
		}
		if mutator == nil {
			continue
		}
		modName := h.GetModuleName(containerID)
		label := fmt.Sprintf("alter %s %s", containerType, s.PageName.String())
		for _, op := range s.Operations {
			var target ast.WidgetRef
			var widgets []*ast.WidgetV3
			var into bool
			switch o := op.(type) {
			case *ast.InsertWidgetOp:
				target, widgets, into = o.Target, o.Widgets, strings.EqualFold(o.Position, "INTO")
				if allListViewTemplates(widgets) {
					continue // built against the list view's own entity, on its own path
				}
			case *ast.ReplaceWidgetOp:
				target, widgets = o.Target, o.NewWidgets
			default:
				continue
			}
			// DataGrid 2 columns take their own path, scoped by the grid.
			if target.IsColumn() && allColumns(widgets) {
				continue
			}
			// A target the stored document lacks is either added earlier in the
			// script or refused by exec as not found; neither is this finding.
			if !mutator.FindWidget(target.Widget) {
				continue
			}
			if msg := unscopedInsertion(ctx, sc, mutator, target.Widget, into, modName, containerID, widgets); msg != "" {
				errs = append(errs, mdlerrors.NewValidation(fmt.Sprintf("%s: %s", label, msg)))
			}
		}
	}
	return errs
}

// unscopedInsertion returns the finding for one INSERT/REPLACE, or "".
func unscopedInsertion(ctx *ExecContext, sc *scriptContext, mutator backend.PageMutator, target string, into bool,
	modName string, modID model.ID, widgets []*ast.WidgetV3) string {
	entity, flow := alterEntityContext(ctx, mutator, target, into, modName, modID)
	if entity != "" {
		return ""
	}
	if flow != "" && sc != nil {
		// A flow the script creates is not in the project yet; its declared
		// return type is what exec will resolve against.
		if sig, ok := sc.flowParams[strings.ToLower(flow)]; ok {
			if sig.Returns != "" {
				return ""
			}
		}
	}
	bare := bindingsWithoutScope(widgets)
	if len(bare) == 0 {
		return ""
	}
	where := fmt.Sprintf("the insertion point (`%s`) is in no data container", target)
	if flow != "" {
		where = fmt.Sprintf("the data container around `%s` is sourced by %s, which does not exist or returns no entity", target, flow)
	}
	return fmt.Sprintf("%s, so no entity is in scope and these bindings cannot be qualified: %s. "+
		"Written without an entity, a template parameter is stored bare (Mendix can no longer load the "+
		"project) and an attribute binding is dropped (CE7005). Qualify them (Module.Entity.Attribute), "+
		"or insert into a data container whose entity is known.",
		where, strings.Join(bare, ", "))
}

// hasWidgetBuildingOp reports whether a statement carries an INSERT or REPLACE,
// so a script of pure SETs and DROPs never opens a document for this pass.
func hasWidgetBuildingOp(s *ast.AlterPageStmt) bool {
	for _, op := range s.Operations {
		switch op.(type) {
		case *ast.InsertWidgetOp, *ast.ReplaceWidgetOp:
			return true
		}
	}
	return false
}
