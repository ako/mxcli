// SPDX-License-Identifier: Apache-2.0

package visitor

import "strings"

// FlattenXPathConstraint collapses a constraint onto one line: whitespace runs
// become a single space, and the padding a formatted constraint carries inside
// its brackets and parentheses is removed.
//
// This is the inverse of FormatXPathConstraint for everything the formatter
// produces — Flatten(Format(x)) == x for any x that was already flat — and it is
// what DESCRIBE emits. The two directions are deliberate: the constraint is
// stored broken across lines because that is what Studio Pro's editor shows
// (upstream #979), while MDL keeps it on one line so a description reads the way
// it always has and re-executing it re-derives the same stored text.
//
// The scan is quote-aware. Mendix escapes a quote inside a literal by doubling
// it, and a literal may legitimately contain runs of spaces that are data.
func FlattenXPathConstraint(constraint string) string {
	if !strings.ContainsAny(constraint, " \t\r\n") {
		return constraint
	}

	var b strings.Builder
	b.Grow(len(constraint))

	inQuote := false
	pendingSpace := false
	for i := 0; i < len(constraint); i++ {
		c := constraint[i]

		if inQuote {
			b.WriteByte(c)
			if c == '\'' {
				if i+1 < len(constraint) && constraint[i+1] == '\'' {
					b.WriteByte('\'')
					i++
					continue
				}
				inQuote = false
			}
			continue
		}

		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			pendingSpace = b.Len() > 0
			continue
		}

		// A separator does not want a space in front of it, and nothing wants one
		// straight after an opener. `][` is the sibling-predicate join Mendix
		// stores — the formatter puts those groups on separate lines, and they go
		// back together with nothing between them.
		if pendingSpace {
			switch {
			case c == ']' || c == ')' || c == ',':
			case c == '[' && strings.HasSuffix(b.String(), "]"):
			case endsWithOpener(b.String()):
			default:
				b.WriteByte(' ')
			}
			pendingSpace = false
		}

		b.WriteByte(c)
		if c == '\'' {
			inQuote = true
		}
	}

	return strings.TrimSpace(b.String())
}

// endsWithOpener reports whether the text written so far ends in a bracket that
// opens a group, so the next token butts straight up against it.
func endsWithOpener(s string) bool {
	if s == "" {
		return false
	}
	switch s[len(s)-1] {
	case '[', '(':
		return true
	}
	return false
}
