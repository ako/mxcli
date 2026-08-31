// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ledger #137: `create or modify translations in Ledger for nl_NL` reported
//
//	Set 212 nl_NL translation(s) across 20 document(s)
//
// and the app's page text switched to Dutch while the MENU did not. The
// navigation is a PROJECT-level document, not a module one, so `in Ledger`
// never reaches it. Dropping the scope found 3.6× as much (151 strings scoped
// against 546 unscoped) and landed 65 more translations across 22 further
// documents.
//
// Nothing warned. The scoped run reported success and a document count, and the
// count is the only thing that would have given it away — if you knew what
// number to expect. `or modify` makes the correction cheap, but the first run
// has to be NOTICED to be corrected.
//
// So a scoped run now says what it did not consider: the entries of this very
// file that match a string in a unit the scope excludes. That is the actionable
// number — not "the project has more strings", which is true of every scoped
// run and would be noise.

func TestOutOfScope_NamesEntriesTheScopeExcluded(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"))),      // A — in scope
		unit(t, text(tr("en_US", "Dashboard"))), // B — out of scope (the navigation)
		unit(t, text(tr("en_US", "Budgets"))),   // C — out of scope
	)
	inA := Scope(func(id model.ID) bool { return id == model.ID("A") })
	dict := Dictionary{"Save": "Opslaan", "Dashboard": "Overzicht", "Budgets": "Budgetten"}

	missed, err := OutOfScope(p, "en_US", inA, dict)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Budgets", "Dashboard"}; !reflect.DeepEqual(missed, want) {
		t.Errorf("missed = %v, want %v — a scoped run must say which of the file's own entries "+
			"it did not reach", missed, want)
	}
}

// CONTROL 1: an UNSCOPED run reaches everything, so there is nothing to report.
// A warning on every run is a warning nobody reads.
func TestOutOfScope_UnscopedRunReportsNothing(t *testing.T) {
	p := newProject(t, unit(t, text(tr("en_US", "Save"))), unit(t, text(tr("en_US", "Dashboard"))))
	missed, err := OutOfScope(p, "en_US", nil, Dictionary{"Save": "Opslaan", "Dashboard": "Overzicht"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missed) != 0 {
		t.Errorf("an unscoped run excludes nothing; got %v", missed)
	}
}

// CONTROL 2: a scoped run whose file happens to name nothing outside the scope
// is silent. The report is about THIS file's entries, not about the project
// having other strings — otherwise every per-module file would warn forever,
// which is the shape the `in <Module>` scoping exists to support.
func TestOutOfScope_SilentWhenTheFileNamesNothingOutside(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"))),
		unit(t, text(tr("en_US", "Dashboard"))),
	)
	inA := Scope(func(id model.ID) bool { return id == model.ID("A") })
	missed, err := OutOfScope(p, "en_US", inA, Dictionary{"Save": "Opslaan"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missed) != 0 {
		t.Errorf("the file names nothing outside the scope; got %v", missed)
	}
}

// A source string present BOTH inside and outside the scope is still reported:
// the scoped run translated the occurrences it reached and left the others, and
// a half-translated string is exactly the symptom that was reported.
func TestOutOfScope_ReportsAStringPresentOnBothSides(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"))),
		unit(t, text(tr("en_US", "Save"))),
	)
	inA := Scope(func(id model.ID) bool { return id == model.ID("A") })
	missed, err := OutOfScope(p, "en_US", inA, Dictionary{"Save": "Opslaan"})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Save"}; !reflect.DeepEqual(missed, want) {
		t.Errorf("missed = %v, want %v — the untouched occurrence is the reported symptom", missed, want)
	}
}

// The drift warning's premise is "no text has this as its source", and for an
// out-of-scope entry that is FALSE — the text exists, the scope just did not
// reach it. The executor subtracts one set from the other; this is the shape of
// that subtraction, kept beside the function it is about.
func TestOutOfScope_SeparatesFromRealDrift(t *testing.T) {
	p := newProject(t,
		unit(t, text(tr("en_US", "Save"))),      // A — in scope
		unit(t, text(tr("en_US", "Dashboard"))), // B — out of scope
	)
	inA := Scope(func(id model.ID) bool { return id == model.ID("A") })
	dict := Dictionary{
		"Dashboard": "Overzicht", // exists, out of scope — NOT drift
		"Gone":      "Weg",       // exists nowhere — real drift
	}

	missed, err := OutOfScope(p, "en_US", inA, dict)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"Dashboard"}; !reflect.DeepEqual(missed, want) {
		t.Fatalf("missed = %v, want %v", missed, want)
	}
	// "Gone" must NOT be here: it matched nothing anywhere, so the drift
	// heuristic is the right one to explain it.
	for _, m := range missed {
		if m == "Gone" {
			t.Error("a key that matches nothing anywhere is drift, not a scoping miss")
		}
	}
}
