// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// mendixlabs/mxcli#1175: a `--` comment inside the select list was split into
// the column list as if it were a column, so a documented view reported
//
//	select column 1 has no as alias: '-- the customer's running total'
//
// and, since the comment's own comma split it again, a phantom second column.
// Neither the apostrophe nor the comma is the cause — any comment in the
// select list was a column — but together they are the reported text.
const oqlWithSelectComment = `SELECT
    -- the customer's running total, summed here rather than on the page
    SUM(s.Amount) as Total,
    /* the number of sales, from the table */
    COUNT(s.Amount) as Cnt
  FROM MyFirstModule.Sale as s`

func TestOQLComment_IsNotASelectColumn(t *testing.T) {
	for _, v := range ValidateOQLSyntax(oqlWithSelectComment) {
		t.Errorf("unexpected %s: %s", v.RuleID, v.Message)
	}
}

func TestStripOQLComments(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"line comment", "a -- x, y\nb", "a        \nb"},
		{"block comment", "a /* x */ b", "a         b"},
		{"dashes in a string literal", "'a--b' as X", "'a--b' as X"},
		{"doubled quote in a literal", "'it''s -- no' as X", "'it''s -- no' as X"},
		{"dashes in a quoted identifier", `s."a--b" as X`, `s."a--b" as X`},
		{"apostrophe in a comment", "-- it's\n'x'", "       \n'x'"},
		{"unterminated block", "a /* x", "a     "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripOQLComments(tt.in)
			if got != tt.want {
				t.Errorf("stripOQLComments(%q)\n got: %q\nwant: %q", tt.in, got, tt.want)
			}
			if len(got) != len(tt.in) {
				t.Errorf("length changed: %d -> %d (offsets must be preserved)", len(tt.in), len(got))
			}
		})
	}
	if strings.Contains(stripOQLComments(oqlWithSelectComment), "customer") {
		t.Error("comment text survived")
	}
}
