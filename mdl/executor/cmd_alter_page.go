// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// execAlterPage handles ALTER PAGE/SNIPPET Module.Name { operations }.
func execAlterPage(ctx *ExecContext, s *ast.AlterPageStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	var unitID model.ID
	var containerID model.ID
	containerType := strings.ToLower(s.ContainerType)
	if containerType == "" {
		containerType = "page"
	}

	if containerType == "snippet" {
		snippet, modID, err := findSnippetByName(ctx, s.PageName, h)
		if err != nil {
			return err
		}
		unitID = snippet.ID
		containerID = modID
	} else {
		page, err := findPageByName(ctx, s.PageName, h)
		if err != nil {
			return err
		}
		unitID = page.ID
		containerID = h.FindModuleID(page.ContainerID)
	}

	// Open the page for mutation via the backend
	mutator, err := ctx.Backend.OpenPageForMutation(unitID)
	if err != nil {
		return mdlerrors.NewBackend("open "+strings.ToLower(containerType)+" for mutation", err)
	}

	// Resolve module name for building new widgets
	modName := h.GetModuleName(containerID)

	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.SetPropertyOp:
			if err := applySetPropertyMutator(ctx, mutator, o, modName, containerID); err != nil {
				return mdlerrors.NewBackend("set", err)
			}
		case *ast.InsertWidgetOp:
			if err := applyInsertWidgetMutator(ctx, mutator, o, modName, containerID); err != nil {
				return mdlerrors.NewBackend("insert", err)
			}
		case *ast.DropWidgetOp:
			if err := applyDropWidgetMutator(mutator, o); err != nil {
				return mdlerrors.NewBackend("drop", err)
			}
		case *ast.ReplaceWidgetOp:
			if err := applyReplaceWidgetMutator(ctx, mutator, o, modName, containerID); err != nil {
				return mdlerrors.NewBackend("replace", err)
			}
		case *ast.AddVariableOp:
			if err := mutator.AddVariable(o.Variable.Name, o.Variable.DataType, o.Variable.DefaultValue); err != nil {
				return mdlerrors.NewBackend("add VARIABLE", err)
			}
		case *ast.DropVariableOp:
			if err := mutator.DropVariable(o.VariableName); err != nil {
				return mdlerrors.NewBackend("drop VARIABLE", err)
			}
		case *ast.SetLayoutOp:
			if containerType == "snippet" {
				return mdlerrors.NewUnsupported("set Layout is not supported for snippets")
			}
			newLayoutQN := o.NewLayout.Module + "." + o.NewLayout.Name
			if err := mutator.SetLayout(newLayoutQN, o.Mappings); err != nil {
				return mdlerrors.NewBackend("set Layout", err)
			}
		default:
			return mdlerrors.NewUnsupported(fmt.Sprintf("unknown alter %s operation type: %T", containerType, op))
		}
	}

	// Persist
	if err := mutator.Save(); err != nil {
		return mdlerrors.NewBackend("save modified "+strings.ToLower(containerType), err)
	}

	fmt.Fprintf(ctx.Output, "Altered %s %s\n", strings.ToLower(containerType), s.PageName.String())
	return nil
}

// ============================================================================
// SET property via mutator
// ============================================================================

func applySetPropertyMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.SetPropertyOp, moduleName string, moduleID model.ID) error {
	// Sort property names for deterministic application order.
	propNames := make([]string, 0, len(op.Properties))
	for k := range op.Properties {
		propNames = append(propNames, k)
	}
	sort.Strings(propNames)

	for _, propName := range propNames {
		value := op.Properties[propName]
		if op.Target.IsColumn() {
			if err := mutator.SetColumnProperty(op.Target.Widget, op.Target.Column, propName, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		} else if propName == "DataSource" {
			// DataSource requires special handling via SetWidgetDataSource
			ds, err := convertASTDataSource(value)
			if err != nil {
				return err
			}
			if err := mutator.SetWidgetDataSource(op.Target.Widget, ds); err != nil {
				return mdlerrors.NewBackend("set DataSource on "+op.Target.Name(), err)
			}
		} else if propName == "Action" {
			// Action is a polymorphic node, not a scalar — it goes through the
			// same builder CREATE PAGE uses rather than being written as a value.
			action, err := convertASTAction(ctx, value, moduleName, moduleID)
			if err != nil {
				return err
			}
			if err := mutator.SetWidgetAction(op.Target.Widget, action); err != nil {
				return mdlerrors.NewBackend("set Action on "+op.Target.Name(), err)
			}
		} else {
			if err := mutator.SetWidgetProperty(op.Target.Widget, propName, value); err != nil {
				return mdlerrors.NewBackend("set "+propName+" on "+op.Target.Name(), err)
			}
		}
	}
	return nil
}

// convertASTDataSource converts an AST DataSource value to a pages.DataSource.
func convertASTDataSource(value interface{}) (pages.DataSource, error) {
	ds, ok := value.(*ast.DataSourceV3)
	if !ok {
		return nil, mdlerrors.NewValidation("DataSource value must be a datasource expression")
	}

	switch ds.Type {
	case "selection":
		return &pages.ListenToWidgetSource{WidgetName: ds.Reference}, nil
	case "database":
		return &pages.DatabaseSource{EntityName: ds.Reference}, nil
	case "microflow":
		return &pages.MicroflowSource{Microflow: ds.Reference}, nil
	case "nanoflow":
		return &pages.NanoflowSource{Nanoflow: ds.Reference}, nil
	case "parameter":
		// "Data from context". Only the parameter name is known here; the entity
		// it is typed with lives in the container's own Parameters list, which
		// the mutator holds — it resolves and validates the name (#855).
		return &pages.DataViewSource{ParameterName: strings.TrimPrefix(ds.Reference, "$")}, nil
	default:
		// Name the way out. The REPLACE path rebuilds the widget through the
		// CREATE PAGE builder, which handles every datasource type, so an
		// unsupported SET is an inconvenience rather than a dead end — saying so
		// beats leaving the author to discover it (#855).
		return nil, mdlerrors.NewUnsupported(fmt.Sprintf(
			"unsupported DataSource type for alter page set: %s — "+
				"use `replace <widget> with …` instead, which rebuilds the widget through the "+
				"CREATE PAGE path and supports every datasource type", ds.Type))
	}
}

// convertASTAction converts an AST action value to a pages.ClientAction.
//
// It delegates to the CREATE PAGE builder rather than reimplementing the switch,
// so every action form is supported here the day it is supported there — the
// alternative is the failure mode #855 documents for DataSource, where SET
// carried a narrower vocabulary than REPLACE and each missing case surfaced as
// its own bug report.
func convertASTAction(ctx *ExecContext, value any, moduleName string, moduleID model.ID) (pages.ClientAction, error) {
	action, ok := value.(*ast.ActionV3)
	if !ok {
		return nil, mdlerrors.NewValidation("Action value must be an action expression, " +
			"for example `set Action = microflow Module.MF on btnSave`")
	}
	pb := &pageBuilder{
		ctx:           ctx,
		backend:       ctx.Backend,
		moduleID:      moduleID,
		moduleName:    moduleName,
		execCache:     ctx.Cache,
		fragments:     ctx.Fragments,
		themeRegistry: ctx.GetThemeRegistry(),
		widgetBackend: ctx.Backend,
	}
	return pb.buildClientActionV3(action)
}

// ============================================================================
// INSERT widget via mutator
// ============================================================================

func applyInsertWidgetMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.InsertWidgetOp, moduleName string, moduleID model.ID) error {
	// Check for duplicate widget names before building
	for _, w := range op.Widgets {
		if w.Name != "" && mutator.FindWidget(w.Name) {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Special path: inserting columns into a DataGrid2 column. Columns are
	// CustomWidgets$WidgetObject, not Forms$* widgets, so we go through
	// InsertColumns to serialize them correctly. Column children inherit the
	// grid's data source entity as their entity context.
	if op.Target.IsColumn() && allColumns(op.Widgets) {
		entityCtx := mutator.EnclosingEntityForChildren(op.Target.Widget)
		specs, err := buildColumnSpecsFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
		if err != nil {
			return mdlerrors.NewBackend("build column specs", err)
		}
		return mutator.InsertColumns(op.Target.Widget, op.Target.Column, backend.InsertPosition(op.Position), specs)
	}

	// Find entity context for the new widgets. For INSERT BEFORE/AFTER the target
	// is a sibling, so use its enclosing container's context; for INSERT INTO the
	// target IS the container, so the children take the target's own context (e.g.
	// a dataview's entity).
	into := strings.EqualFold(op.Position, "INTO")
	entityCtx := mutator.EnclosingEntity(op.Target.Widget)
	if into {
		entityCtx = mutator.EnclosingEntityForChildren(op.Target.Widget)
	}
	// A microflow/nanoflow datasource contributes no entity to the BSON walk (its
	// entity is the flow's RETURN type), so resolve it via the model — otherwise a
	// widget inserted into a flow-sourced list binds nothing (CE0402/CE1613). (#55)
	if entityCtx == "" {
		mfQN, nfQN := mutator.EnclosingDataSourceFlow(op.Target.Widget, into)
		if e := resolveDataSourceFlowEntity(ctx, moduleName, moduleID, mfQN, nfQN); e != "" {
			entityCtx = e
		}
	}

	// Build new widgets from AST
	widgets, err := buildWidgetsFromAST(ctx, op.Widgets, moduleName, moduleID, entityCtx, mutator)
	if err != nil {
		return mdlerrors.NewBackend("build widgets", err)
	}

	return mutator.InsertWidget(op.Target.Widget, op.Target.Column, backend.InsertPosition(op.Position), widgets)
}

// ============================================================================
// DROP widget via mutator
// ============================================================================

func applyDropWidgetMutator(mutator backend.PageMutator, op *ast.DropWidgetOp) error {
	refs := make([]backend.WidgetRef, len(op.Targets))
	for i, t := range op.Targets {
		refs[i] = backend.WidgetRef{Widget: t.Widget, Column: t.Column}
	}
	return mutator.DropWidget(refs)
}

// ============================================================================
// REPLACE widget via mutator
// ============================================================================

func applyReplaceWidgetMutator(ctx *ExecContext, mutator backend.PageMutator, op *ast.ReplaceWidgetOp, moduleName string, moduleID model.ID) error {
	// Check for duplicate widget names (skip the widget being replaced)
	for _, w := range op.NewWidgets {
		if w.Name != "" && w.Name != op.Target.Widget && w.Name != op.Target.Column && mutator.FindWidget(w.Name) {
			return mdlerrors.NewAlreadyExistsMsg("widget", w.Name, fmt.Sprintf("duplicate widget name '%s': a widget with this name already exists on the page", w.Name))
		}
	}

	// Special path: replacing a DataGrid2 column with new columns. Columns are
	// CustomWidgets$WidgetObject, not Forms$* widgets. Column children inherit
	// the grid's data source entity as their entity context.
	if op.Target.IsColumn() && allColumns(op.NewWidgets) {
		entityCtx := mutator.EnclosingEntityForChildren(op.Target.Widget)
		specs, err := buildColumnSpecsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator, op.Target.Widget, op.Target.Column)
		if err != nil {
			return mdlerrors.NewBackend("build replacement column specs", err)
		}
		return mutator.ReplaceColumn(op.Target.Widget, op.Target.Column, specs)
	}

	// Find entity context from enclosing DataView/DataGrid/ListView for regular widget replace.
	entityCtx := mutator.EnclosingEntity(op.Target.Widget)
	// Resolve a microflow/nanoflow datasource's return entity (see the INSERT path).
	if entityCtx == "" {
		mfQN, nfQN := mutator.EnclosingDataSourceFlow(op.Target.Widget, false)
		if e := resolveDataSourceFlowEntity(ctx, moduleName, moduleID, mfQN, nfQN); e != "" {
			entityCtx = e
		}
	}

	// Build new widgets from AST, excluding the target widget/column from the
	// duplicate-name scope so a same-name replacement is allowed.
	widgets, err := buildWidgetsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator, op.Target.Widget, op.Target.Column)
	if err != nil {
		return mdlerrors.NewBackend("build replacement widgets", err)
	}

	return mutator.ReplaceWidget(op.Target.Widget, op.Target.Column, widgets)
}

