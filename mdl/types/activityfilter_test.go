// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"
	"testing"
)

func cond(col string, vals ...string) ActivityCondition {
	return ActivityCondition{Column: col, Values: vals}
}

// CheckActivityFilter is the one resolver `mxcli check` and the executor share.
// Everything it refuses has the same shape: a filter that would match NOTHING,
// write nothing, and exit 0 — indistinguishable from a genuinely empty result.
func TestCheckActivityFilterRefusesFiltersThatSelectNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    ActivityFilter
		want string // fragment of the expected message
	}{
		{"no conditions", ActivityFilter{}, "selects nothing"},
		{"unknown column", ActivityFilter{Conditions: []ActivityCondition{cond("colour", "red")}}, "unknown column"},
		{"unknown action", ActivityFilter{Conditions: []ActivityCondition{cond(ActivityColAction, "frobnicate")}}, "unknown action"},
		{"unknown level", ActivityFilter{Conditions: []ActivityCondition{cond(ActivityColLevel, "verbose")}}, "unknown log level"},
		{"disabled not boolean", ActivityFilter{Conditions: []ActivityCondition{cond(ActivityColDisabled, "maybe")}}, "takes true or false"},
		{"LIKE on a non-text column", ActivityFilter{Conditions: []ActivityCondition{
			{Column: ActivityColLevel, Like: true, Values: []string{"deb%"}}}}, "`LIKE` on `level`"},
		{"level with a non-log action", ActivityFilter{Conditions: []ActivityCondition{
			cond(ActivityColAction, "commit"), cond(ActivityColLevel, "debug")}}, "match nothing"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckActivityFilter(tc.f, nil)
			if len(got) == 0 {
				t.Fatalf("accepted a filter that selects nothing")
			}
			if !strings.Contains(strings.Join(got, "\n"), tc.want) {
				t.Errorf("message %q does not contain %q", got, tc.want)
			}
		})
	}
}

// The controls. Without them the test above passes against a checker that
// refuses every filter, which would make the statement unusable.
func TestCheckActivityFilterAcceptsSoundFilters(t *testing.T) {
	for _, tc := range []struct {
		name string
		f    ActivityFilter
	}{
		{"the reported case", ActivityFilter{Conditions: []ActivityCondition{
			cond(ActivityColAction, "log"), cond(ActivityColLevel, "debug")}}},
		{"level alone, no action named", ActivityFilter{Conditions: []ActivityCondition{
			cond(ActivityColLevel, "debug", "trace")}}},
		{"multi-word action", ActivityFilter{Conditions: []ActivityCondition{
			cond(ActivityColAction, "call javascript action")}}},
		{"caption LIKE", ActivityFilter{Conditions: []ActivityCondition{
			{Column: ActivityColCaption, Like: true, Values: []string{"%TODO%"}}}}},
		{"disabled state", ActivityFilter{Conditions: []ActivityCondition{cond(ActivityColDisabled, "true")}}},
		{"negated action beside level", ActivityFilter{Conditions: []ActivityCondition{
			{Column: ActivityColAction, Negate: true, Values: []string{"commit"}},
			cond(ActivityColLevel, "debug")}}},
		{"column case does not matter", ActivityFilter{Conditions: []ActivityCondition{cond("ACTION", "log")}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckActivityFilter(tc.f, nil); len(got) != 0 {
				t.Errorf("refused a sound filter: %v", got)
			}
		})
	}
}

// A storage label cannot be confirmed without a project — `check` has no
// registry — so it is ACCEPTED there and resolved at exec. Refusing it would
// make `check` reject a script `exec` runs, which is worse than a late error.
func TestCheckActivityFilterAcceptsAStorageLabelWithoutAProject(t *testing.T) {
	f := ActivityFilter{Conditions: []ActivityCondition{cond(ActivityColAction, "SendEmailAction")}}
	if got := CheckActivityFilter(f, map[string]bool{"Microflows$SendEmailAction": true}); len(got) != 0 {
		t.Errorf("with the registry, a real storage label was refused: %v", got)
	}
	// Without the registry it is reported, because nothing can distinguish it
	// from a typo — and the message says what it accepts.
	got := CheckActivityFilter(f, nil)
	if len(got) == 0 {
		t.Fatal("without a registry an unverifiable action word was accepted silently")
	}
	if !strings.Contains(got[0], "CATALOG.ACTIVITIES") {
		t.Errorf("the message does not point at how to find the right word: %q", got[0])
	}
}

// The level-pairing message must echo the word the author WROTE, not the
// resolved $Type: `action = Microflows$CommitAction` names something they never
// typed and cannot search their script for.
func TestLevelPairingMessageEchoesTheWrittenWord(t *testing.T) {
	f := ActivityFilter{Conditions: []ActivityCondition{
		cond(ActivityColAction, "commit"), cond(ActivityColLevel, "debug")}}
	got := strings.Join(CheckActivityFilter(f, nil), "\n")
	if !strings.Contains(got, "`action = commit`") {
		t.Errorf("message does not quote the written word: %q", got)
	}
	if strings.Contains(got, "Microflows$") {
		t.Errorf("message leaks the storage name: %q", got)
	}
}
