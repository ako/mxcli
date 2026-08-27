// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-ledger #131: `mxcli check` reported every string literal containing
// Mendix's own apostrophe escape as a malformed expression.
//
// ” is how the Mendix expression language escapes an apostrophe inside a string
// literal — CLAUDE.md states the rule ("escaped by doubling them: 'it”s here'";
// never backslashes), the executor writes it, and DESCRIBE round-trips it. Only
// exprcheck disagreed: its lexer scanned to the next ' with no notion of an
// escape, so 'a”b' lexed as TWO strings, the parser consumed the first and
// reported the second as a leftover token.
//
// The message was a red herring — "possible missing space between keywords",
// with nothing glued and no keyword involved — which is what made the reporter
// bisect a 600-line file by hand to find it.
//
// It reproduces only with `-p`: exprcheck runs when a project is supplied, and
// `make check-mdl` runs without one, which is why this repo's own MDL gate never
// saw it across six of the reporter's builds.
package exprcheck

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/exprcheck/hints"
)

func TestLexEscapedApostrophe(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // expected TokString texts, in order
	}{
		{"doubled in the middle", `'a''b'`, []string{`'a''b'`}},
		{"an OData filter value", `' and cat eq ''' + $x`, []string{`' and cat eq '''`}},
		{"a doubled pair as the whole value", `''''`, []string{`''''`}},
		{"an empty string is still empty", `''`, []string{`''`}},
		{"two separate literals still lex as two", `'a' + 'b'`, []string{`'a'`, `'b'`}},
		{"escape at the start", `'''a'`, []string{`'''a'`}},
		{"escape at the end", `'a'''`, []string{`'a'''`}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for _, tok := range Lex(tc.src) {
				if tok.Kind == TokError {
					t.Fatalf("lex error on %q: %q", tc.src, tok.Text)
				}
				if tok.Kind == TokString {
					got = append(got, tok.Text)
				}
			}
			if len(got) != len(tc.want) {
				t.Fatalf("lexed %d strings %q, want %d %q", len(got), got, len(tc.want), tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("string %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseEscapedApostropheHasNoLeftover(t *testing.T) {
	// The reported symptom: a leftover token after the expression.
	srcs := []string{`'a''b'`, `' and cat eq ''' + 'x'`}
	for _, src := range srcs {
		_, hs := NewParser().Parse(src, Context{})
		for _, h := range hs {
			if h.Severity == hints.SeverityError {
				t.Errorf("Parse(%q): unexpected error %q (ako/mxcli-ledger #131)", src, h.Problem)
			}
		}
	}
}

func TestLexUnterminatedStringStillErrors(t *testing.T) {
	// The control for the lexer change, asserted at the layer that changes: a
	// lexer that simply swallowed to end-of-input would satisfy every case above
	// while silently accepting a broken literal. Both an ordinary unterminated
	// string and one that ends mid-escape must still produce TokError.
	srcs := []string{`'unterminated`, `'a''b`}
	for _, src := range srcs {
		var sawErr bool
		for _, tok := range Lex(src) {
			if tok.Kind == TokError {
				sawErr = true
			}
		}
		if !sawErr {
			t.Errorf("Lex(%q): no TokError, want one — an unterminated literal must not lex clean", src)
		}
	}
}
