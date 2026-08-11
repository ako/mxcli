// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

func mkQueue(containerID model.ID, name, parallelism string, clusterWide bool) *types.Queue {
	q := &types.Queue{
		ContainerID: containerID,
		Name:        name,
		Parallelism: parallelism,
		ClusterWide: clusterWide,
	}
	q.ID = nextID("queue")
	return q
}

func TestShowQueues_Mock(t *testing.T) {
	mod := mkModule("Ops")
	q1 := mkQueue(mod.ID, "OrderProcessing", "3", true)
	q2 := mkQueue(mod.ID, "Mail", "1", false)

	h := mkHierarchy(mod)
	withContainer(h, q1.ContainerID, mod.ID)
	withContainer(h, q2.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListQueuesFunc:  func() ([]*types.Queue, error) { return []*types.Queue{q1, q2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execShowQueues(ctx, &ast.ShowQueuesStmt{}))

	out := buf.String()
	assertContainsStr(t, out, "Ops.OrderProcessing")
	assertContainsStr(t, out, "Ops.Mail")
	assertContainsStr(t, out, "(2 queue(s))")
}

func TestShowQueues_Mock_FilterByModule(t *testing.T) {
	alpha := mkModule("Alpha")
	beta := mkModule("Beta")
	q1 := mkQueue(alpha.ID, "One", "1", false)
	q2 := mkQueue(beta.ID, "Two", "2", false)

	h := mkHierarchy(alpha, beta)
	withContainer(h, q1.ContainerID, alpha.ID)
	withContainer(h, q2.ContainerID, beta.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListQueuesFunc:  func() ([]*types.Queue, error) { return []*types.Queue{q1, q2}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execShowQueues(ctx, &ast.ShowQueuesStmt{Module: "Beta"}))

	out := buf.String()
	assertNotContainsStr(t, out, "Alpha.One")
	assertContainsStr(t, out, "Beta.Two")
}

// TestCreateQueue_Mock_PassesParallelismThrough checks that the expression
// reaches the backend as written. Parallelism is an expression, so it must not
// be parsed into a number anywhere on the way down.
func TestCreateQueue_Mock_PassesParallelismThrough(t *testing.T) {
	mod := mkModule("Ops")
	h := mkHierarchy(mod)

	var created *types.Queue
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListQueuesFunc:      func() ([]*types.Queue, error) { return nil, nil },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		CreateQueueFunc: func(q *types.Queue) error {
			created = q
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateQueueStmt{
		Name:        ast.QualifiedName{Module: "Ops", Name: "OrderProcessing"},
		Parallelism: "$Config/Workers",
		ClusterWide: true,
	}
	assertNoError(t, execCreateQueue(ctx, stmt))

	if created == nil {
		t.Fatal("CreateQueue was not called")
	}
	if created.Parallelism != "$Config/Workers" {
		t.Errorf("Parallelism = %q, want the expression verbatim", created.Parallelism)
	}
	if !created.ClusterWide {
		t.Error("ClusterWide did not reach the backend")
	}
	assertContainsStr(t, buf.String(), "Created queue: Ops.OrderProcessing")
}

func TestCreateQueue_Mock_DuplicateWithoutOrModify(t *testing.T) {
	mod := mkModule("Ops")
	existing := mkQueue(mod.ID, "OrderProcessing", "1", false)
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListQueuesFunc:      func() ([]*types.Queue, error) { return []*types.Queue{existing}, nil },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		CreateQueueFunc: func(q *types.Queue) error {
			t.Error("CreateQueue must not be called for a duplicate")
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateQueueStmt{Name: ast.QualifiedName{Module: "Ops", Name: "OrderProcessing"}}
	if err := execCreateQueue(ctx, stmt); err == nil {
		t.Fatal("expected an already-exists error")
	}
}

func TestCreateQueue_Mock_OrModifyUpdates(t *testing.T) {
	mod := mkModule("Ops")
	existing := mkQueue(mod.ID, "OrderProcessing", "1", false)
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	var updated *types.Queue
	mb := &mock.MockBackend{
		IsConnectedFunc:     func() bool { return true },
		ListQueuesFunc:      func() ([]*types.Queue, error) { return []*types.Queue{existing}, nil },
		ListModulesFunc:     func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		UpdateQueueFunc: func(q *types.Queue) error {
			updated = q
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.CreateQueueStmt{
		Name:           ast.QualifiedName{Module: "Ops", Name: "OrderProcessing"},
		Parallelism:    "8",
		CreateOrModify: true,
	}
	assertNoError(t, execCreateQueue(ctx, stmt))

	if updated == nil {
		t.Fatal("UpdateQueue was not called")
	}
	// The stored ID must be reused, or the update becomes a second queue.
	if updated.ID != existing.ID {
		t.Errorf("ID = %q, want the existing %q", updated.ID, existing.ID)
	}
	if updated.Parallelism != "8" {
		t.Errorf("Parallelism = %q, want 8", updated.Parallelism)
	}
	assertContainsStr(t, buf.String(), "Modified queue: Ops.OrderProcessing")
}

func TestDropQueue_Mock(t *testing.T) {
	mod := mkModule("Ops")
	existing := mkQueue(mod.ID, "OrderProcessing", "1", false)
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	var deleted string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListQueuesFunc:  func() ([]*types.Queue, error) { return []*types.Queue{existing}, nil },
		DeleteQueueFunc: func(id string) error {
			deleted = id
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.DropQueueStmt{Name: ast.QualifiedName{Module: "Ops", Name: "OrderProcessing"}}
	assertNoError(t, execDropQueue(ctx, stmt))

	if deleted != string(existing.ID) {
		t.Errorf("deleted %q, want %q", deleted, existing.ID)
	}
	assertContainsStr(t, buf.String(), "Dropped queue: Ops.OrderProcessing")
}

func TestDropQueue_Mock_NotFound(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListQueuesFunc:  func() ([]*types.Queue, error) { return nil, nil },
		DeleteQueueFunc: func(id string) error {
			t.Error("DeleteQueue must not be called when the queue does not exist")
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy()))
	stmt := &ast.DropQueueStmt{Name: ast.QualifiedName{Module: "Ops", Name: "Missing"}}
	if err := execDropQueue(ctx, stmt); err == nil {
		t.Fatal("expected a not-found error")
	}
}

// TestDescribeQueue_Mock_RoundTrips checks that DESCRIBE emits MDL that can be
// fed straight back in, including quoting a non-numeric parallelism expression.
func TestDescribeQueue_Mock_RoundTrips(t *testing.T) {
	mod := mkModule("Ops")
	q := mkQueue(mod.ID, "OrderProcessing", "$Config/Workers", true)
	h := mkHierarchy(mod)
	withContainer(h, q.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListQueuesFunc:  func() ([]*types.Queue, error) { return []*types.Queue{q}, nil },
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	stmt := &ast.DescribeQueueStmt{Name: ast.QualifiedName{Module: "Ops", Name: "OrderProcessing"}}
	assertNoError(t, execDescribeQueue(ctx, stmt))

	out := buf.String()
	assertContainsStr(t, out, "create or modify queue Ops.OrderProcessing (")
	assertContainsStr(t, out, "Parallelism: '$Config/Workers',")
	assertContainsStr(t, out, "ClusterWide: true,")
}

func TestFormatParallelism(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "1"},
		{"3", "3"},
		{"$Config/Workers", "'$Config/Workers'"},
		{"it's", "'it''s'"}, // Mendix escapes a quote by doubling it, never with a backslash
	}
	for _, tt := range tests {
		if got := formatParallelism(tt.in); got != tt.want {
			t.Errorf("formatParallelism(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
