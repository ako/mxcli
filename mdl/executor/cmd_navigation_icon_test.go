// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// menuMDL renders items through the DESCRIBE emitter.
func menuMDL(items []*types.NavMenuItem) string {
	var b bytes.Buffer
	printMenuMDL(&b, items, 0, "CREATE NAVIGATION")
	return b.String()
}

// DESCRIBE emits re-executable MDL, so an icon it read must come back out — the
// alternative is output that silently rewrites the menu when replayed. The
// hyphenated segment has to be re-quoted: `align-center` lexes as HYPHENATED_ID,
// not IDENTIFIER, so an unquoted emission would be output the parser rejects.
func TestPrintMenuMDL_RoundTripsAnIconCollectionIcon(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{
		Caption:  "Dashboard",
		Page:     "M.Dash",
		Icon:     "Atlas_Core.Atlas.align-center",
		IconType: "Forms$IconCollectionIcon",
	}})
	want := "menu item 'Dashboard' page M.Dash icon Atlas_Core.Atlas.\"align-center\";\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A name a qualifiedName segment can hold is emitted bare, and quoting it
// anyway would be noise. `home` is a KEYWORD token, not an IDENTIFIER — and
// `identifierOrKeyword` accepts keywords, so it must NOT be quoted. Most short
// Atlas names are in this bucket (`home`, `user`, `add`, `folder`), which is why
// the emitter tests parser acceptance rather than reusing mdlIdent's
// lexes-as-IDENTIFIER rule.
func TestPrintMenuMDL_LeavesAKeywordIconNameUnquoted(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{
		Caption: "Home", Page: "M.Home",
		Icon: "Atlas_Core.Atlas.home", IconType: "Forms$IconCollectionIcon",
	}})
	want := "menu item 'Home' page M.Home icon Atlas_Core.Atlas.home;\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A sub-menu carries its icon before the parenthesised body, matching the
// grammar's second alternative.
func TestPrintMenuMDL_RoundTripsASubMenuIcon(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{
		Caption:  "Reports",
		Icon:     "Atlas_Core.Atlas.list-bullets",
		IconType: "Forms$IconCollectionIcon",
		Items:    []*types.NavMenuItem{{Caption: "Monthly", Page: "M.Monthly"}},
	}})
	if !strings.HasPrefix(got, "menu 'Reports' icon Atlas_Core.Atlas.\"list-bullets\" (\n") {
		t.Errorf("sub-menu header lost its icon: %q", got)
	}
	if !strings.Contains(got, "menu item 'Monthly' page M.Monthly;") {
		t.Errorf("sub-items went missing: %q", got)
	}
}

// The other two variants are real and appear in Studio Pro-authored projects,
// but CREATE NAVIGATION cannot write them. Emitting `icon '…'` for an ImageIcon
// would convert it to an IconCollectionIcon on replay — a silent variant swap.
// The loss has to be visible instead.
func TestPrintMenuMDL_FlagsAnIconItCannotReproduce(t *testing.T) {
	for _, tc := range []struct{ name, iconType, icon, wantIn string }{
		{"image icon", "Forms$ImageIcon", "System.Images.Close", "System.Images.Close"},
		{"glyph icon", "Forms$GlyphIcon", "", "a numeric glyph code"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := menuMDL([]*types.NavMenuItem{{
				Caption: "Close", Page: "M.Close", Icon: tc.icon, IconType: tc.iconType,
			}})
			// Check the statement line only — the note below it also says "icon".
			stmt := strings.SplitN(got, "\n", 2)[0]
			if strings.Contains(stmt, " icon ") {
				t.Errorf("emitted an ICON clause for %s, which replay would convert: %q",
					tc.iconType, got)
			}
			if !strings.Contains(got, "-- icon") || !strings.Contains(got, tc.wantIn) {
				t.Errorf("the unreproducible icon was dropped silently: %q", got)
			}
		})
	}
}

// No icon means no clause and no note — the common case must stay clean.
func TestPrintMenuMDL_SilentWhenThereIsNoIcon(t *testing.T) {
	got := menuMDL([]*types.NavMenuItem{{Caption: "Dashboard", Page: "M.Dash"}})
	if got != "menu item 'Dashboard' page M.Dash;\n" {
		t.Errorf("got %q", got)
	}
}

// The point of quoting is that DESCRIBE output re-executes. Assert that
// directly by feeding the emitted statement back through the parser, rather
// than trusting a hand-written expectation about which names need quotes —
// which is exactly the thing that was wrong twice while writing this.
func TestPrintMenuMDL_EmittedIconsReParse(t *testing.T) {
	for _, icon := range []string{
		"Atlas_Core.Atlas.align-center", // HYPHENATED_ID: needs quoting
		"Atlas_Core.Atlas.home",         // keyword token: accepted bare
		"Atlas_Core.Atlas.add",          // keyword token
		"Atlas_Core.Atlas_Filled.user",  // keyword token, non-default collection
	} {
		t.Run(icon, func(t *testing.T) {
			body := menuMDL([]*types.NavMenuItem{{
				Caption: "X", Page: "M.P", Icon: icon, IconType: "Forms$IconCollectionIcon",
			}})
			script := "create or replace navigation Responsive menu (\n" + body + ");"
			prog, errs := visitor.Build(script)
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE emitted output its own parser rejects:\n%s\nerrors: %v", script, errs)
			}
			stmt, ok := prog.Statements[0].(*ast.AlterNavigationStmt)
			if !ok || len(stmt.MenuItems) != 1 {
				t.Fatalf("re-parsed to %T", prog.Statements[0])
			}
			if got := stmt.MenuItems[0].Icon; got != icon {
				t.Errorf("round trip changed the icon: %q -> %q", icon, got)
			}
		})
	}
}
