// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// A published REST operation is an entry point of the same shape as a scheduled
// event: a document that NAMES a microflow and runs it, with no call activity
// anywhere in the model. The catalog recorded the binding in
// published_rest_operations_data.Microflow and emitted no `refs` edge, so every
// microflow whose only caller is an operation had zero inbound references —
// listed by GRAPH_DEAD_ASSETS, "(no callers found)" from SHOW CALLERS, and
// QUAL004 "not called from anywhere. Remove if unused." on the back end of the
// public API. Measured on the reporter's model: 92 of 93 operations name a
// microflow and all 92 were reported dead (mendixlabs/mxcli#1126).

const restRefsModuleID = model.ID("mod-sales")

// restFixture runs the two real halves of the path — buildPublishedRestServices,
// which collects the edges, and extractPublishedRestRefs, which emits them — over
// one service, and returns the catalog they wrote into.
func restFixture(t *testing.T, svc *model.PublishedRestService) *Catalog {
	t.Helper()

	cat, err := New()
	if err != nil {
		t.Fatalf("new catalog: %v", err)
	}
	t.Cleanup(func() { cat.Close() })

	tx, err := cat.db.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	b := &Builder{
		catalog: cat,
		reader: &mock.MockBackend{
			ListPublishedRestServicesFunc: func() ([]*model.PublishedRestService, error) {
				return []*model.PublishedRestService{svc}, nil
			},
		},
		snapshot: &Snapshot{ID: "snap-1"},
		hierarchy: &hierarchy{
			moduleIDs:       map[model.ID]bool{restRefsModuleID: true},
			moduleNames:     map[model.ID]string{restRefsModuleID: "Sales"},
			containerParent: map[model.ID]model.ID{},
			folderNames:     map[model.ID]string{},
		},
		tx: tx,
	}

	if err := b.buildPublishedRestServices(); err != nil {
		t.Fatalf("buildPublishedRestServices: %v", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO refs (SourceType, SourceId, SourceName, TargetType, TargetId, TargetName, RefKind, ModuleName, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		t.Fatalf("prepare refs: %v", err)
	}
	projectID, snapshotID := b.snapshotMeta()
	b.extractPublishedRestRefs(stmt, projectID, snapshotID)
	stmt.Close()

	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	return cat
}

// oneService returns a service with a single resource holding the given
// operations, containered directly in the module.
func oneService(ops ...*model.PublishedRestOperation) *model.PublishedRestService {
	svc := &model.PublishedRestService{
		ContainerID: restRefsModuleID,
		Name:        "OrdersApi",
		Path:        "orders/v1",
		Resources: []*model.PublishedRestResource{
			{Name: "Order", Operations: ops},
		},
	}
	svc.ID = model.ID("svc-1")
	return svc
}

func queryRows(t *testing.T, cat *Catalog, q string) [][]any {
	t.Helper()
	res, err := cat.Query(q)
	if err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return res.Rows
}

// TestPublishedRestOperationEmitsPublishEdge pins the edge itself: its endpoints,
// its kind, and that it carries the operation's own id so it joins back to
// published_rest_operations_data.
func TestPublishedRestOperationEmitsPublishEdge(t *testing.T) {
	cat := restFixture(t, oneService(&model.PublishedRestOperation{
		HTTPMethod: "GET",
		Path:       "{id}",
		Microflow:  "Sales.GetOrder",
	}))

	rows := queryRows(t, cat, `SELECT SourceType, SourceName, TargetType, TargetName, RefKind, ModuleName FROM refs`)
	if len(rows) != 1 {
		t.Fatalf("want exactly 1 refs row, got %d: %v", len(rows), rows)
	}
	got := rows[0]
	want := []any{"PUBLISHED_REST_OPERATION", "Sales.OrdersApi Order GET {id}", "MICROFLOW", "Sales.GetOrder", "publish", "Sales"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d = %v, want %v", i, got[i], want[i])
		}
	}

	// The edge must be joinable to the operation it came from, so a consumer can
	// get from "who calls this microflow" to the operation's path and summary.
	joined := queryRows(t, cat, `
		SELECT COUNT(*) FROM refs r
		JOIN published_rest_operations_data o ON o.Id = r.SourceId
		WHERE r.RefKind = 'publish'`)
	if joined[0][0].(int64) != 1 {
		t.Errorf("the publish edge does not join to published_rest_operations_data on SourceId; "+
			"got %v matches", joined[0][0])
	}
}

// An operation with no microflow is a real shape — Mendix allows a
// published operation bound to nothing while it is being built — and must not
// produce an edge to the empty name, which would collide with every other
// unnamed target in the table.
func TestPublishedRestOperationWithoutMicroflowEmitsNothing(t *testing.T) {
	cat := restFixture(t, oneService(
		&model.PublishedRestOperation{HTTPMethod: "GET", Path: "{id}", Microflow: "Sales.GetOrder"},
		&model.PublishedRestOperation{HTTPMethod: "POST", Path: ""},
	))

	rows := queryRows(t, cat, `SELECT COUNT(*) FROM published_rest_operations_data`)
	if rows[0][0].(int64) != 2 {
		t.Fatalf("control: want both operations catalogued, got %v", rows[0][0])
	}
	rows = queryRows(t, cat, `SELECT COUNT(*) FROM refs`)
	if rows[0][0].(int64) != 1 {
		t.Errorf("want 1 edge for the 2 operations (only one names a microflow), got %v", rows[0][0])
	}
}

// TestPublishedRestMicroflowIsNotDead is the reported symptom. GRAPH_DEAD_ASSETS
// is kind-agnostic — it asks only whether ANY refs row targets the name — so the
// edge alone clears it, whatever kind it carries.
//
// The unreferenced microflow in the same fixture is the control: without it a
// fix that emptied the view entirely would pass.
func TestPublishedRestMicroflowIsNotDead(t *testing.T) {
	cat := restFixture(t, oneService(&model.PublishedRestOperation{
		HTTPMethod: "GET",
		Path:       "{id}",
		Microflow:  "Sales.GetOrder",
	}))

	for _, mf := range []string{"GetOrder", "Orphan"} {
		if _, err := cat.db.Exec(
			`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType)
			 VALUES (?, ?, ?, 'Sales', 'MICROFLOW')`,
			"mf-"+mf, mf, "Sales."+mf); err != nil {
			t.Fatalf("insert microflow: %v", err)
		}
	}

	dead := map[string]bool{}
	for _, row := range queryRows(t, cat, `SELECT QualifiedName FROM graph_dead_assets`) {
		dead[row[0].(string)] = true
	}
	if !dead["Sales.Orphan"] {
		t.Fatal("control failed: a microflow nothing references is not reported dead, " +
			"so this test cannot detect the bug")
	}
	if dead["Sales.GetOrder"] {
		t.Error("a microflow invoked by a published REST operation is reported dead — " +
			"the dead list recommends deleting the back end of the public API (#1126)")
	}
}
