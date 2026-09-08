// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateMenuItemGlyphCodes (MDL078) flags `icon glyph <n>` with a code the
// Mendix glyph font does not define.
//
// # Why this needs its own rule
//
// A glyph code is a bare integer: nothing resolves it, so it passes `mxcli check`
// AND `mx check` at 0 errors, and fails only at `mxbuild --target=deploy` — which
// is what `mxcli run --local` does. The message it fails with names a document
// that is not the problem:
//
//	ERROR: One or more errors occurred.
//	(An exception occurred while exporting layout 'CapTrack.App_Default.')
//
// Neither the navigation nor the menu item is mentioned. Bisecting it cost three
// build cycles (ako/CapTrackV4 007, R2).
//
// # The mechanism, and why a code-point table is the right check
//
// mxbuild resolves the code through GlyphFont.GetClass(Int32), which is a LINQ
// `.First(...)` over its glyph table and throws `InvalidOperationException:
// Sequence contains no matching element` when the code is absent. Measured on
// 11.14.0 with two builds: 57562 (0xEBDA, past the font's 0xE260 end) produces
// exactly that stack, and 57377 (0xE021, in the font) exports pages and layouts
// cleanly. The font's cmap and mxbuild's table agree on both points.
//
// # A warning, not an error
//
// The table is a snapshot of a Mendix asset. If Mendix ever extends the font,
// a correct code would be reported here, and refusing it would be worse than the
// gap this closes — so `exec` still writes it and the author still gets told.
//
// # It needs no project
//
// The code is in the script. This runs in the project-free pass alongside
// MDL077, which is how CI reaches it.
func validateMenuItemGlyphCodes(stmt ast.Statement) []linter.Violation {
	var items []ast.NavMenuItemDef
	var where string

	switch s := stmt.(type) {
	case *ast.AlterNavigationStmt:
		items, where = s.MenuItems, "navigation "+s.ProfileName
	case *ast.CreateMenuStmt:
		items, where = s.Items, "menu "+s.Name.String()
	default:
		return nil
	}

	var out []linter.Violation
	var walk func(list []ast.NavMenuItemDef)
	walk = func(list []ast.NavMenuItemDef) {
		for _, item := range list {
			if item.IconKind == types.MenuIconGlyph && !glyphCodeDefined(item.IconCode) {
				out = append(out, linter.Violation{
					RuleID:   "MDL078",
					Severity: linter.SeverityWarning,
					Message: fmt.Sprintf(
						"%s: menu item %q uses `icon glyph %d`, which the Mendix glyph font does not "+
							"define — the build fails at `mxbuild --target=deploy` with "+
							"\"An exception occurred while exporting layout '<some layout>'\", naming a "+
							"document that is not the cause",
						where, item.Caption, item.IconCode),
					Suggestion: "prefer an icon collection reference, which IS resolved before anything " +
						"is written: `icon Atlas_Core.Atlas.<name>` (list them with " +
						"`describe icon collection Atlas_Core.Atlas`). To keep a glyph, pick a code from " +
						"`show glyphs` — the font's codes are sparse, so nearby numbers are usually " +
						"not defined either.",
				})
			}
			walk(item.Items)
		}
	}
	walk(items)
	return out
}
