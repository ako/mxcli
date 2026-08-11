// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// describeMenu renders a standalone Menus$MenuDocument — the reusable menu a
// menu widget points at, as opposed to the menu embedded in a navigation
// profile. Atlas_Core ships Phone_Menu and Tablet_Menu.
//
// The entries are ordinary Menus$MenuItem elements, so the tree is rendered by
// printMenuMDL, the same renderer DESCRIBE NAVIGATION uses. Output is
// deliberately not re-executable: Mendix offers no way to author a menu document
// outside Studio Pro, so there is no CREATE MENU for it to round-trip into. That
// is stated in the header rather than left for the reader to discover, following
// the DESCRIBE BUILDING BLOCK precedent.
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

	fmt.Fprintf(ctx.Output, "-- Menu: %s.%s (%d top-level item(s))\n",
		name.Module, md.Name, len(md.Items))
	if md.ExportLevel != "" {
		fmt.Fprintf(ctx.Output, "-- Export level: %s\n", md.ExportLevel)
	}
	if md.Excluded {
		fmt.Fprintln(ctx.Output, "-- Excluded from the project")
	}
	fmt.Fprintln(ctx.Output, "-- Menus are read-only; they cannot be created via MDL.")

	if len(md.Items) == 0 {
		fmt.Fprintln(ctx.Output, "{ }")
		return nil
	}

	fmt.Fprintln(ctx.Output, "{")
	printMenuMDL(ctx.Output, md.Items, 1, "MDL")
	fmt.Fprintln(ctx.Output, "}")
	return nil
}
