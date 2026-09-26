// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// flowCountsCatalog runs the REAL catalog builder over a backend holding two
// microflows, one nanoflow and one rule in module Sales. Seeding microflows_data
// by hand would only prove the reader agrees with whatever value the test typed;
// going through the builder is what detects the writer and the reader spelling
// MicroflowType differently.
func flowCountsCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	const mod = model.ID("mod-sales")
	be := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		// The builder dereferences project settings; the mock's nil default
		// would panic before any flow is catalogued.
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) { return &model.ProjectSettings{}, nil },
		ListModuleSettingsFunc: func() ([]*types.ModuleSettings, error) { return nil, nil },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{BaseElement: model.BaseElement{ID: mod}, Name: "Sales"}}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) {
			return []*microflows.Microflow{
				{BaseElement: model.BaseElement{ID: "mf-1"}, ContainerID: mod, Name: "ACT_One"},
				{BaseElement: model.BaseElement{ID: "mf-2"}, ContainerID: mod, Name: "ACT_Two"},
			}, nil
		},
		ListNanoflowsFunc: func() ([]*microflows.Nanoflow, error) {
			return []*microflows.Nanoflow{
				{BaseElement: model.BaseElement{ID: "nf-1"}, ContainerID: mod, Name: "NF_One"},
			}, nil
		},
		ListRulesFunc: func() ([]*microflows.Rule, error) {
			return []*microflows.Rule{
				{BaseElement: model.BaseElement{ID: "rule-1"}, ContainerID: mod, Name: "RULE_One"},
			}, nil
		},
	}
	cat, err := catalog.New()
	if err != nil {
		t.Fatalf("catalog.New: %v", err)
	}
	t.Cleanup(func() { cat.Close() })
	if err := catalog.NewBuilder(cat, be).Build(nil); err != nil {
		t.Fatalf("catalog build: %v", err)
	}
	return cat
}

// `show structure depth 1` never listed microflow or nanoflow counts: it
// filtered on MicroflowType = 'microflow' / 'nanoflow' while the catalog
// builder stores 'MICROFLOW' / 'NANOFLOW', so both queries matched nothing and
// the zero counts were silently omitted from the summary line.
func TestStructureDepth1CountsFlowsFromBuiltCatalog(t *testing.T) {
	cat := flowCountsCatalog(t)
	ctx, buf := newMockCtx(t)
	ctx.Catalog = cat

	mods := []structureModule{{Name: "Sales", ID: "mod-sales"}}
	if err := structureDepth1(ctx, mods); err != nil {
		t.Fatalf("structureDepth1: %v", err)
	}
	out := buf.String()
	// The rule is its own doctype: it must not be counted as a microflow.
	for _, want := range []string{"2 microflows", "1 nanoflow"} {
		if !strings.Contains(out, want) {
			t.Errorf("depth-1 summary lacks %q; got:\n%s", want, out)
		}
	}
}

func TestStructureDepth1JSONCountsFlowsFromBuiltCatalog(t *testing.T) {
	cat := flowCountsCatalog(t)
	ctx, buf := newMockCtx(t)
	ctx.Catalog = cat
	ctx.Format = FormatJSON

	mods := []structureModule{{Name: "Sales", ID: "mod-sales"}}
	if err := structureDepth1JSON(ctx, mods); err != nil {
		t.Fatalf("structureDepth1JSON: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`"Microflows": 2`, `"Nanoflows": 1`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON structure lacks %s; got:\n%s", want, out)
		}
	}
}
