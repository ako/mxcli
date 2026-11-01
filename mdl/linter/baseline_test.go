// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1455 (ako/mxcli#1328): on a mature app lint reports a large
// existing backlog, so its totals say nothing about the change just made. A
// baseline records today's findings; a later run reports only what is new.

func v(rule, module, doc, id, msg string) Violation {
	return Violation{RuleID: rule, Severity: SeverityWarning, Message: msg,
		Location: Location{Module: module, DocumentType: "microflow", DocumentName: doc, DocumentID: id}}
}

func roundTrip(t *testing.T, findings []Violation) *Baseline {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lint-baseline.json")
	if err := WriteBaseline(path, findings); err != nil {
		t.Fatalf("WriteBaseline: %v", err)
	}
	b, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	return b
}

func messages(vs []Violation) string {
	var out []string
	for _, x := range vs {
		out = append(out, x.RuleID+" "+x.Location.QualifiedName()+" "+x.Message)
	}
	return strings.Join(out, " | ")
}

func TestBaselineReportsOnlyNewFindings(t *testing.T) {
	old := []Violation{
		v("QUAL001", "Sales", "ACT_A", "id-a", "ACT_A has no documentation"),
		v("CONV011", "Sales", "ACT_A", "id-a", "loop over $Orders commits inside"),
		v("CONV011", "Sales", "ACT_A", "id-a", "loop over $Lines commits inside"),
		v("SEC001", "Sales", "ACT_Gone", "id-gone", "fixed since"),
	}
	b := roundTrip(t, old)

	now := []Violation{
		// Unchanged.
		v("QUAL001", "Sales", "ACT_A", "id-a", "ACT_A has no documentation"),
		v("CONV011", "Sales", "ACT_A", "id-a", "loop over $Orders commits inside"),
		v("CONV011", "Sales", "ACT_A", "id-a", "loop over $Lines commits inside"),
		// A third finding of the same rule in the same document is new, even
		// though the rule and the document are already in the baseline.
		v("CONV011", "Sales", "ACT_A", "id-a", "loop over $Items commits inside"),
		// A new document.
		v("QUAL001", "Sales", "ACT_B", "id-b", "ACT_B has no documentation"),
	}
	got := b.Compare(now)
	if want := "CONV011 Sales.ACT_A loop over $Items commits inside | QUAL001 Sales.ACT_B ACT_B has no documentation"; messages(got.New) != want {
		t.Errorf("new = %q,\nwant %q", messages(got.New), want)
	}
	if got.Fixed != 1 || got.Unchanged != 3 {
		t.Errorf("fixed = %d, unchanged = %d; want 1 and 3", got.Fixed, got.Unchanged)
	}
}

// The reason teams' own SARIF fingerprints broke: a rule's wording changes.
// A reworded finding in the same place is the same finding.
func TestBaselineSurvivesARewordedMessage(t *testing.T) {
	b := roundTrip(t, []Violation{v("QUAL001", "Sales", "ACT_A", "id-a", "Microflow has no documentation")})
	got := b.Compare([]Violation{v("QUAL001", "Sales", "ACT_A", "id-a", "ACT_A has no documentation — add a description")})
	if len(got.New) != 0 || got.Unchanged != 1 {
		t.Errorf("a reworded message read as new: new = %q, unchanged = %d", messages(got.New), got.Unchanged)
	}
}

// A renamed document keeps its id, so its findings are not new.
func TestBaselineFollowsADocumentByItsID(t *testing.T) {
	b := roundTrip(t, []Violation{v("QUAL001", "Sales", "ACT_Old", "id-a", "no documentation")})
	if got := b.Compare([]Violation{v("QUAL001", "Sales", "ACT_New", "id-a", "no documentation")}); len(got.New) != 0 {
		t.Errorf("a renamed document's finding read as new: %q", messages(got.New))
	}
	// Control: without an id the name is the key, and a different document is new.
	b = roundTrip(t, []Violation{v("QUAL001", "Sales", "ACT_Old", "", "no documentation")})
	if got := b.Compare([]Violation{v("QUAL001", "Sales", "ACT_Other", "", "no documentation")}); len(got.New) != 1 {
		t.Errorf("a finding on another document was matched to the baseline: new = %q", messages(got.New))
	}
}

// A rule that failed to run says nothing about the model: it is never
// absorbed by a baseline, or a broken rule would read as "nothing new".
func TestBaselineNeverAbsorbsARuleFailure(t *testing.T) {
	fail := Violation{RuleID: "CUST01", Severity: SeverityError, Message: "rule failed", RuleFailure: true}
	b := roundTrip(t, []Violation{fail})
	if got := b.Compare([]Violation{fail}); len(got.New) != 1 {
		t.Errorf("a rule failure was absorbed by the baseline: %q", messages(got.New))
	}
}

func TestLoadBaselineRejectsAFileItCannotRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadBaseline(path); err == nil {
		t.Error("a corrupt baseline loaded; a gate must not silently compare against nothing")
	}
	if _, err := LoadBaseline(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Error("a missing baseline loaded; it must be an error, or every finding would read as new without saying why")
	}
}
