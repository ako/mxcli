// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// describeMenu renders a standalone Menus$MenuDocument — the reusable menu a
// menu widget points at, as opposed to the menu embedded in a navigation
// profile. Atlas_Core ships Phone_Menu and Tablet_Menu.
//
// The entries are ordinary Menus$MenuItem elements, so the tree is rendered by
// printMenuMDL, the same renderer DESCRIBE NAVIGATION uses — and the same syntax
// CREATE MENU accepts, so the output round-trips.
func describeMenu(ctx *ExecContext, name ast.QualifiedName) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	md, err := ctx.Backend.GetMenuDocumentByQualifiedName(name.Module, name.Name)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return mdlerrors.NewNotFound("menu", name.String())
		}
		return mdlerrors.NewBackend("get menu", err)
	}

	if md.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n",
			strings.ReplaceAll(md.Documentation, "\n", "\n * "))
	}

	if md.Excluded {
		fmt.Fprintln(ctx.Output, "-- Excluded from the project")
	}

	// Output is re-executable: the item syntax is the same one CREATE MENU
	// accepts, so describe → exec → describe is a fixed point.
	fmt.Fprintf(ctx.Output, "create or modify menu %s.%s%s (\n", name.Module, md.Name, describeFolderClause(ctx, md.ContainerID))
	printMenuMDL(ctx.Output, md.Items, 1, "CREATE MENU")
	fmt.Fprintln(ctx.Output, ");")
	return nil
}

// execCreateMenu handles CREATE [OR MODIFY] MENU Module.Name ( items ).
//
// Like CREATE NAVIGATION, the item list is the document's complete contents, so
// a modify replaces the items wholesale rather than merging. The existing
// document's $ID is reused on modify, so references to the menu survive and the
// unit is rewritten in place rather than replaced.
func execCreateMenu(ctx *ExecContext, s *ast.CreateMenuStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	mod, err := ctx.Backend.GetModuleByName(s.Name.Module)
	if err != nil || mod == nil {
		return mdlerrors.NewNotFound("module", s.Name.Module)
	}

	existing, _ := ctx.Backend.GetMenuDocumentByQualifiedName(s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("menu", s.Name.String())
	}

	var existingContainer model.ID
	if existing != nil {
		existingContainer = existing.ContainerID
	}
	containerID, err := containerForDocument(ctx, mod.ID, s.Folder, existingContainer)
	if err != nil {
		return err
	}

	md := &types.MenuDocument{
		Name:          s.Name.Name,
		ContainerID:   containerID,
		Documentation: s.Documentation,
		Items:         menuItemsFromAST(s.Items),
	}
	// A rewrite that carried no doc comment keeps the stored one (#1018).
	if existing != nil {
		md.Documentation = carriedDocumentation(s.DocumentationSet, s.Documentation, existing.Documentation)
	}

	if existing != nil {
		// Preserve the document's identity and the properties MDL does not
		// author, so a modify does not silently reset them.
		md.ID = existing.ID
		md.ExportLevel = existing.ExportLevel
		md.Excluded = existing.Excluded
		if md.Documentation == "" {
			md.Documentation = existing.Documentation
		}
		if err := ctx.Backend.UpdateMenuDocument(md); err != nil {
			return mdlerrors.NewBackend("update menu", err)
		}
		if _, err := applyDocumentFolder(ctx, md.ID, existingContainer, containerID); err != nil {
			return err
		}
		ctx.ReportMutation("Modified", "menu %s", s.Name.String())
		return nil
	}

	if err := ctx.Backend.CreateMenuDocument(md); err != nil {
		return mdlerrors.NewBackend("create menu", err)
	}
	fmt.Fprintf(ctx.Output, "Created menu %s\n", s.Name.String())
	return nil
}

// execDropMenu handles DROP MENU Module.Name.
func execDropMenu(ctx *ExecContext, s *ast.DropMenuStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	md, err := ctx.Backend.GetMenuDocumentByQualifiedName(s.Name.Module, s.Name.Name)
	if err != nil || md == nil {
		return mdlerrors.NewNotFound("menu", s.Name.String())
	}
	if err := ctx.Backend.DeleteMenuDocument(md.ID); err != nil {
		return mdlerrors.NewBackend("drop menu", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped menu %s\n", s.Name.String())
	return nil
}

// menuItemsFromAST converts parsed menu items to the semantic model. The AST and
// semantic shapes differ only in how the target is held (pointer vs string), so
// this stays a direct mapping rather than acquiring behaviour.
func menuItemsFromAST(defs []ast.NavMenuItemDef) []*types.NavMenuItem {
	var out []*types.NavMenuItem
	for _, d := range defs {
		item := &types.NavMenuItem{Caption: d.Caption, Icon: d.Icon}
		if d.Icon != "" {
			item.IconType = "Forms$IconCollectionIcon"
		}
		if d.Page != nil {
			item.Page = d.Page.String()
			item.ActionType = "PageAction"
		} else if d.Microflow != nil {
			item.Microflow = d.Microflow.String()
			item.ActionType = "MicroflowAction"
		} else {
			item.ActionType = "NoAction"
		}
		item.Items = menuItemsFromAST(d.Items)
		out = append(out, item)
	}
	return out
}
