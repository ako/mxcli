// SPDX-License-Identifier: Apache-2.0

// Package executor - Icon collection commands (SHOW / DESCRIBE ICON COLLECTION).
// Icon collections (CustomIcons$CustomIconCollection, e.g. Atlas_Core.Atlas_Filled)
// are read-only in mxcli — they ship with the theme/Atlas. Their icons are
// referenced from a widget as `Module.CollectionName.IconName` (a button's
// `icon:` property), so these commands exist to discover valid icon names.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// listIconCollections handles SHOW ICON COLLECTION [IN module].
func listIconCollections(ctx *ExecContext, moduleName string) error {
	collections, err := ctx.Backend.ListIconCollections()
	if err != nil {
		return mdlerrors.NewBackend("list icon collections", err)
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}

	result := &TableResult{
		Columns: []string{"Icon Collection", "Prefix", "Export Level", "Icons"},
	}
	for _, ic := range collections {
		modID := h.FindModuleID(ic.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}
		exportLevel := ic.ExportLevel
		if exportLevel == "" {
			exportLevel = "Hidden"
		}
		result.Rows = append(result.Rows, []any{
			fmt.Sprintf("%s.%s", modName, ic.Name), ic.Prefix, exportLevel, len(ic.Icons),
		})
	}
	result.Summary = fmt.Sprintf("(%d icon collection(s))", len(result.Rows))
	return writeResult(ctx, result)
}

// describeIconCollection handles DESCRIBE ICON COLLECTION Module.Name — lists the
// collection's icons and the reference form to use them on a widget.
func describeIconCollection(ctx *ExecContext, name ast.QualifiedName) error {
	ic := findIconCollection(ctx, name.Module, name.Name)
	if ic == nil {
		return mdlerrors.NewNotFound("icon collection", name.String())
	}

	h, err := getHierarchy(ctx)
	if err != nil {
		return err
	}
	modID := h.FindModuleID(ic.ContainerID)
	modName := h.GetModuleName(modID)
	qn := fmt.Sprintf("%s.%s", modName, ic.Name)

	fmt.Fprintf(ctx.Output, "-- Icon collection %s (%d icons, read-only)\n", qn, len(ic.Icons))
	fmt.Fprintf(ctx.Output, "-- Reference an icon on a widget (e.g. a button's `icon:`) as %s.<IconName>\n", qn)

	result := &TableResult{
		Columns: []string{"Icon", "Reference"},
		Summary: fmt.Sprintf("(%d icon(s))", len(ic.Icons)),
	}
	for _, item := range ic.Icons {
		result.Rows = append(result.Rows, []any{item.Name, qn + "." + item.Name})
	}
	return writeResult(ctx, result)
}

// findIconCollection finds an icon collection by module and name.
func findIconCollection(ctx *ExecContext, moduleName, collectionName string) *types.IconCollection {
	collections, err := ctx.Backend.ListIconCollections()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, ic := range collections {
		modID := h.FindModuleID(ic.ContainerID)
		if ic.Name == collectionName && h.GetModuleName(modID) == moduleName {
			return ic
		}
	}
	return nil
}
