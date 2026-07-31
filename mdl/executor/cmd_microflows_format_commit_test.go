// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#779: DESCRIBE MICROFLOW could not distinguish a create/change
// activity with Commit enabled from one without — both rendered identically, so
// transaction boundaries were invisible to automated analysis and to anyone not
// willing to open Studio Pro.
//
// The round-trip half matters as much as the display: whatever DESCRIBE emits must
// re-parse to the same flag, or describe → edit → re-exec silently rewrites the
// project's commit boundaries.
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestCommitModifier(t *testing.T) {
	tests := []struct {
		in   microflows.CommitType
		want string
	}{
		{microflows.CommitTypeYes, " commit"},
		{microflows.CommitTypeYesWithoutEvents, " commit without events"},
		// The default is omitted, so existing describe output is unchanged for the
		// overwhelmingly common case and enabling it is a one-word diff.
		{microflows.CommitTypeNo, ""},
		{microflows.CommitType(""), ""},
	}
	for _, tc := range tests {
		if got := commitModifier(tc.in); got != tc.want {
			t.Errorf("commitModifier(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatAction_CreateEmitsCommit(t *testing.T) {
	tests := []struct {
		name   string
		commit microflows.CommitType
		want   string
	}{
		{"default omits the modifier", microflows.CommitTypeNo,
			"$Order = create Sales.Order (Number = 'X');"},
		{"commit", microflows.CommitTypeYes,
			"$Order = create Sales.Order (Number = 'X') commit;"},
		{"commit without events", microflows.CommitTypeYesWithoutEvents,
			"$Order = create Sales.Order (Number = 'X') commit without events;"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &microflows.CreateObjectAction{
				EntityQualifiedName: "Sales.Order",
				OutputVariable:      "Order",
				Commit:              tc.commit,
				InitialMembers: []*microflows.MemberChange{
					{AttributeQualifiedName: "Sales.Order.Number", Value: "'X'"},
				},
			}
			if got := formatAction(nil, a, nil, nil); got != tc.want {
				t.Errorf("formatAction =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

func TestFormatAction_CreateNoMembersEmitsCommit(t *testing.T) {
	a := &microflows.CreateObjectAction{
		EntityQualifiedName: "Sales.Order",
		OutputVariable:      "Order",
		Commit:              microflows.CommitTypeYes,
	}
	want := "$Order = create Sales.Order commit;"
	if got := formatAction(nil, a, nil, nil); got != want {
		t.Errorf("formatAction = %q, want %q", got, want)
	}
}

func TestFormatAction_ChangeEmitsCommit(t *testing.T) {
	tests := []struct {
		name    string
		commit  microflows.CommitType
		refresh bool
		want    string
	}{
		{"default", microflows.CommitTypeNo, false, "change $Order (Status = 'Paid');"},
		{"commit", microflows.CommitTypeYes, false, "change $Order (Status = 'Paid') commit;"},
		{"commit without events", microflows.CommitTypeYesWithoutEvents, false,
			"change $Order (Status = 'Paid') commit without events;"},
		// Canonical order: commit precedes refresh, matching the grammar.
		{"commit and refresh", microflows.CommitTypeYes, true,
			"change $Order (Status = 'Paid') commit refresh;"},
		{"refresh alone", microflows.CommitTypeNo, true,
			"change $Order (Status = 'Paid') refresh;"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := &microflows.ChangeObjectAction{
				ChangeVariable:  "Order",
				Commit:          tc.commit,
				RefreshInClient: tc.refresh,
				Changes: []*microflows.MemberChange{
					{AttributeQualifiedName: "Sales.Order.Status", Value: "'Paid'"},
				},
			}
			if got := formatAction(nil, a, nil, nil); got != tc.want {
				t.Errorf("formatAction =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// TestFormatAction_CommitRoundTrips is the acceptance criterion from #779: what
// DESCRIBE emits must parse back to the same flag. A formatter that renders a
// modifier the grammar cannot read would be worse than not rendering it at all.
func TestFormatAction_CommitRoundTrips(t *testing.T) {
	for _, commit := range []microflows.CommitType{
		microflows.CommitTypeNo,
		microflows.CommitTypeYes,
		microflows.CommitTypeYesWithoutEvents,
	} {
		t.Run(string(commit), func(t *testing.T) {
			emitted := formatAction(nil, &microflows.CreateObjectAction{
				EntityQualifiedName: "Sales.Order",
				OutputVariable:      "Order",
				Commit:              commit,
				InitialMembers: []*microflows.MemberChange{
					{AttributeQualifiedName: "Sales.Order.Number", Value: "'X'"},
				},
			}, nil, nil)

			src := "create microflow M.F ()\nbegin\n  " + emitted + "\nend;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("DESCRIBE output does not re-parse: %q\n%v", emitted, errs)
			}
			mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
			got := mf.Body[0].(*ast.CreateObjectStmt).Commit

			want := ast.CommitNo
			switch commit {
			case microflows.CommitTypeYes:
				want = ast.CommitYes
			case microflows.CommitTypeYesWithoutEvents:
				want = ast.CommitYesWithoutEvents
			}
			if got != want {
				t.Errorf("round-trip lost the flag: emitted %q, re-parsed as %v, want %v", emitted, got, want)
			}
			// Guard the reported symptom directly: two different flags must not
			// produce identical MDL.
			if commit != microflows.CommitTypeNo && !strings.Contains(emitted, "commit") {
				t.Errorf("Commit=%q rendered without a commit modifier: %q", commit, emitted)
			}
		})
	}
}