// allColumns returns true if all widgets in the slice have type "column".
// Used to dispatch ALTER PAGE INSERT/REPLACE into a DataGrid2 column to the
// column-specific mutator path.
func allColumns(widgets []*ast.WidgetV3) bool {
	if len(widgets) == 0 {
		return false
	}
	for _, w := range widgets {
		if !strings.EqualFold(w.Type, "column") {
			return false
		}
	}
	return true
}

// buildColumnSpecsFromAST converts AST column widgets to DataGridColumnSpec
// instances using the executor's pageBuilder for attribute path resolution
// and child widget construction.
func buildColumnSpecsFromAST(ctx *ExecContext, widgets []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator, excludeFromScope ...string) ([]*backend.DataGridColumnSpec, error) {
	paramScope, paramEntityNames := mutator.ParamScope()
	widgetScope := mutator.WidgetScope()
	for _, name := range excludeFromScope {
		delete(widgetScope, name)
	}

	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       moduleName,
		entityContext:    entityContext,
		widgetScope:      widgetScope,
		paramScope:       paramScope,
		paramEntityNames: paramEntityNames,
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
	}

	var result []*backend.DataGridColumnSpec
	for _, w := range widgets {
		spec, err := pb.buildColumnSpecFromAST(w)
		if err != nil {
			return nil, mdlerrors.NewBackend("build column "+w.Name, err)
		}
		result = append(result, spec)
	}
	return result, nil
}

