// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// ProxyHost / ProxyPort / ProxyUsername / ProxyPassword are BY_NAME references to
// a constant: Studio Pro stores the bare qualified name `Odata.Bug1073_ProxyHost`
// (measured, ako/TestApp@37e0cc0). `@Module.Const` is how MDL spells a constant
// everywhere else, but it was written into the reference verbatim — `"@Odata.X"`
// names nothing, so the proxy silently resolves to no constant. Same class as
// #573's "MICROFLOW " prefix: the value handed to the backend must be the bare
// name, whatever spelling the author used.

var proxyConstantSpellings = []struct {
	name, mdl string
}{
	{"at", "@MyModule.ProxyHost"},
	{"quoted at", "'@MyModule.ProxyHost'"},
	// The bare name already stored correctly: the control.
	{"bare", "MyModule.ProxyHost"},
}

func TestCreateODataClient_ProxyConstantStoredAsBareName(t *testing.T) {
	for _, sp := range proxyConstantSpellings {
		t.Run(sp.name, func(t *testing.T) {
			mod := mkModule("MyModule")
			var captured *model.ConsumedODataService
			mb := &mock.MockBackend{
				IsConnectedFunc:   func() bool { return true },
				ListModulesFunc:   func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListConstantsFunc: func() ([]*model.Constant, error) { return nil, nil },
				ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
					return nil, nil
				},
				CreateConsumedODataServiceFunc: func(svc *model.ConsumedODataService) error {
					captured = svc
					return nil
				},
			}
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

			prog := parseMDL(t, `create odata client MyModule.Api (
  ODataVersion: OData4,
  MetadataUrl: 'https://example.com/odata/$metadata',
  ProxyType: Override,
  ProxyHost: `+sp.mdl+`,
  ProxyPort: `+sp.mdl+`,
  ProxyUsername: `+sp.mdl+`,
  ProxyPassword: `+sp.mdl+`
);`)
			stmt := prog.Statements[0].(*ast.CreateODataClientStmt)
			_ = createODataClient(ctx, stmt) // the $metadata fetch may warn; the write is what is under test

			if captured == nil {
				t.Fatal("CreateConsumedODataService was not called")
			}
			for field, got := range map[string]string{
				"ProxyHost":     captured.ProxyHost,
				"ProxyPort":     captured.ProxyPort,
				"ProxyUsername": captured.ProxyUsername,
				"ProxyPassword": captured.ProxyPassword,
			} {
				if got != "MyModule.ProxyHost" {
					t.Errorf("%s written as %s: stored %q, want the bare constant name %q",
						field, sp.mdl, got, "MyModule.ProxyHost")
				}
			}
		})
	}
}

func TestAlterODataClient_ProxyConstantStoredAsBareName(t *testing.T) {
	for _, sp := range proxyConstantSpellings {
		t.Run(sp.name, func(t *testing.T) {
			mod := mkModule("MyModule")
			svc := &model.ConsumedODataService{
				BaseElement: model.BaseElement{ID: nextID("cos")},
				ContainerID: mod.ID,
				Name:        "Api",
			}
			h := mkHierarchy(mod)
			withContainer(h, svc.ContainerID, mod.ID)
			var updated *model.ConsumedODataService
			mb := &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
				ListConsumedODataServicesFunc: func() ([]*model.ConsumedODataService, error) {
					return []*model.ConsumedODataService{svc}, nil
				},
				UpdateConsumedODataServiceFunc: func(s *model.ConsumedODataService) error {
					updated = s
					return nil
				},
			}
			ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))

			prog := parseMDL(t, `alter odata client MyModule.Api set ProxyHost = `+sp.mdl+`;`)
			assertNoError(t, alterODataClient(ctx, prog.Statements[0].(*ast.AlterODataClientStmt)))

			if updated == nil {
				t.Fatal("UpdateConsumedODataService was not called")
			}
			if updated.ProxyHost != "MyModule.ProxyHost" {
				t.Errorf("ProxyHost set to %s: stored %q, want the bare constant name %q",
					sp.mdl, updated.ProxyHost, "MyModule.ProxyHost")
			}
		})
	}
}
