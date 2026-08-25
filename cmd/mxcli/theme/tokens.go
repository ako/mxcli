// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// A theme's palette is nothing but --mxt-* custom properties, so seeding one
// from a design artifact is a matter of reading those declarations back out.
// That is deliberately the whole contract: a design tool that emits
//
//	:root { --mxt-brand: #7f5af0; --mxt-ground: #fffffe; }
//	@media (prefers-color-scheme: dark) { :root { --mxt-ground: #16161a; } }
//
// anywhere in a .css, .scss or .html file is understood, and nothing else has
// to be agreed. Inferring a palette from arbitrary inline styles was the
// alternative and it is a guess: two greys in a mockup do not say which is the
// ground and which is a hovered row.

// tokenDecl matches one custom-property declaration at the tail of the text
// accumulated since the last brace or semicolon.
var tokenDecl = regexp.MustCompile(`(--mxt-[a-z0-9-]+)\s*:\s*([^;]+)$`)

// darkContext matches the block headers that mean "these are the dark values".
var darkContext = regexp.MustCompile(`(?i)prefers-color-scheme\s*:\s*dark|\.theme-dark|\[data-theme\s*=\s*["']?dark|\bdark-mode\b`)

// lightContext is the same for an explicitly light block, which matters when a
// dark-first design nests its light overrides inside one.
var lightContext = regexp.MustCompile(`(?i)prefers-color-scheme\s*:\s*light|\.theme-light|\[data-theme\s*=\s*["']?light|\blight-mode\b`)

// TokenSet is one palette's worth of extracted values, keyed by token name.
type TokenSet map[string]string

// Names returns the token names, sorted, so output and errors are stable.
func (s TokenSet) Names() []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Tokens is what a design artifact yielded: the base palette and, if the
// artifact declared one, the values that differ in the other variant.
type Tokens struct {
	// Source is the file the tokens were read from, for reporting.
	Source string
	// Base holds declarations outside any variant block.
	Base TokenSet
	// Dark and Light hold declarations found inside a variant block.
	Dark  TokenSet
	Light TokenSet
}

// Count is the total number of declarations read.
func (t *Tokens) Count() int { return len(t.Base) + len(t.Dark) + len(t.Light) }

// scoped returns just the declarations the artifact put inside a block for
// this variant, with no base values mixed in.
func (t *Tokens) scoped(v Variant) TokenSet {
	if v == VariantLight {
		return t.Light
	}
	return t.Dark
}

// forVariant returns the token set that seeds a theme's given variant: the
// base declarations, overlaid with anything the artifact scoped to that
// variant. A design that declares only a base palette seeds both.
func (t *Tokens) forVariant(v Variant) TokenSet {
	out := TokenSet{}
	for k, val := range t.Base {
		out[k] = val
	}
	scoped := t.Dark
	if v == VariantLight {
		scoped = t.Light
	}
	for k, val := range scoped {
		out[k] = val
	}
	return out
}

// ExtractTokens reads --mxt-* declarations out of CSS-shaped text — a
// stylesheet, an SCSS partial, or the <style> blocks of an HTML export.
//
// It is a scanner rather than a CSS parser because it has to survive being
// pointed at a whole HTML page: it tracks brace depth to know which selector a
// declaration sits under, respects quotes so a brace inside a string does not
// unbalance it, and ignores everything that is not a --mxt-* declaration.
func ExtractTokens(source, content string) *Tokens {
	t := &Tokens{Source: source, Base: TokenSet{}, Dark: TokenSet{}, Light: TokenSet{}}

	content = stripComments(content)

	var stack []string
	var pending strings.Builder
	var quote rune

	flush := func() {
		text := pending.String()
		pending.Reset()
		m := tokenDecl.FindStringSubmatch(text)
		if m == nil {
			return
		}
		name, value := m[1], strings.TrimSpace(m[2])
		if value == "" {
			return
		}
		switch scopeOf(stack) {
		case VariantDark:
			t.Dark[name] = value
		case VariantLight:
			t.Light[name] = value
		default:
			t.Base[name] = value
		}
	}

	for _, r := range content {
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			pending.WriteRune(r)
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
			pending.WriteRune(r)
		case '{':
			stack = append(stack, strings.TrimSpace(pending.String()))
			pending.Reset()
		case '}':
			flush()
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			pending.Reset()
		case ';':
			flush()
		default:
			pending.WriteRune(r)
		}
	}
	return t
}

