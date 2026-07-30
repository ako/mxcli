// SPDX-License-Identifier: Apache-2.0

// A data container bound to a microflow or nanoflow reported the *flow* as its
// context entity — `-- Context: $currentObject (Module.GetOrders)` instead of
// `(Module.Order)` — because the datasource reference was used verbatim. That is
// correct for a database source, where the reference is the entity, and wrong for
// a flow source, where it is the flow's own name.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func TestDataTypeEntity(t *testing.T) {
	tests := []struct {
		name string
		in   microflows.DataType
		want string
	}{
		{"object pointer", &microflows.ObjectType{EntityQualifiedName: "M.Order"}, "M.Order"},
		{"object value", microflows.ObjectType{EntityQualifiedName: "M.Order"}, "M.Order"},
		{"list pointer", &microflows.ListType{EntityQualifiedName: "M.Order"}, "M.Order"},
		{"list value", microflows.ListType{EntityQualifiedName: "M.Order"}, "M.Order"},
		{"boolean", &microflows.BooleanType{}, ""},
		{"nil", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dataTypeEntity(tc.in); got != tc.want {
				t.Errorf("dataTypeEntity = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDataSourceEntityContext_NonFlowSources: a database or association source
// already names its entity, so it must pass straight through.
func TestDataSourceEntityContext_NonFlowSources(t *testing.T) {
	for _, dsType := range []string{"database", "association", "parameter"} {
		t.Run(dsType, func(t *testing.T) {
			ds := &rawDataSource{Type: dsType, Reference: "M.Order"}
			if got := dataSourceEntityContext(nil, ds); got != "M.Order" {
				t.Errorf("dataSourceEntityContext = %q, want M.Order", got)
			}
		})
	}
	if got := dataSourceEntityContext(nil, nil); got != "" {
		t.Errorf("nil datasource = %q, want empty", got)
	}
	if got := dataSourceEntityContext(nil, &rawDataSource{Type: "database"}); got != "" {
		t.Errorf("empty reference = %q, want empty", got)
	}
}

// TestDataSourceEntityContext_ResolvesFlowReturnEntity is the fix: the flow's
// return entity, not the flow's name.
func TestDataSourceEntityContext_ResolvesFlowReturnEntity(t *testing.T) {
	mod := &model.Module{BaseElement: model.BaseElement{ID: nextID("mod")}, Name: "M"}
	h := mkHierarchy(mod)

	mf := &microflows.Microflow{
		ContainerID: mod.ID,
		Name:        "GetOrders",
		ReturnType:  &microflows.ListType{EntityQualifiedName: "M.Order"},
	}
	nf := &microflows.Nanoflow{
		ContainerID: mod.ID,
		Name:        "GetOrderClient",
		ReturnType:  &microflows.ObjectType{EntityQualifiedName: "M.Order"},
	}
	// A flow that returns something a data container cannot bind against.
	scalar := &microflows.Microflow{
		ContainerID: mod.ID,
		Name:        "CountOrders",
		ReturnType:  &microflows.IntegerType{},
	}

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{mf, scalar}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{nf}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

	tests := []struct {
		name string
		ds   *rawDataSource
		want string
	}{
		{"microflow list", &rawDataSource{Type: "microflow", Reference: "M.GetOrders"}, "M.Order"},
		{"nanoflow object", &rawDataSource{Type: "nanoflow", Reference: "M.GetOrderClient"}, "M.Order"},
		// Unresolvable cases keep the reference — no worse than before the fix.
		{"scalar return", &rawDataSource{Type: "microflow", Reference: "M.CountOrders"}, "M.CountOrders"},
		{"unknown flow", &rawDataSource{Type: "microflow", Reference: "M.Missing"}, "M.Missing"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := dataSourceEntityContext(ctx, tc.ds); got != tc.want {
				t.Errorf("dataSourceEntityContext = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDataSourceEntityContext_NoProject: with no backend the reference is kept
// rather than the context being blanked.
func TestDataSourceEntityContext_NoProject(t *testing.T) {
	ds := &rawDataSource{Type: "microflow", Reference: "M.GetOrders"}
	if got := dataSourceEntityContext(nil, ds); got != "M.GetOrders" {
		t.Errorf("dataSourceEntityContext = %q, want the reference kept", got)
	}
}
