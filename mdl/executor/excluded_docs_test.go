// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// TestPickLive covers the selector every by-name lookup now goes through.
// The order cases are the point: before #914 the lookups took the first match,
// so which document they resolved to depended on enumeration order rather than
// on which one the app contains.
func TestPickLive(t *testing.T) {
	type doc struct {
		name string
		excl bool
	}
	match := func(want string) func(doc) bool {
		return func(d doc) bool { return d.name == want }
	}
	excluded := func(d doc) bool { return d.excl }

	t.Run("excluded first", func(t *testing.T) {
		got, ok := pickLive([]doc{{"A", true}, {"A", false}}, match("A"), excluded)
		if !ok || got.excl {
			t.Fatalf("want the live document, got %+v (ok=%v)", got, ok)
		}
	})

	t.Run("live first", func(t *testing.T) {
		got, ok := pickLive([]doc{{"A", false}, {"A", true}}, match("A"), excluded)
		if !ok || got.excl {
			t.Fatalf("want the live document, got %+v (ok=%v)", got, ok)
		}
	})

	t.Run("all excluded falls back to the first match", func(t *testing.T) {
		got, ok := pickLive([]doc{{"B", true}, {"A", true}, {"A", true}}, match("A"), excluded)
		if !ok || !got.excl {
			t.Fatalf("an all-excluded set must still resolve, got %+v (ok=%v)", got, ok)
		}
	})

	t.Run("no match", func(t *testing.T) {
		if _, ok := pickLive([]doc{{"B", false}}, match("A"), excluded); ok {
			t.Fatal("no match must report false")
		}
	})
}

// microflowWriteProbe wires a backend whose ListMicroflows returns stored and
// captures whatever CreateMicroflow is handed.
func microflowWriteProbe(t *testing.T, stored []*microflows.Microflow, moduleID model.ID) (*ExecContext, **microflows.Microflow) {
	t.Helper()
	var written *microflows.Microflow
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "MyModule",
			}}, nil
		},
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name != "MyModule" {
				return nil, nil
			}
			return &model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: "MyModule"}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return stored, nil },
		CreateMicroflowFunc: func(mf *microflows.Microflow) error {
			written = mf
			return nil
		},
		// An existing document is rewritten through UpdateMicroflow, which is
		// the path both #914 halves run down.
		UpdateMicroflowFunc: func(mf *microflows.Microflow) error {
			written = mf
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx, &written
}

// TestCreateOrModifyMicroflow_PreservesStoredExclusion is the #914 write half:
// a statement that never mentions exclusion must not clear one. Before the fix
// the rebuild wrote Excluded=false from the AST default, which turned an
// excluded document active — and with a same-named live twin present that is
// CE0122 "Duplicate document name", i.e. a valid project made unbuildable by a
// statement that only meant to edit a body.
func TestCreateOrModifyMicroflow_PreservesStoredExclusion(t *testing.T) {
	const moduleID = model.ID("module-1")
	stored := []*microflows.Microflow{{
		BaseElement: model.BaseElement{ID: "mf-excluded"},
		ContainerID: moduleID,
		Name:        "Calc",
		Excluded:    true,
	}}
	ctx, written := microflowWriteProbe(t, stored, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "Calc"},
		CreateOrModify: true,
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if !(*written).Excluded {
		t.Error("a rewrite cleared the stored Excluded flag; the document would become active and collide with its twin (CE0122)")
	}
}

// TestCreateOrModifyMicroflow_TargetsLiveTwin is the #914 read half: with two
// documents of the same name, the write must land on the one the app contains.
// The excluded twin is deliberately first so that a first-match lookup picks
// the wrong document — which is exactly what happened before the fix, leaving
// the live microflow silently unchanged.
func TestCreateOrModifyMicroflow_TargetsLiveTwin(t *testing.T) {
	const moduleID = model.ID("module-1")
	stored := []*microflows.Microflow{
		{
			BaseElement: model.BaseElement{ID: "mf-excluded"},
			ContainerID: moduleID,
			Name:        "Calc",
			Excluded:    true,
		},
		{
			BaseElement: model.BaseElement{ID: "mf-live"},
			ContainerID: moduleID,
			Name:        "Calc",
		},
	}
	ctx, written := microflowWriteProbe(t, stored, moduleID)

	stmt := &ast.CreateMicroflowStmt{
		Name:           ast.QualifiedName{Module: "MyModule", Name: "Calc"},
		CreateOrModify: true,
	}
	if err := execCreateMicroflow(ctx, stmt); err != nil {
		t.Fatalf("CREATE OR MODIFY MICROFLOW failed: %v", err)
	}
	if *written == nil {
		t.Fatal("no microflow was written")
	}
	if got := (*written).ID; got != "mf-live" {
		t.Errorf("write targeted %q, want the live document %q", got, "mf-live")
	}
	if (*written).Excluded {
		t.Error("the live document must not inherit the excluded twin's flag")
	}
}
