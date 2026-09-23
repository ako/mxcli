// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// eventFixture is an entity carrying one handler per moment/event combination
// that matters, plus the two cases that must be skipped.
func eventFixture() *domainmodel.Entity {
	e := &domainmodel.Entity{
		Name: "Order",
		EventHandlers: []*domainmodel.EventHandler{
			{Moment: "Before", Event: "Commit", MicroflowName: "Sales.BCO_Order_Validate",
				RaiseErrorOnFalse: true, PassEventObject: true},
			{Moment: "After", Event: "Commit", MicroflowName: "Sales.ACO_Order_Notify",
				PassEventObject: true},
			{Moment: "Before", Event: "Delete", MicroflowName: "Sales.BDE_Order_Guard",
				RaiseErrorOnFalse: true},
			{Moment: "After", Event: "Create", MicroflowName: "Sales.ACR_Order_Defaults"},
			// A handler naming no microflow is a hole in the document, not a
			// reference. A row for it would put an empty TargetName in refs.
			{Moment: "Before", Event: "Rollback", MicroflowName: ""},
			nil,
		},
	}
	e.ID = model.ID("entity-order")
	for i, eh := range e.EventHandlers {
		if eh != nil {
			eh.ID = model.ID(string(rune('a'+i)) + "-handler")
		}
	}
	return e
}

// The flag said "some exist" and nothing else — the same shape
// NavigationProfile.OfflineEntityCount had before CATALOG.OFFLINE_ENTITY_CONFIGS.
// Which moment, which event and which microflow is the whole question, and refs
// has no column for the first two.
func TestEntityEventHandlerRowsCarryMomentAndEvent(t *testing.T) {
	rows := entityEventHandlerRows(eventFixture(), "Sales")

	if len(rows) != 4 {
		t.Fatalf("got %d rows, want 4 — the microflow-less handler and the nil must be skipped", len(rows))
	}

	want := map[string][2]string{
		"Sales.BCO_Order_Validate": {"Before", "Commit"},
		"Sales.ACO_Order_Notify":   {"After", "Commit"},
		"Sales.BDE_Order_Guard":    {"Before", "Delete"},
		"Sales.ACR_Order_Defaults": {"After", "Create"},
	}
	for _, r := range rows {
		w, ok := want[r.microflow]
		if !ok {
			t.Errorf("unexpected row for %q", r.microflow)
			continue
		}
		if r.moment != w[0] || r.event != w[1] {
			t.Errorf("%s = %s %s, want %s %s", r.microflow, r.moment, r.event, w[0], w[1])
		}
		if r.entityQualifiedName != "Sales.Order" {
			t.Errorf("%s entity = %q, want Sales.Order", r.microflow, r.entityQualifiedName)
		}
		if r.moduleName != "Sales" {
			t.Errorf("%s module = %q, want Sales", r.microflow, r.moduleName)
		}
		delete(want, r.microflow)
	}
	for missing := range want {
		t.Errorf("no row for %s", missing)
	}

	// RaiseErrorOnFalse is the difference between a handler that can veto a
	// commit and one that only observes it. Dropping it would make the table
	// agree with itself and disagree with the model.
	for _, r := range rows {
		switch r.microflow {
		case "Sales.BCO_Order_Validate":
			if !r.raiseErrorOnFalse || !r.passEventObject {
				t.Errorf("BCO handler = raise:%v pass:%v, want both true", r.raiseErrorOnFalse, r.passEventObject)
			}
		case "Sales.ACO_Order_Notify":
			if r.raiseErrorOnFalse {
				t.Error("ACO handler must not raise on false — it runs after the commit")
			}
		}
	}

	// Mendix spells the rollback event with a capital B, which
	// generated/metamodel confirms and which disagrees with every neighbouring
	// enum in that file. The value is stored verbatim: normalising it here
	// would make the table agree with itself and disagree with the model.
	rb := &domainmodel.Entity{Name: "Order", EventHandlers: []*domainmodel.EventHandler{
		{Moment: "Before", Event: domainmodel.EventTypeRollback, MicroflowName: "Sales.BRB_Order"},
	}}
	if got := entityEventHandlerRows(rb, "Sales"); len(got) != 1 || got[0].event != "RollBack" {
		t.Errorf("rollback event stored as %q, want %q verbatim", got[0].event, "RollBack")
	}

	if rows := entityEventHandlerRows(nil, "Sales"); rows != nil {
		t.Errorf("nil entity = %v, want nil", rows)
	}
}

