// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// seedRefTargets builds an in-memory catalog holding one widget edge (stored
// SHOUTED, as widget_definitions_data holds MDL names) and one ordinary
// module-qualified target, so the exact path and the fallback are exercised
// against the same catalog.
func seedRefTargets(t *testing.T, targets ...string) *ExecContext {
	t.Helper()
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	for _, tgt := range targets {
		if _, err := cat.CatalogDB().Exec(
			`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ProjectId, SnapshotId)
			 VALUES ('PAGE', '', 'Sales.OrderList', 'WIDGET', '', ?, 'widget', 'p', 's')`, tgt); err != nil {
			t.Fatalf("seed %q: %v", tgt, err)
		}
	}
	return &ExecContext{Catalog: cat, Output: &bytes.Buffer{}}
}

func TestResolveReferenceTarget(t *testing.T) {
	ctx := seedRefTargets(t, "COMBOBOX", "Sales.Order")

	cases := []struct {
		name       string
		typed      string
		want       string
		wantLoose  bool
		reasonWhen string
	}{
		{"exact widget name", "COMBOBOX", "COMBOBOX", false,
			"an exact match must be returned untouched, so no existing answer changes"},
		{"lower-case widget name", "combobox", "COMBOBOX", true,
			"the spelling every MDL example uses must find the stored SHOUTED name"},
		{"mixed-case widget name", "ComboBox", "COMBOBOX", true,
			"MDL keywords are case-insensitive, so any casing must resolve"},
		{"exact qualified name", "Sales.Order", "Sales.Order", false,
			"an ordinary target keeps exact-match behaviour"},
		{"wrong-case qualified name", "sales.order", "Sales.Order", true,
			"the fallback is not widget-specific; it can only turn an empty answer into a right one"},
		{"genuine typo", "Sales.Ordr", "Sales.Ordr", false,
			"a name that matches nothing in any casing must not be rewritten to something else"},
		{"unknown widget", "gallery", "gallery", false,
			"a widget with no edges must stay unresolved rather than borrow another's name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, loose := resolveReferenceTarget(ctx, tc.typed)
			if got != tc.want || loose != tc.wantLoose {
				t.Errorf("resolveReferenceTarget(%q) = (%q, %v), want (%q, %v) — %s",
					tc.typed, got, loose, tc.want, tc.wantLoose, tc.reasonWhen)
			}
		})
	}
}

// Two targets differing only in case are a question this cannot answer, so it
// must decline rather than pick one. Without the Count != 1 guard it would
// silently return whichever the database ordered first.
func TestResolveReferenceTarget_AmbiguousCaseDeclines(t *testing.T) {
	ctx := seedRefTargets(t, "Sales.Order", "sales.ORDER")

	got, loose := resolveReferenceTarget(ctx, "SALES.order")
	if loose || got != "SALES.order" {
		t.Errorf("resolveReferenceTarget = (%q, %v), want (%q, false) — two case-variant targets must not be guessed between",
			got, loose, "SALES.order")
	}

	// Control: with only one variant present, the same input DOES resolve — so
	// the decline above is the ambiguity guard, not a broken lookup.
	single := seedRefTargets(t, "Sales.Order")
	if got, loose := resolveReferenceTarget(single, "SALES.order"); !loose || got != "Sales.Order" {
		t.Errorf("control: resolveReferenceTarget = (%q, %v), want (\"Sales.Order\", true)", got, loose)
	}
}

// The resolved spelling is reported, because a user who typed `combobox` and
// got results under `COMBOBOX` should be able to see which name matched.
func TestReportResolvedTarget(t *testing.T) {
	var buf bytes.Buffer
	ctx := &ExecContext{Output: &buf}

	reportResolvedTarget(ctx, "COMBOBOX", "COMBOBOX", false)
	if buf.Len() != 0 {
		t.Errorf("exact match printed %q, want nothing", buf.String())
	}

	reportResolvedTarget(ctx, "combobox", "COMBOBOX", true)
	if got := buf.String(); !strings.Contains(got, "COMBOBOX") {
		t.Errorf("loose match printed %q, want it to name COMBOBOX", got)
	}
}

// A nil catalog must not panic — `show references` reaches here only after
// ensureCatalog, but the helper is small enough to be called elsewhere.
func TestResolveReferenceTarget_NoCatalog(t *testing.T) {
	if got, loose := resolveReferenceTarget(&ExecContext{}, "combobox"); got != "combobox" || loose {
		t.Errorf("no catalog = (%q, %v), want (\"combobox\", false)", got, loose)
	}
	if got, loose := resolveReferenceTarget(nil, "combobox"); got != "combobox" || loose {
		t.Errorf("nil ctx = (%q, %v), want (\"combobox\", false)", got, loose)
	}
}
