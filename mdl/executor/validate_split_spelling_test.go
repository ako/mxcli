// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

func splitSource(branch, empty string) string {
	return `CREATE MICROFLOW Sample.Route ($Animal: Sample.Animal)
RETURNS String
BEGIN
  split type $Animal
    ` + branch + `
      return 'woof';
    ` + empty + `
      return 'none';
  end split;
END;`
}

// MDL065 warns on the pre-#913 spelling and stays quiet on the current one.
// Both build the identical flow, so it is a warning: an error would reject
// working scripts.
func TestMDL065_WarnsOnLegacySplitSpelling(t *testing.T) {
	tests := []struct {
		name          string
		branch, empty string
		wantWarnings  int
	}{
		{"current spelling is silent", "when Sample.Dog then", "when (empty) then", 0},
		{"legacy branch keyword", "case Sample.Dog", "when (empty) then", 1},
		{"legacy empty branch", "when Sample.Dog then", "else", 1},
		{"both legacy", "case Sample.Dog", "else", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got int
			for _, v := range checkMicroflowSource(t, splitSource(tc.branch, tc.empty)) {
				if v.RuleID != "MDL065" {
					continue
				}
				got++
				if v.Severity != linter.SeverityWarning {
					t.Errorf("MDL065 severity = %v, want warning — an error would reject working scripts",
						v.Severity)
				}
			}
			if got != tc.wantWarnings {
				t.Errorf("MDL065 count = %d, want %d", got, tc.wantWarnings)
			}
		})
	}
}

// The `else` diagnostic has to say what `else` actually IS, not just that it is
// deprecated. An author who reads it as a default branch is wrong, and mxbuild
// tells them so only indirectly, via CE0090 on the types they did not cover.
func TestMDL065_ElseWarningExplainsTheEmptySemantics(t *testing.T) {
	var msg, hint string
	for _, v := range checkMicroflowSource(t, splitSource("when Sample.Dog then", "else")) {
		if v.RuleID == "MDL065" {
			msg, hint = v.Message, v.Suggestion
		}
	}
	if msg == "" {
		t.Fatal("no MDL065 violation for the `else` spelling")
	}
	for _, want := range []string{"(empty)", "null", "CE0090"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not mention %q — it reads as a rename, not a meaning change:\n%s", want, msg)
		}
	}
	if !strings.Contains(hint, "when (empty) then") {
		t.Errorf("suggestion does not name the replacement:\n%s", hint)
	}
}

// A microflow carrying the legacy spelling must still EXECUTE. MDL065 is a
// warning, and `exec`'s pre-flight gate halts only on errors — if this rule
// were ever promoted to an error it would break every script in the wild.
func TestMDL065_DoesNotBlockExecution(t *testing.T) {
	violations := checkMicroflowSource(t, splitSource("case Sample.Dog", "else"))
	if summary := linter.Summarize(violations); summary.Errors > 0 {
		t.Fatalf("legacy split spelling produced %d error(s); exec would refuse the script:\n%+v",
			summary.Errors, violations)
	}
}
