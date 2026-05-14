// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
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
			if err := applySetPropertyMutator(mutator, o); err != nil {
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

func applySetPropertyMutator(mutator backend.PageMutator, op *ast.SetPropertyOp) error {
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
	default:
		return nil, mdlerrors.NewUnsupported("unsupported DataSource type for alter page set: " + ds.Type)
	}
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

	// Find entity context from enclosing DataView/DataGrid/ListView
	entityCtx := mutator.EnclosingEntity(op.Target.Widget)

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

	// Find entity context from enclosing DataView/DataGrid/ListView
	entityCtx := mutator.EnclosingEntity(op.Target.Widget)

	// Build new widgets from AST, excluding the target widget/column from the
	// duplicate-name scope so a same-name replacement is allowed.
	widgets, err := buildWidgetsFromAST(ctx, op.NewWidgets, moduleName, moduleID, entityCtx, mutator, op.Target.Widget, op.Target.Column)
	if err != nil {
		return mdlerrors.NewBackend("build replacement widgets", err)
	}

	return mutator.ReplaceWidget(op.Target.Widget, op.Target.Column, widgets)
}

// ============================================================================
// Widget building from AST (domain logic stays in executor)
// ============================================================================

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

// ============================================================================
// Static validation (used by mxcli check without project connection)
// ============================================================================

// ValidateAlterPage checks ALTER PAGE statements for patterns that produce
// invalid BSON and crash MxBuild without a useful error message.
func ValidateAlterPage(s *ast.AlterPageStmt) []linter.Violation {
	var violations []linter.Violation
	for _, op := range s.Operations {
		insert, ok := op.(*ast.InsertWidgetOp)
		if !ok {
			continue
		}
		for _, w := range insert.Widgets {
			if strings.EqualFold(w.Type, "column") && len(w.Children) > 0 {
				violations = append(violations, linter.Violation{
					RuleID:   "MDL-ALTPAGE001",
					Severity: linter.SeverityError,
					Message: fmt.Sprintf(
						"ALTER PAGE INSERT: column '%s' has nested widgets — "+
							"inserting a custom content column via ALTER PAGE produces invalid BSON "+
							"and crashes MxBuild (InvalidCastException: DivContainer → WidgetObject). "+
							"Use CREATE OR REPLACE PAGE to rebuild the page instead.",
						w.Name,
					),
				})
			}
		}
	}
	return violations
}
