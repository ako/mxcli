// SPDX-License-Identifier: Apache-2.0

// Regression tests for expression string literal escaping in expressionToString.
//
// The previous implementation had two dueling bugs:
//  1. Using mdlQuote() duplicated every backslash, breaking regex escape
//     sequences like `\d` that the Mendix expression engine reads literally.
//  2. Using only apostrophe-doubling was believed to emit control characters
//     "which the MDL lexer rejects" — it does not. STRING_LITERAL is
//     `'\” ( ~['\\] | '\\' . | '\'\” )* '\”`, and `~['\\]` admits every byte
//     but an apostrophe and a backslash. Escaping them was added for a problem
//     that did not exist, and cost the stored value: measured against a running
//     11.13 runtime, a microflow storing 'a\tb' put FOUR bytes in the database
//     (61 5c 74 62 — backslash, t), because Mendix's expression engine has no
//     backslash escapes at all.
//
// So quoteExpressionLiteral now passes a control character through as itself,
// and still doubles an apostrophe and a backslash followed by an
// MDL-significant letter (n/r/t/\/') so `\t` meant literally survives a
// reparse, while other backslash sequences pass through so regex escapes do.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func TestQuoteExpressionLiteral_PreservesRegexBackslashEscapes(t *testing.T) {
	// `\d`, `\w`, `\s`, `\p{Lu}` etc. must pass through verbatim — the
	// Mendix expression engine treats them as literal `\d` and relies on
	// the regex compiler to interpret the escape.
	cases := []string{
		`^\d+$`,
		`\w+`,
		`\s*`,
		`\p{Lu}`,
		`mix \d and \w`,
	}
	for _, in := range cases {
		got := quoteExpressionLiteral(in)
		want := "'" + in + "'"
		if got != want {
			t.Errorf("quoteExpressionLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuoteExpressionLiteral_PassesRawControlCharsThrough(t *testing.T) {
	// The value is going into a MENDIX expression, where a backslash is just a
	// backslash. Escaping a tab here is what put `a\tb` — four bytes, with a
	// literal backslash — into the database instead of a tab, which is how a
	// tab-delimited export became one cell per row (ako/mxcli-captrack #5).
	//
	// The MDL lexer accepts these raw, so the describe output still reparses;
	// the raw newline already round-tripped this way, which is why only the tab
	// ever looked broken.
	cases := []struct {
		in   string
		want string
	}{
		{"line1\nline2", "'line1\nline2'"},
		{"line1\r\nline2", "'line1\r\nline2'"},
		{"col\tcol", "'col\tcol'"},
	}
	for _, tc := range cases {
		got := quoteExpressionLiteral(tc.in)
		if got != tc.want {
			t.Errorf("quoteExpressionLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
		// The inverse of the old invariant, and the point of the fix: the byte
		// the author wrote must still be there.
		if !strings.ContainsRune(got, []rune(strings.TrimLeft(tc.in, "linecol0123456789"))[0]) {
			t.Errorf("output %q lost its control character", got)
		}
	}
}

func TestQuoteExpressionLiteral_StillEscapesALiteralBackslashLetterPair(t *testing.T) {
	// A backslash the author meant literally, followed by n/r/t, must still be
	// doubled — otherwise unquoteString decodes the pair back into a control
	// character on reparse and a two-character `\t` silently becomes a tab.
	// This is the one case where the two grammars genuinely differ, and it is
	// why the fix is not "stop escaping everything".
	for _, tc := range []struct{ in, want string }{
		{`\t`, `'\\t'`},
		{`\n`, `'\\n'`},
		{`\d`, `'\d'`}, // not MDL-significant: passes through for regexes
	} {
		if got := quoteExpressionLiteral(tc.in); got != tc.want {
			t.Errorf("quoteExpressionLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestQuoteExpressionLiteral_DoublesApostrophes(t *testing.T) {
	got := quoteExpressionLiteral("it's here")
	want := "'it''s here'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestQuoteExpressionLiteral_EscapesBackslashBeforeEscapeLetter(t *testing.T) {
	// `\` followed by one of the recognised escape letters must be doubled,
	// otherwise the visitor's unquoteString would decode a backslash-letter
	// pair back into a control character on reparse.
	cases := []struct {
		in   string
		want string
	}{
		{`\n`, `'\\n'`},
		{`\r`, `'\\r'`},
		{`\t`, `'\\t'`},
		{`\\`, `'\\\\'`}, // double backslash roundtrips as double backslash
	}
	for _, tc := range cases {
		got := quoteExpressionLiteral(tc.in)
		if got != tc.want {
			t.Errorf("quoteExpressionLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestQuoteExpressionLiteral_TrailingBackslashDoubled(t *testing.T) {
	// A trailing backslash in the AST value cannot be emitted raw: the lexer
	// reads the closing `\'` as an escape pair (`\\ .`), never terminating the
	// literal. Doubling is the only safe representation — unquoteString
	// decodes `\\` back to a single backslash, preserving the value.
	cases := []struct {
		in   string
		want string
	}{
		{`abc\`, `'abc\\'`},
		{`\`, `'\\'`},
		{`regex \d\`, `'regex \d\\'`},
	}
	for _, tc := range cases {
		got := quoteExpressionLiteral(tc.in)
		if got != tc.want {
			t.Errorf("quoteExpressionLiteral(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestQuoteExpressionLiteral_RoundTripsThroughTheRealParser(t *testing.T) {
	// Critical invariant: whatever quoteExpressionLiteral emits must come back
	// out of the actual MDL parser as the same value, so describe → exec →
	// describe is stable.
	//
	// This is now a real round trip. The previous version asserted "output
	// contains no control bytes", which was asserting the bug — a control byte
	// in the output is exactly what correctness requires, since the string is
	// stored into a Mendix expression where a backslash is only a backslash.
	for _, raw := range []string{
		"multi\nline\twith 'quotes'",
		"col\tcol",
		"line1\r\nline2",
		`regex ^\d+$`,
		`literal \t backslash-t`,
		`trailing backslash \`,
		"it's here",
	} {
		src := "create microflow M.MF_RT()\nbegin\n  declare $s String = " +
			quoteExpressionLiteral(raw) + ";\nend\n"
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Errorf("%q: emitted MDL does not parse: %v\n  src: %q", raw, errs, src)
			continue
		}
		got, ok := firstDeclaredStringLiteral(prog)
		if !ok {
			t.Errorf("%q: could not read the literal back", raw)
			continue
		}
		if got != raw {
			t.Errorf("round trip changed the value:\n  in   %q\n  MDL  %q\n  back %q",
				raw, src, got)
		}
	}
}

// firstDeclaredStringLiteral digs the value out of `declare $s String = '...'`.
func firstDeclaredStringLiteral(prog *ast.Program) (string, bool) {
	for _, st := range prog.Statements {
		mf, ok := st.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, body := range mf.Body {
			d, ok := body.(*ast.DeclareStmt)
			if !ok {
				continue
			}
			// A multi-line expression comes back wrapped in a SourceExpr, which
			// keeps the raw source text. That wrapper is also WHY the asymmetry
			// existed: an expression spanning lines never reached
			// quoteExpressionLiteral, so a raw newline was stored intact while a
			// raw tab — single-line, unwrapped — got escaped.
			inner := d.InitialValue
			if se, ok := inner.(*ast.SourceExpr); ok {
				inner = se.Expression
			}
			lit, ok := inner.(*ast.LiteralExpr)
			if !ok || lit.Kind != ast.LiteralString {
				continue
			}
			s, ok := lit.Value.(string)
			return s, ok
		}
	}
	return "", false
}
