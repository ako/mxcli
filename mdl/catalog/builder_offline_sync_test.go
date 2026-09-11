// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// offlineFixture is a profile carrying one config per sync mode, plus the two
// cases that must be skipped.
func offlineFixture() *types.NavigationProfile {
	return &types.NavigationProfile{
		Name: "TabletOffline", Kind: "TabletOffline",
		OfflineEntities: []*types.NavOfflineEntity{
			{Entity: "Sales.Order", SyncMode: "All"},
			{Entity: "Sales.Trip", SyncMode: "Constrained", Constraint: "[Distance > 0]"},
			{Entity: "Sales.Setting", SyncMode: "Online"},
			{Entity: "Sales.Audit", SyncMode: "Never"},
			{Entity: "Sales.Lookup", SyncMode: "None"},
			{Entity: "Sales.Draft", SyncMode: "NoneAndPreserveData", CompatibilityMode: true},
			// An entity with no name is not a config; it is a hole in the
			// document and must produce neither a row nor an edge.
			{Entity: "", SyncMode: "All"},
		},
	}
}

// The count told you HOW MANY and nothing else. These are the rows that make
// "which entities does this profile sync, and how?" answerable — the first
// question anyone auditing an offline app asks.
func TestOfflineConfigsAreIndexedAsRows(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	p := offlineFixture()
	for _, oe := range p.OfflineEntities {
		if oe.Entity == "" {
			continue // mirrors the builder's skip
		}
		if _, err := db.Exec(
			`INSERT INTO offline_entity_configs_data (ProfileName, ProfileKind,
			   EntityQualifiedName, ModuleName, SyncMode, XPathConstraint,
			   CompatibilityMode, ProjectId, SnapshotId)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'p', 's')`,
			p.Name, p.Kind, oe.Entity, moduleOf(oe.Entity), oe.SyncMode,
			oe.Constraint, boolToInt(oe.CompatibilityMode)); err != nil {
			t.Fatalf("seed %s: %v", oe.Entity, err)
		}
	}

	res, err := cat.Query(`SELECT EntityQualifiedName, ModuleName, SyncMode,
		XPathConstraint, CompatibilityMode FROM offline_entity_configs ORDER BY EntityQualifiedName`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 6 {
		t.Fatalf("got %d rows, want 6 (the nameless config must be skipped)", res.Count)
	}

	// Every mode must survive verbatim. A mode silently rewritten — to a
	// caption, or to a default — would make the table agree with itself and
	// disagree with the model.
	want := map[string]string{
		"Sales.Order": "All", "Sales.Trip": "Constrained", "Sales.Setting": "Online",
		"Sales.Audit": "Never", "Sales.Lookup": "None", "Sales.Draft": "NoneAndPreserveData",
	}
	for _, row := range res.Rows {
		entity := row[0].(string)
		if got, expected := row[2].(string), want[entity]; got != expected {
			t.Errorf("%s: SyncMode = %q, want %q", entity, got, expected)
		}
		if row[1].(string) != "Sales" {
			t.Errorf("%s: ModuleName = %v, want Sales", entity, row[1])
		}
		if entity == "Sales.Trip" && row[3].(string) != "[Distance > 0]" {
			t.Errorf("constraint lost: %v", row[3])
		}
		// CompatibilityMode is unauthorable but stored; a flag invisible to
		// every query is one nobody discovers until it matters.
		if entity == "Sales.Draft" && row[4].(int64) != 1 {
			t.Errorf("CompatibilityMode not indexed: %v", row[4])
		}
	}
}

// moduleOf must take everything before the FIRST dot. The graph views derive a
// module the same way, and a variant taking the last dot would silently
// regroup every node — which is why the helper is shared rather than copied.
func TestModuleOfTakesTheFirstDot(t *testing.T) {
	for in, want := range map[string]string{
		"Sales.Order":        "Sales",
		"Sales.Order.Status": "Sales",
		"System":             "System",
		"":                   "",
	} {
		if got := moduleOf(in); got != want {
			t.Errorf("moduleOf(%q) = %q, want %q", in, got, want)
		}
	}
}

// Every sync mode gets an edge, including the ones that download nothing.
//
// The tempting narrowing — edges only for ALL and Constrained, the modes that
// actually sync — is wrong, and this is the test that says so. A profile with
// `sync Sales.Audit never` still NAMES that entity, so renaming or dropping it
// leaves the config dangling. Emitting only the downloading modes would hide
// exactly the cases the reference edge exists to reveal.
func TestSyncEdgeCoversEveryModeIncludingTheQuietOnes(t *testing.T) {
	cat, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	p := offlineFixture()
	for _, oe := range p.OfflineEntities {
		if oe.Entity == "" {
			continue
		}
		if _, err := db.Exec(
			`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId,
			   TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
			 VALUES ('NAVIGATION', '', ?, 'ENTITY', '', ?, ?, '', 'p', 's')`,
			"Navigation."+p.Name, oe.Entity, RefKindSync); err != nil {
			t.Fatalf("seed edge %s: %v", oe.Entity, err)
		}
	}

	res, err := cat.Query(`SELECT TargetName FROM refs WHERE RefKind = 'sync' ORDER BY TargetName`)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count != 6 {
		t.Fatalf("got %d sync edges, want 6 — one per named config, quiet modes included", res.Count)
	}

	// Name the quiet ones explicitly, so a future narrowing has to delete an
	// assertion that says why rather than watch a total change.
	got := map[string]bool{}
	for _, row := range res.Rows {
		got[row[0].(string)] = true
	}
	for _, quiet := range []string{"Sales.Audit", "Sales.Lookup", "Sales.Setting", "Sales.Draft"} {
		if !got[quiet] {
			t.Errorf("%s has no sync edge; a mode that downloads nothing still names the entity", quiet)
		}
	}
}

func TestRefKindSyncIsItsOwnKind(t *testing.T) {
	// Reusing an existing kind would make `show references` say the wrong
	// thing about how the profile uses the entity.
	for _, other := range []string{
		RefKindDatasource, RefKindParameter, RefKindRetrieve, RefKindHomePage, RefKindMenuItem,
	} {
		if RefKindSync == other {
			t.Errorf("RefKindSync collides with %q", other)
		}
	}
	if RefKindSync != "sync" {
		t.Errorf("RefKindSync = %q; the value appears in user-facing output", RefKindSync)
	}
}
