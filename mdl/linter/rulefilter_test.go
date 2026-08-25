// SPDX-License-Identifier: Apache-2.0

package linter

import "testing"

// stubRule comes from linter_test.go.
func stubRules(ids ...string) []Rule {
	out := make([]Rule, 0, len(ids))
	for _, id := range ids {
		out = append(out, &stubRule{id})
	}
	return out
}

// #904: `mxcli lint -r CONV009` on a project where CONV009 had not loaded
// printed "No issues found." — indistinguishable from "the rule ran and your
// project is clean". Measured, the problem was broader than reported: `-r` never
// validated the id at all, so even a rule that has never existed in any release
// reported a clean project.
//
// The filter disables every rule not named, so an unmatched name disables
// everything and the run reports success. Naming what is not there must be an
// error instead.
func TestUnknownRuleIDs(t *testing.T) {
	known := stubRules("MPR001", "CONV009", "MDL-FLOW01")

	tests := []struct {
		name      string
		requested []string
		want      []string
	}{
		{"all known", []string{"MPR001", "CONV009"}, nil},
		{"one unknown", []string{"MPR001", "NOPE"}, []string{"NOPE"}},
		{"never existed", []string{"TOTALLY_MADE_UP"}, []string{"TOTALLY_MADE_UP"}},
		{"several unknown keep request order", []string{"ZZZ", "MPR001", "AAA"}, []string{"ZZZ", "AAA"}},
		{"nothing requested", nil, nil},
		// A hyphenated id must not be mangled by the comparison.
		{"hyphenated id is known", []string{"MDL-FLOW01"}, nil},
		// Case-insensitive: a lowercase id previously matched nothing and
		// silently disabled every rule, which is the same trap in miniature.
		{"lowercase matches", []string{"conv009"}, nil},
		{"mixed case matches", []string{"Mdl-Flow01"}, nil},
		// Whitespace from a shell-split list should not turn a real id unknown.
		{"surrounding spaces tolerated", []string{" CONV009 "}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnknownRuleIDs(tt.requested, known)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// The reported id is echoed back exactly as the user typed it, so the error
// message names what they wrote rather than a normalised form they would not
// recognise.
func TestUnknownRuleIDs_EchoesRequestedSpelling(t *testing.T) {
	got := UnknownRuleIDs([]string{"conv999"}, stubRules("CONV009"))
	if len(got) != 1 || got[0] != "conv999" {
		t.Fatalf("got %v, want [conv999]", got)
	}
}

// MatchesRuleID is what the command's allowlist uses, so it must agree with
// UnknownRuleIDs — otherwise an id passes validation and then disables
// everything anyway, which is the original bug with an extra step.
func TestMatchesRuleIDAgreesWithValidation(t *testing.T) {
	known := stubRules("MPR001", "CONV009", "MDL-FLOW01")
	for _, requested := range []string{"conv009", "CONV009", " MDL-FLOW01 "} {
		if len(UnknownRuleIDs([]string{requested}, known)) != 0 {
			t.Fatalf("%q should validate as known", requested)
		}
		var matched bool
		for _, r := range known {
			if MatchesRuleID(requested, r.ID()) {
				matched = true
			}
		}
		if !matched {
			t.Errorf("%q validated as known but matches no rule — the allowlist would disable everything", requested)
		}
	}
}
