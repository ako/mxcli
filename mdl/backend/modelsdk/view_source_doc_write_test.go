// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#583 — a view entity's OQL document was DELETED and RE-INSERTED on
// every CREATE OR MODIFY, under a fresh unit id, whether or not the query had
// changed. Two consequences, both measured against a real project:
//
//   - `exec` printed `Unchanged view entity: …` for a statement that rewrote the
//     OQL, because the elision check counts writes at the update choke points
//     and an InsertUnit is not one. The domain-model unit really was unchanged
//     (same attribute list), the OQL document really was written, and the report
//     believed the half it could see.
//   - the unit was replaced under a new GUID on every run, so the .mpr could
//     never come back clean in version control (#556 counted these units).
//
// Writing the document in place fixes both: the update path reconciles against
// what is stored (ADR-0008), so an unchanged query is elided and a changed one
// is counted.
package modelsdkbackend

import (
	"testing"
)

func TestWriteViewEntitySourceDocument_KeepsItsUnitAndReconciles(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}

	const oql = "from MyFirstModule.Thing as t select t.Name as Name"
	first, err := b.WriteViewEntitySourceDocument(mod.ID, "MyFirstModule", "ZzTotals", oql, "")
	if err != nil {
		t.Fatalf("WriteViewEntitySourceDocument (create): %v", err)
	}

	// Writing the SAME query again must keep the unit and land nothing.
	before := b.WriteStats()
	again, err := b.WriteViewEntitySourceDocument(mod.ID, "MyFirstModule", "ZzTotals", oql, "")
	if err != nil {
		t.Fatalf("WriteViewEntitySourceDocument (no-op): %v", err)
	}
	if again != first {
		t.Errorf("an unchanged rewrite replaced the unit: %s -> %s — the .mpr can never come back "+
			"clean in version control (ako/mxcli#556)", first, again)
	}
	if after := b.WriteStats(); after.Written != before.Written {
		t.Errorf("an unchanged rewrite landed a write (Written %d -> %d) — `exec` would report it as "+
			"Modified", before.Written, after.Written)
	}

	// Changing the query must keep the unit AND land a write, or ReportMutation
	// downgrades the verb to "Unchanged" for a statement that changed the model.
	before = b.WriteStats()
	changed, err := b.WriteViewEntitySourceDocument(mod.ID, "MyFirstModule", "ZzTotals",
		oql+" where t.Name != ''", "")
	if err != nil {
		t.Fatalf("WriteViewEntitySourceDocument (change): %v", err)
	}
	if changed != first {
		t.Errorf("a changed rewrite replaced the unit: %s -> %s", first, changed)
	}
	after := b.WriteStats()
	if after.Written == before.Written {
		t.Errorf("a changed OQL landed no counted write (Written stayed %d) — this is the reported "+
			"symptom: `exec` prints \"Unchanged view entity\" while `describe entity` shows the new query",
			before.Written)
	}

	// And the new query is what is stored, not just what was offered.
	id, err := b.FindViewEntitySourceDocumentID("MyFirstModule", "ZzTotals")
	if err != nil || id == "" {
		t.Fatalf("FindViewEntitySourceDocumentID: %v (id=%q)", err, id)
	}
	if id != first {
		t.Errorf("the stored document is a different unit: %s, want %s", id, first)
	}
}
