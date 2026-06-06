package enginecompare

import (
	"strings"
	"testing"
)

// TestWriteParity_Index validates the modelsdk engine's index serialization
// against the AUTHORITATIVE Studio-Pro 11.x structure captured via the MCP
// backend (mx-test-projects/test7-app: MxcliDiskProbe.IdxProbe), NOT against
// legacy — legacy is known wrong here (SortOrder string, marker 3, no GUID /
// IncludeInOffline / AssociationPointer / Type). See
// docs/plans/2026-06-05-adopt-modelsdk-engine.md "Index spec resolved via MCP".
func TestWriteParity_Index(t *testing.T) {
	const s = "CREATE PERSISTENT ENTITY MyFirstModule.IdxTest " +
		"( Code: string(20), Rank: integer ) index (Code, Rank desc)"

	mp := copyProject(t)
	if _, e := Run(ModelSDK, mp, s); e != nil {
		t.Fatalf("modelsdk: %v", e)
	}
	msd, e := EntityCanonBSON(mp, "MyFirstModule", "IdxTest")
	if e != nil {
		t.Fatalf("msd: %v", e)
	}

	// The authoritative EntityIndex block: marker 2 on the IndexedAttribute list,
	// Ascending(bool)+Type("Normal")+AttributePointer+AssociationPointer per
	// segment, GUID + IncludeInOffline=false on the index. Keys are canonical
	// (alphabetical) and IDs/binaries masked.
	const wantIndex = `Indexes=[{"$numberInt":"3"},` +
		`{$ID=<masked>,$Type="DomainModels$EntityIndex",` +
		`Attributes=[{"$numberInt":"2"},` +
		`{$ID=<masked>,$Type="DomainModels$IndexedAttribute",Ascending=true,AssociationPointer=<binary>,AttributePointer=<binary>,Type="Normal"},` +
		`{$ID=<masked>,$Type="DomainModels$IndexedAttribute",Ascending=false,AssociationPointer=<binary>,AttributePointer=<binary>,Type="Normal"}],` +
		`GUID=<masked>,IncludeInOffline=false}]`

	if !strings.Contains(msd, wantIndex) {
		t.Errorf("index serialization does not match Studio-Pro 11.x truth.\nwant substring: %s\ngot:            %s", wantIndex, msd)
	}

	// Document the legacy bug: legacy must NOT match the truth (proves why the
	// legacy-parity gate cannot validate indexes and the MCP capture was needed).
	lp := copyProject(t)
	if _, e := Run(Legacy, lp, s); e != nil {
		t.Fatalf("legacy: %v", e)
	}
	leg, e := EntityCanonBSON(lp, "MyFirstModule", "IdxTest")
	if e != nil {
		t.Fatalf("leg: %v", e)
	}
	if strings.Contains(leg, wantIndex) {
		t.Errorf("legacy unexpectedly matches Studio-Pro index truth — the legacy SortOrder bug may have been fixed; update this test")
	}
	if !strings.Contains(leg, "SortOrder") {
		t.Logf("note: legacy no longer emits SortOrder — legacy index fix may have landed:\n%s", leg)
	}
}
