// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strconv"
	"strings"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
)

// listGlyphs handles SHOW GLYPHS [LIKE 'pattern'].
//
// The icon-collection pair (`show icon collections` / `describe icon collection`)
// covers icons that are model documents. A glyph is not one: `icon glyph <n>`
// stores a bare character code into a FONT, so there is nothing in the project to
// list and nothing a connection would add — which is exactly why the codes were
// unbrowsable, and why MDL078 previously had to spell its advice as a list of
// numeric ranges with holes in it.
//
// LIKE matches the name, because that is the direction an author needs: they
// know they want a star, not that a star is 57350.
func listGlyphs(ctx *ExecContext, like string) error {
	needle := strings.ToLower(strings.TrimSpace(like))
	result := &TableResult{Columns: []string{"Code", "Name", "MDL"}}
	for _, g := range GlyphIcons() {
		if needle != "" && !glyphMatches(g, needle) {
			continue
		}
		name := g.Name
		if len(g.Aliases) > 0 {
			name += " (" + strings.Join(g.Aliases, ", ") + ")"
		}
		result.Rows = append(result.Rows, []any{
			g.Code, name, fmt.Sprintf("icon glyph %d", g.Code),
		})
	}
	if len(result.Rows) == 0 {
		// Say what was searched, or an empty table reads as "the font is empty".
		result.Summary = fmt.Sprintf("(no glyph name contains %q — the font defines %d icons)",
			like, len(GlyphIcons()))
		return writeResult(ctx, result)
	}
	if needle != "" {
		result.Summary = fmt.Sprintf("(%d of %d glyph(s) matching %q)",
			len(result.Rows), len(GlyphIcons()), like)
	} else {
		result.Summary = fmt.Sprintf("(%d glyph(s) in the Mendix glyph font)", len(result.Rows))
	}
	return writeResult(ctx, result)
}

// glyphMatches reports whether the needle appears in the icon's name or any of
// its aliases. Aliases are searched too, or `show glyphs like 'btc'` would find
// nothing while `icon glyph 57895` is exactly what the author wants.
func glyphMatches(g GlyphIcon, needle string) bool {
	if strings.Contains(strings.ToLower(g.Name), needle) {
		return true
	}
	for _, a := range g.Aliases {
		if strings.Contains(strings.ToLower(a), needle) {
			return true
		}
	}
	return false
}

// describeGlyph handles DESCRIBE GLYPH 57350 and DESCRIBE GLYPH 'star'.
//
// Both directions are accepted because both are asked: a code when reading a
// menu somebody else wrote, a name when writing one.
func describeGlyph(ctx *ExecContext, subject string) error {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return mdlerrors.NewValidation("describe glyph needs a character code or an icon name")
	}

	if code, err := strconv.Atoi(subject); err == nil {
		g, ok := LookupGlyph(code)
		if !ok {
			return mdlerrors.NewNotFoundMsg("glyph", strconv.Itoa(code), fmt.Sprintf(
				"glyph %d not found: the Mendix glyph font does not define this code. Its codes "+
					"are sparse, so a nearby number is usually not defined either; browse them "+
					"with `show glyphs` or search by name with `show glyphs like '<text>'`", code))
		}
		return writeResult(ctx, glyphDetail(g))
	}

	// A name. Exact match wins over a substring, so `describe glyph 'star'`
	// answers about star rather than about star-empty.
	needle := strings.ToLower(subject)
	var partial []GlyphIcon
	for _, g := range GlyphIcons() {
		if strings.EqualFold(g.Name, subject) {
			return writeResult(ctx, glyphDetail(g))
		}
		for _, a := range g.Aliases {
			if strings.EqualFold(a, subject) {
				return writeResult(ctx, glyphDetail(g))
			}
		}
		if glyphMatches(g, needle) {
			partial = append(partial, g)
		}
	}
	if len(partial) == 1 {
		return writeResult(ctx, glyphDetail(partial[0]))
	}
	if len(partial) > 1 {
		// Ambiguous: name them rather than picking one, since the codes differ.
		names := make([]string, 0, len(partial))
		for _, g := range partial {
			names = append(names, fmt.Sprintf("%s (%d)", g.Name, g.Code))
		}
		return mdlerrors.NewValidationf("%q matches %d glyphs: %s. Name one exactly, "+
			"or list them with `show glyphs like '%s'`",
			subject, len(partial), strings.Join(names, ", "), subject)
	}
	return mdlerrors.NewNotFoundMsg("glyph", subject, fmt.Sprintf(
		"glyph %q not found: no icon in the Mendix glyph font has that name. "+
			"Search with `show glyphs like '<text>'`", subject))
}

// glyphDetail renders one glyph, including the MDL to paste.
func glyphDetail(g GlyphIcon) *TableResult {
	result := &TableResult{Columns: []string{"Property", "Value"}}
	result.Rows = append(result.Rows,
		[]any{"Name", g.Name},
		[]any{"Code", strconv.Itoa(g.Code)},
		[]any{"Hex", fmt.Sprintf("0x%04X", g.Code)},
		[]any{"CSS class", "glyphicon-" + g.Name},
	)
	if len(g.Aliases) > 0 {
		result.Rows = append(result.Rows, []any{"Aliases", strings.Join(g.Aliases, ", ")})
	}
	result.Rows = append(result.Rows, []any{"MDL", fmt.Sprintf("icon glyph %d", g.Code)})
	// The point of the pair, not a footnote: a glyph code is unchecked until
	// MDL078 sees it, while a collection reference is resolved by
	// `check --references` before anything is written.
	result.Summary = "An icon collection reference is checked when a glyph code is not — " +
		"prefer `icon Atlas_Core.Atlas.<name>` where the icon exists there."
	return result
}
