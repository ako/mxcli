// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance §5: `Title` parses inside `create entity` but is
// rejected in a `grant … write (…)` list, and mxcli's hint offered three
// renames. The app renamed a stored attribute to RequestTitle — a schema change
// — when quoting it would have cost nothing and kept the name.
package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

const grantWith = `grant Administration.Administrator on M.Doc ( read (%s) );`

func hintFor(t *testing.T, name string) string {
	t.Helper()
	_, errs := Build(strings.Replace(grantWith, "%s", name, 1))
	if len(errs) == 0 {
		t.Fatalf("expected a parse error for %q used bare", name)
	}
	return errsText(errs)
}

func TestParserKeywordHintOffersQuoting(t *testing.T) {
	// The fix. Quoting is the remedy the grammar itself names — QUOTED_IDENTIFIER
	// is in the expectation set the raw ANTLR message prints — and it leaves the
	// name intact.
	got := hintFor(t, "Title")
	if !strings.Contains(got, `"Title"`) {
		t.Errorf("hint does not show the quoted form:\n%s", got)
	}
	if strings.Contains(got, "add underscore suffix") || strings.Contains(got, "MyTitle") {
		t.Errorf("hint still recommends renaming a name that quoting rescues:\n%s", got)
	}
}

func TestQuotingActuallyParses(t *testing.T) {
	// The control, and the one that makes the advice above honest: a hint that
	// recommends quoting is only worth printing if the quoted form parses. Without
	// this, the test above would pass against a suggestion that does not work.
	if _, errs := Build(strings.Replace(grantWith, "%s", `"Title"`, 1)); len(errs) != 0 {
		t.Fatalf("the hint recommends quoting, but the quoted form does not parse: %v", errs)
	}
}

func TestPlatformReservedHintDoesNotOfferQuoting(t *testing.T) {
	// The other half. `Type` is reserved by MENDIX, so the check strips the quotes
	// and rejects the bare name with CE7247 — quoting parses and then fails the
	// build, which is a worse outcome than the rename it replaced.
	got := hintFor(t, "Type")
	if strings.Contains(got, "Quote it") {
		t.Errorf("hint offers quoting for a platform-reserved name:\n%s", got)
	}
	for _, want := range []string{"CE7247", "TypeValue"} {
		if !strings.Contains(got, want) {
			t.Errorf("hint does not mention %q:\n%s", want, got)
		}
	}
}

func TestEveryHintedKeywordIsClassified(t *testing.T) {
	// The two branches are chosen by types.IsPlatformReserved, so the hint list and
	// the platform list must not drift apart silently. Measured when this was
	// written: 38 of the 41 keywords are rescued by quoting, and exactly three
	// (Type, Default, Owner) are not — the old hint was wrong for the 38 and right
	// for the 3 by accident.
	quotable, renameOnly := 0, []string{}
	for _, k := range hintedKeywords() {
		if types.IsPlatformReserved(k) {
			renameOnly = append(renameOnly, k)
			continue
		}
		quotable++
	}
	if quotable == 0 || len(renameOnly) == 0 {
		t.Fatalf("expected both branches to be reachable; quotable=%d renameOnly=%v", quotable, renameOnly)
	}
	// Not an exact-count assertion — the keyword list is allowed to grow — but a
	// new keyword that is platform-reserved should be a deliberate addition.
	for _, k := range renameOnly {
		if h := reservedKeywordHint(k); !strings.Contains(h, "CE7247") {
			t.Errorf("%s is platform-reserved but its hint does not say so:\n%s", k, h)
		}
	}
	t.Logf("quotable=%d rename-only=%v", quotable, renameOnly)
}
