// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// A view-entity OQL that uses a reserved OQL word (e.g. Quarter, a date-part
// keyword) as an attribute/alias parses in mxcli but fails MxBuild with CE0174.
// mxcli can't quote it (the OQL grammar has no quoted-identifier form), so
// ValidateOQLSyntax must surface it early as MDL032.
func TestValidateOQLSyntax_ReservedWord(t *testing.T) {
	hasMDL032 := func(oql string) bool {
		for _, v := range ValidateOQLSyntax(oql) {
			if v.RuleID == "MDL032" {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name string
		oql  string
		want bool
	}{
		{"reserved word as attr ref + alias", "select s.Quarter as Quarter, sum(s.Amount) as Total from M.Sales as s group by s.Quarter", true},
		{"reserved word only as alias", "select s.Period as Month, sum(s.Amount) as Total from M.Sales as s group by s.Period", true},
		{"reserved word only in attr ref", "select s.Year as Yr, sum(s.Amount) as Total from M.Sales as s group by s.Year", true},
		{"no reserved word", "select s.Region as Region, sum(s.Amount) as Total from M.Sales as s group by s.Region", false},
		// A reserved substring inside a longer identifier must NOT trip (word boundary).
		{"reserved substring in longer name", "select s.QuarterName as QuarterName, sum(s.Amount) as Total from M.Sales as s group by s.QuarterName", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasMDL032(c.oql); got != c.want {
				t.Errorf("MDL032 present = %v, want %v for %q", got, c.want, c.oql)
			}
		})
	}
}

// The MDL032 message must name the offending word and mention CE0174 so the
// diagnostic is actionable.
func TestValidateOQLSyntax_ReservedWordMessage(t *testing.T) {
	vs := ValidateOQLSyntax("select s.Quarter as Quarter from M.Sales as s group by s.Quarter")
	var msg string
	for _, v := range vs {
		if v.RuleID == "MDL032" {
			msg = v.Message
		}
	}
	if msg == "" {
		t.Fatal("expected an MDL032 violation")
	}
	if !strings.Contains(msg, "Quarter") || !strings.Contains(msg, "CE0174") {
		t.Errorf("message should name the word and CE0174: %q", msg)
	}
}