// ============================================================================
// Widget building from AST (domain logic stays in executor)
// ============================================================================

// resolveDataSourceFlowEntity resolves the entity context contributed by a
// microflow/nanoflow datasource — its RETURN entity — for ALTER PAGE widget
// builds. A flow datasource stores no entity in its own BSON (the entity lives
// in the flow document), so the BSON walk yields "" and a bare inserted
// attribute would bind nothing. Returns "" when neither flow qualified name
// resolves (e.g. a void-returning flow). (FINDINGS #55)
func resolveDataSourceFlowEntity(ctx *ExecContext, moduleName string, moduleID model.ID, mfQN, nfQN string) string {
	if mfQN == "" && nfQN == "" {
		return ""
	}
	pb := &pageBuilder{
		ctx:        ctx,
		backend:    ctx.Backend,
		moduleID:   moduleID,
		moduleName: moduleName,
		execCache:  ctx.Cache,
	}
	if mfQN != "" {
		return pb.getMicroflowReturnEntityName(mfQN)
	}
	return pb.getNanoflowReturnEntityName(nfQN)
}

// buildWidgetsFromAST converts AST widgets to pages.Widget domain objects.
// It uses the mutator for scope resolution (WidgetScope, ParamScope).
// excludeFromScope removes named widgets from the duplicate-detection scope,
// used when replacing a widget so the new one may reuse the target's name.
func buildWidgetsFromAST(ctx *ExecContext, widgets []*ast.WidgetV3, moduleName string, moduleID model.ID, entityContext string, mutator backend.PageMutator, excludeFromScope ...string) ([]pages.Widget, error) {
	paramScope, paramEntityNames := mutator.ParamScope()
	widgetScope := mutator.WidgetScope()
	for _, name := range excludeFromScope {
		delete(widgetScope, name)
	}

	pb := &pageBuilder{
		ctx:              ctx,
		backend:          ctx.Backend,
		moduleID:         moduleID,
		moduleName:       moduleName,
		entityContext:    entityContext,
		widgetScope:      widgetScope,
		paramScope:       paramScope,
		paramEntityNames: paramEntityNames,
		execCache:        ctx.Cache,
		fragments:        ctx.Fragments,
		themeRegistry:    ctx.GetThemeRegistry(),
		widgetBackend:    ctx.Backend,
	}

	var result []pages.Widget
	for _, w := range widgets {
		widget, err := pb.buildWidgetV3(w)
		if err != nil {
			return nil, mdlerrors.NewBackend("build widget "+w.Name, err)
		}
		if widget == nil {
			continue
		}
		result = append(result, widget)
	}
	return result, nil
}
