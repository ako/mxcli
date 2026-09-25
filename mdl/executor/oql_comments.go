// SPDX-License-Identifier: Apache-2.0

package executor

import "strings"

// stripOQLComments blanks out the comments in an OQL query so the text-level
// checks below never read one as query text.
//
// Every static OQL check here — the select-list split, the alias rules, type
// inference, the association columns — works on the raw query string, which
// keeps the author's comments because the query is stored verbatim. A `--`
// comment in the select list was therefore split out as a column of its own,
// and each comma inside it made another (mendixlabs/mxcli#1175):
//
//	select column 1 has no as alias: '-- the customer's running total'
//
// Each comment character is replaced by a space and newlines are kept, so the
// result is the same length and every byte offset still points at the same
// place in the original. String literals ('…', with ” as the escape) and
// quoted identifiers ("…") are skipped, since `--` inside either is data.
func stripOQLComments(oql string) string {
	if !strings.Contains(oql, "--") && !strings.Contains(oql, "/*") {
		return oql
	}
	b := []byte(oql)
	blank := func(from, to int) {
		for k := from; k < to; k++ {
			if b[k] != '\n' && b[k] != '\r' {
				b[k] = ' '
			}
		}
	}
	for i := 0; i < len(b); i++ {
		switch c := b[i]; {
		case c == '\'' || c == '"':
			// Skip to the closing quote. A doubled quote ('it''s') reads as two
			// adjacent runs, which lands in the same place.
			for i++; i < len(b) && b[i] != c; i++ {
			}
		case c == '-' && i+1 < len(b) && b[i+1] == '-':
			end := i
			for end < len(b) && b[end] != '\n' {
				end++
			}
			blank(i, end)
			i = end
		case c == '/' && i+1 < len(b) && b[i+1] == '*':
			end := len(b)
			if k := strings.Index(oql[i+2:], "*/"); k >= 0 {
				end = i + 2 + k + 2
			}
			blank(i, end)
			i = end - 1
		}
	}
	return string(b)
}
