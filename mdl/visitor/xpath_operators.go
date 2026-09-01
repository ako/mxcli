// SPDX-License-Identifier: Apache-2.0

package visitor

import "strings"

// XPath 1.0 spells its operator keywords in LOWER CASE only. MDL's lexer accepts
// any case — `AND: A N D;` — so a constraint written `WHERE a = x AND b = y` is
// perfectly good MDL and, stored verbatim, is XPath mxbuild rejects:
//
//	Retrieve object(s) activity 'Retrieve list of LiveForecast from database':
//	  Error(s) in XPath constraint.                                   (CE0161)
//
// mxcli lowercased the operator already, on one of its two paths.
// expressionToXPath does it while walking the parse tree; but
// buildRetrieveWhereExpression freezes the RAW SOURCE whenever the clause
// contains a `/`, which every variable path does, and expressionToXPath's
// SourceExpr case hands that text straight back. So the casing survived exactly
// when a path was present — which is why the reporting project's literal-only
// reproducer built cleanly and looked like a non-bug (mxcli-formula1 §80).
//
// Normalising here rather than in the retrieve builder covers all three of
// mxcli's constraint writers — retrieve, page data source, entity access rule —
// since all three go through FormatXPathConstraint.

// xpathOperatorKeywords are the operator names to fold to lower case. Only the
// three MDL itself accepts in any case; `div` and `mod` are XPath keywords too
// but nothing in MDL produces them, and a rewrite nothing needs is a rewrite
// that can only be wrong.
var xpathOperatorKeywords = map[string]string{
	"and": "and",
	"or":  "or",
	"not": "not",
}

// NormalizeXPathOperators folds XPath's operator keywords to lower case,
// leaving string literals and identifiers exactly as they are.
//
// The two things it must not touch are the reason this scans rather than
// substitutes:
//
//   - Inside a STRING LITERAL the letters are data. Rewriting `'A AND B'`
//     changes which rows the constraint matches — a silent wrong-answer bug,
//     strictly worse than the build error being fixed.
//   - An identifier that merely contains the letters (`Brand`, `Andrew`,
//     `NOTES`, `Order_Andon`) is not an operator. Only a whole token counts.
func NormalizeXPathOperators(constraint string) string {
	if constraint == "" {
		return constraint
	}
	var b strings.Builder
	b.Grow(len(constraint))

	for i := 0; i < len(constraint); {
		c := constraint[i]

		// A string literal runs to its closing quote; a doubled quote inside one
		// is an escaped quote, not the end (the same rule the MDL expression
		// scanner uses).
		if c == '\'' {
			j := i + 1
			for j < len(constraint) {
				if constraint[j] == '\'' {
					if j+1 < len(constraint) && constraint[j+1] == '\'' {
						j += 2
						continue
					}
					j++
					break
				}
				j++
			}
			b.WriteString(constraint[i:j])
			i = j
			continue
		}

		if !isXPathWordByte(c) {
			b.WriteByte(c)
			i++
			continue
		}

		// A whole word. `not` is a function, so it is a keyword whether or not a
		// `(` follows; `and`/`or` are infix. Either way the test is the token.
		j := i
		for j < len(constraint) && isXPathWordByte(constraint[j]) {
			j++
		}
		word := constraint[i:j]
		if lower, ok := xpathOperatorKeywords[strings.ToLower(word)]; ok && !partOfXPathName(constraint, i, j) {
			b.WriteString(lower)
		} else {
			b.WriteString(word)
		}
		i = j
	}
	return b.String()
}

// isXPathWordByte reports whether c can appear inside an XPath name token.
// `.`, `/` and `$` are deliberately excluded: they SEPARATE names, and
// partOfXPathName uses them to tell a qualified name from a bare keyword.
func isXPathWordByte(c byte) bool {
	return c == '_' || c == '-' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// partOfXPathName reports whether the token at [start,end) is a segment of a
// qualified or navigated name — `Module.Not`, `$v/And`, `And/Name` — where the
// word is somebody's identifier and not an operator.
func partOfXPathName(s string, start, end int) bool {
	for i := start - 1; i >= 0; i-- {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
			continue
		case '.', '/', '$', '@':
			return true
		}
		break
	}
	for i := end; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\r', '\n':
			continue
		case '.', '/':
			return true
		}
		break
	}
	return false
}
