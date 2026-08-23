// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// TestMDL067_BareCommitNote checks the migration note fires on exactly the
// ambiguous form and nothing else.
//
// The distinction it has to make is the whole reason ast.MfCommitStmt records
// ExplicitWithEvents: `commit $X with events;` and `commit $X;` produce the same
// activity, but only the second one leaves the author's intent unknown. Noting
// the first would fire on the 35 occurrences in mxcli's own examples that were
// already explicit — and on every user script that had spelled it out.
func TestMDL067_BareCommitNote(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantNote bool
		wantWord string
	}{
		{
			name:     "bare commit is ambiguous and gets the note",
			body:     "COMMIT $Item;",
			wantNote: true,
			wantWord: "1 commit activity uses",
		},
		{
			name:     "explicit WITH EVENTS says what it wants",
			body:     "COMMIT $Item WITH EVENTS;",
			wantNote: false,
		},
		{
			name:     "explicit WITHOUT EVENTS says what it wants",
			body:     "COMMIT $Item WITHOUT EVENTS;",
			wantNote: false,
		},
		{
			name:     "one note per microflow, counting the bare ones",
			body:     "COMMIT $Item;\n  COMMIT $Item WITHOUT EVENTS;\n  COMMIT $Item;",
			wantNote: true,
			wantWord: "2 commit activities use",
		},
		{
			name:     "a bare commit inside a loop still counts",
			body:     "LOOP $Item IN $Items\n  BEGIN\n    COMMIT $Item;\n  END LOOP;",
			wantNote: true,
			wantWord: "1 commit activity uses",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "CREATE MICROFLOW Test.MF ($Item: Test.Item, $Items: LIST OF Test.Item)\nBEGIN\n  " +
				tt.body + "\nEND;"
			prog, errs := visitor.Build(src)
			if len(errs) > 0 {
				t.Fatalf("parse: %v", errs)
			}

			var note *linter.Violation
			violations := ValidateProgram(prog, "")
			for i := range violations {
				if violations[i].RuleID == "MDL067" {
					note = &violations[i]
					break
				}
			}

			if !tt.wantNote {
				if note != nil {
					t.Fatalf("unexpected MDL067: %s", note.Message)
				}
				return
			}
			if note == nil {
				t.Fatal("expected MDL067 and got none")
			}
			if note.Severity != linter.SeverityInfo {
				t.Errorf("severity = %v, want info — the note must not block exec or check", note.Severity)
			}
			if !strings.Contains(note.Message, tt.wantWord) {
				t.Errorf("message = %q, want it to contain %q", note.Message, tt.wantWord)
			}
		})
	}
}
