// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// glyphRanges is every code point the Mendix glyph font (Glyphicons Halflings,
// shipped by Atlas_Core as glyphicons-halflings-regular.woff) actually defines:
// 247 codes in 35 runs, extracted from that font's cmap.
//
// It is a literal table rather than a read of the project's font because the
// check has to run in the project-free pass — which is how CI runs `mxcli check`
// — and because the font is a fixed Mendix asset, not something a project edits.
var glyphRanges = [][2]int{
	{0xE001, 0xE003}, {0xE005, 0xE009}, {0xE010, 0xE019}, {0xE020, 0xE029},
	{0xE030, 0xE039}, {0xE040, 0xE049}, {0xE050, 0xE059}, {0xE060, 0xE060},
	{0xE062, 0xE069}, {0xE070, 0xE079}, {0xE080, 0xE089}, {0xE090, 0xE097},
	{0xE101, 0xE109}, {0xE110, 0xE119}, {0xE120, 0xE129}, {0xE130, 0xE139},
	{0xE140, 0xE146}, {0xE148, 0xE149}, {0xE150, 0xE159}, {0xE160, 0xE169},
	{0xE170, 0xE179}, {0xE180, 0xE189}, {0xE190, 0xE195}, {0xE197, 0xE199},
	{0xE200, 0xE206}, {0xE209, 0xE209}, {0xE210, 0xE216}, {0xE218, 0xE219},
	{0xE221, 0xE221}, {0xE223, 0xE227}, {0xE230, 0xE239}, {0xE240, 0xE249},
	{0xE250, 0xE259}, {0xE260, 0xE260}, {0xF8FF, 0xF8FF},
}

// glyphCodeDefined reports whether the Mendix glyph font defines this code.
func glyphCodeDefined(code int) bool {
	i := sort.Search(len(glyphRanges), func(i int) bool { return glyphRanges[i][1] >= code })
	return i < len(glyphRanges) && code >= glyphRanges[i][0]
}

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
						"`describe icon collection Atlas_Core.Atlas`). To keep a glyph, use a code the " +
						"font defines — 57345-57495, 57601-57753, 57856-57952 (with gaps), or 63743.",
				})
			}
			walk(item.Items)
		}
	}
	walk(items)
	return out
}