// scopeOf reads the innermost variant marker off the open block headers. A
// declaration under `@media (prefers-color-scheme: dark) { :root { … } }` is
// dark even though the innermost header says nothing about it, so the whole
// stack is consulted — innermost first, so an explicit .theme-light inside a
// dark media query still reads as light.
func scopeOf(stack []string) Variant {
	for i := len(stack) - 1; i >= 0; i-- {
		if lightContext.MatchString(stack[i]) {
			return VariantLight
		}
		if darkContext.MatchString(stack[i]) {
			return VariantDark
		}
	}
	return ""
}

// stripComments removes /* … */ and // … comments so a commented-out token is
// not read as a declaration. It is quote-aware for the same reason the scanner
// is: a URL in a font src carries a //.
func stripComments(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	runes := []rune(s)
	var quote rune
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			out.WriteRune(r)
			continue
		}
		switch {
		case r == '\'' || r == '"':
			quote = r
			out.WriteRune(r)
		case r == '/' && i+1 < len(runes) && runes[i+1] == '*':
			for i += 2; i < len(runes); i++ {
				if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
					i++
					break
				}
			}
		case r == '/' && i+1 < len(runes) && runes[i+1] == '/':
			for i += 2; i < len(runes) && runes[i] != '\n'; i++ {
			}
			out.WriteRune('\n')
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// declaredTokens returns the --mxt-* names an SCSS body declares, in the order
// they appear. Used to validate a design artifact against the theme it is
// seeding, and to place appended tokens.
func declaredTokens(scss string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`(--mxt-[a-z0-9-]+)\s*:`).FindAllStringSubmatch(stripComments(scss), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// applyTokens rewrites the values of tokens the SCSS already declares and
// returns the names it could not place.
//
// Rewriting in place rather than regenerating the file is what keeps the
// generated palette readable: the section comments that say which tokens are
// surfaces and which are ink survive a reseed, and a diff between two
// generated themes is the values that differ and nothing else.
func applyTokens(scss string, tokens TokenSet) (string, []string) {
	out := scss
	var unplaced []string
	for _, name := range tokens.Names() {
		re := regexp.MustCompile(`(?m)^(\s*)` + regexp.QuoteMeta(name) + `\s*:[^;]*;`)
		if !re.MatchString(out) {
			unplaced = append(unplaced, name)
			continue
		}
		// The replacement is built rather than templated: a colour like
		// "#0f6e6b" is fine, but Go's Expand syntax would eat a $ in a value.
		replaced := false
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			if replaced {
				return m
			}
			replaced = true
			indent := m[:len(m)-len(strings.TrimLeft(m, " \t"))]
			return fmt.Sprintf("%s%s: %s;", indent, name, tokens[name])
		})
	}
	return out, unplaced
}

// appendTokens adds declarations to the end of an SCSS block that does not
// already carry them, under a comment saying where they came from.
func appendTokens(scss string, names []string, tokens TokenSet, source string) (string, error) {
	if len(names) == 0 {
		return scss, nil
	}
	end := strings.LastIndex(scss, "}")
	if end < 0 {
		return "", fmt.Errorf("cannot place %s: no block to add it to", strings.Join(names, ", "))
	}
	var b strings.Builder
	b.WriteString("\n  /* from ")
	b.WriteString(source)
	b.WriteString(" */\n")
	for _, n := range names {
		fmt.Fprintf(&b, "  %s: %s;\n", n, tokens[n])
	}
	return scss[:end] + b.String() + scss[end:], nil
}
