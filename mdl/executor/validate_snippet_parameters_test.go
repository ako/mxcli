// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// MDL087 — mendixlabs/mxcli#1028. `mxcli check` has to carry this because
// nothing between the author and mxbuild types a snippet parameter: the
// reported script passed `check`, and the documented spelling is the one that
// fails. The rule and its measurements live in types.SnippetParameterTypeRule;
// this asserts the check reports them, on the same function exec refuses with.
func checkSnippetSource(t *testing.T, src string) []string {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	var msgs []string
	for _, stmt := range prog.Statements {
		for _, v := range validateSnippetParameters(stmt) {
			if v.RuleID != "MDL087" {
				t.Errorf("unexpected rule %s", v.RuleID)
			}
			msgs = append(msgs, v.Message)
		}
	}
	return msgs
}

func TestValidateSnippetParameters(t *testing.T) {
	cases := []struct {
		name    string
		params  string
		want    int
		phrases []string
	}{{
		// The reported repro, in the reporter's casing.
		name:    "the reported String parameter",
		params:  "$Label: string",
		want:    1,
		phrases: []string{"$Label", "CE0046", "Invalid data type 'String'"},
	}, {
		// Long and Integer are one stored type, so both quote Studio Pro's
		// combined caption — the message is meant to match a build log verbatim.
		name:    "Long reports the Integer/Long caption",
		params:  "$Count: Long",
		want:    1,
		phrases: []string{"Integer/Long"},
	}, {
		name:    "DateTime reports Mendix's own wording",
		params:  "$Due: DateTime",
		want:    1,
		phrases: []string{"Date and time"},
	}, {
		// One violation per bad parameter: a clause with six of them names all
		// six rather than one per run.
		name: "every primitive in one clause",
		params: "$Label: String, $Count: Long, $Rank: Integer, " +
			"$Amount: Decimal, $Active: Boolean, $Due: DateTime",
		want: 6,
	}, {
		// CONTROL: the form Mendix accepts. Measured at 0 errors on 11.13.0.
		name:   "an entity parameter is clean",
		params: "$Order: Sales.Order",
		want:   0,
	}, {
		// CONTROL: a quoted entity name is still an entity — the clause's other
		// bug (visitor.TestSnippetParameter_QuotedEntityNameIsUnquoted) must not
		// come back as a spurious MDL087.
		name:   "a quoted entity parameter is clean",
		params: `$Order: Sales."Order"`,
		want:   0,
	}, {
		name:   "no parameters at all",
		params: "",
		want:   0,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			header := ""
			if tc.params != "" {
				header = "( Params: { " + tc.params + " } )"
			}
			msgs := checkSnippetSource(t, "CREATE SNIPPET M.S "+header+
				" { DYNAMICTEXT dt (Content: 'x') }")
			if len(msgs) != tc.want {
				t.Fatalf("got %d violations, want %d: %v", len(msgs), tc.want, msgs)
			}
			for _, phrase := range tc.phrases {
				if !strings.Contains(msgs[0], phrase) {
					t.Errorf("message does not contain %q: %s", phrase, msgs[0])
				}
			}
		})
	}
}

// CONTROL: a PAGE with the same parameters produces nothing. MDL087 is about
// snippet parameters, and a rule that also fired on pages would break the
// documented, measured-valid page form.
func TestValidateSnippetParameters_IgnoresPages(t *testing.T) {
	if msgs := checkSnippetSource(t, `CREATE PAGE M.P (
  Title: 'P', Params: { $Label: String, $Count: Long }
) { }`); len(msgs) != 0 {
		t.Errorf("page flagged by MDL087: %v", msgs)
	}
}
