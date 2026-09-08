// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// Whitespace inside a string literal is part of the value being matched on.
// Folding it would change what the constraint selects, silently, in a place
// nobody would look — the naive strings.Fields version did exactly that.
func TestSingleLinePreservesWhitespaceInsideLiterals(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{
			name: "newlines and indentation outside literals are folded",
			in:   "[\n  (\n    contains(Value, 'abc')\n  )\n]",
			want: "[ ( contains(Value, 'abc') ) ]",
		},
		{
			name: "two spaces inside a literal survive",
			in:   "[Name = 'two  spaces']",
			want: "[Name = 'two  spaces']",
		},
		{
			name: "a newline inside a literal survives",
			in:   "[Name = 'a\nb']",
			want: "[Name = 'a\nb']",
		},
		{
			name: "an escaped quote does not end the literal",
			in:   "[contains(V, '''a  b''')]",
			want: "[contains(V, '''a  b''')]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := singleLine(tc.in); got != tc.want {
				t.Errorf("singleLine(%q)\n got %q\nwant %q", tc.in, got, tc.want)
			}
		})
	}
}