// The defect this fixes, stated as the query three tools ask. The control is
// the same catalog WITHOUT the event edges: it must report the handler
// microflow as dead, or the assertion below proves nothing.
func TestEventEdgeClearsTheDeadAssetVerdict(t *testing.T) {
	seed := func(t *testing.T, withEdges bool) []string {
		t.Helper()
		cat, err := New()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cat.Close() })
		db := cat.CatalogDB()

		if _, err := db.Exec(`INSERT INTO snapshots (SnapshotId, ProjectId) VALUES ('s','p')`); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(
			`INSERT INTO entities_data (Id, Name, QualifiedName, ModuleName, EntityType,
			   HasEventHandlers, ProjectId, SnapshotId)
			 VALUES ('entity-order', 'Order', 'Sales.Order', 'Sales', 'PERSISTENT', 1, 'p', 's')`); err != nil {
			t.Fatal(err)
		}

		rows := entityEventHandlerRows(eventFixture(), "Sales")
		for _, r := range rows {
			name := r.microflow[len("Sales."):]
			if _, err := db.Exec(
				`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName,
				   MicroflowType, ProjectId, SnapshotId)
				 VALUES (?, ?, ?, 'Sales', 'MICROFLOW', 'p', 's')`,
				r.microflow, name, r.microflow); err != nil {
				t.Fatal(err)
			}
			if !withEdges {
				continue
			}
			if _, err := db.Exec(
				`INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId,
				   TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
				 VALUES ('ENTITY', '', ?, 'MICROFLOW', '', ?, ?, 'Sales', 'p', 's')`,
				r.entityQualifiedName, r.microflow, RefKindEvent); err != nil {
				t.Fatal(err)
			}
		}

		res, err := cat.Query(
			`SELECT QualifiedName FROM graph_dead_assets WHERE ObjectType = 'MICROFLOW' ORDER BY QualifiedName`)
		if err != nil {
			t.Fatal(err)
		}
		var dead []string
		for _, row := range res.Rows {
			dead = append(dead, row[0].(string))
		}
		return dead
	}

	// Control: without the edge, every handler microflow is reported dead —
	// the bug as filed (mendixlabs/mxcli#1127).
	if dead := seed(t, false); len(dead) != 4 {
		t.Fatalf("control: %d dead microflows, want 4 — if the control does not "+
			"reproduce the defect, the assertion below detects nothing", len(dead))
	}

	if dead := seed(t, true); len(dead) != 0 {
		t.Errorf("with the event edge, %v are still reported dead", dead)
	}
}

// Reusing an existing kind would make `show references` say the wrong thing
// about how the entity uses the microflow.
func TestRefKindEventIsItsOwnKind(t *testing.T) {
	for _, other := range []string{
		RefKindCall, RefKindCalculate, RefKindSchedule, RefKindChange,
		RefKindDelete, RefKindAction, RefKindSettings,
	} {
		if RefKindEvent == other {
			t.Errorf("RefKindEvent collides with %q", other)
		}
	}
	if RefKindEvent != "event" {
		t.Errorf("RefKindEvent = %q; the value appears in user-facing output and in "+
			"the bundled Starlark rules", RefKindEvent)
	}
}

// The three consumers of the reference graph keep independent lists, and none
// shares the other's. A kind added to the builder alone is an edge nothing reads.
func TestEventKindReachesTheAssetGraph(t *testing.T) {
	if !contains(graphRefKinds, RefKindEvent) {
		t.Error("graphRefKinds is missing 'event' — the handler microflow stays " +
			"outside the analysis graph even though the edge exists")
	}
}
