// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// ServiceUrl names a constant, the same way ProxyHost does: Studio Pro picks the
// service URL as a constant, and stores it in HttpConfiguration.CustomLocation
// as `@Module.Name` (ako/TestApp Odata.Bug1073: "@Odata.Bug1073_Location").
// So MDL takes the same spellings as the proxy references — the bare name, `@`
// and quoted `@` — and describe prints the bare name, like `ProxyHost:
// Odata.Bug1073_ProxyHost`. Before, only the two `@` spellings were accepted and
// the bare name was refused as "not a constant reference".

var serviceURLSpellings = []struct{ name, mdl string }{
	{"bare", "MyModule.Location"},
	// The two spellings that already worked: the controls.
	{"at", "@MyModule.Location"},
	{"quoted at", "'@MyModule.Location'"},
}

func TestCreateODataClient_ServiceUrlNamesAConstant(t *testing.T) {
	for _, sp := range serviceURLSpellings {
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
  ServiceUrl: `+sp.mdl+`
);`)
			_ = createODataClient(ctx, prog.Statements[0].(*ast.CreateODataClientStmt))
			if captured == nil || captured.HttpConfiguration == nil {
				t.Fatalf("ServiceUrl: %s was not written", sp.mdl)
			}
			if got := captured.HttpConfiguration.CustomLocation; got != "@MyModule.Location" {
				t.Errorf("ServiceUrl: %s stored CustomLocation %q, want %q", sp.mdl, got, "@MyModule.Location")
			}
			if !captured.HttpConfiguration.OverrideLocation {
				t.Errorf("ServiceUrl: %s did not set OverrideLocation", sp.mdl)
			}
		})
	}
}

func TestAlterODataClient_ServiceUrlNamesAConstant(t *testing.T) {
	for _, sp := range serviceURLSpellings {
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
			prog := parseMDL(t, `alter odata client MyModule.Api set ServiceUrl = `+sp.mdl+`;`)
			assertNoError(t, alterODataClient(ctx, prog.Statements[0].(*ast.AlterODataClientStmt)))
			if updated == nil || updated.HttpConfiguration == nil {
				t.Fatal("UpdateConsumedODataService was not called with an HTTP configuration")
			}
			if got := updated.HttpConfiguration.CustomLocation; got != "@MyModule.Location" {
				t.Errorf("set ServiceUrl = %s stored CustomLocation %q, want %q", sp.mdl, got, "@MyModule.Location")
			}
		})
	}
}

// A literal URL is still refused: Studio Pro requires a constant (CE6825).
func TestServiceURLConstant_RefusesAString(t *testing.T) {
	for _, v := range []string{"https://api.example.com/odata", "", "not a name"} {
		if _, err := serviceURLConstant(v); err == nil {
			t.Errorf("serviceURLConstant(%q) accepted a value that is not a constant name", v)
		}
	}
	for _, v := range []string{"M.Loc", "@M.Loc", "M.Sub.Loc"} {
		if got, err := serviceURLConstant(v); err != nil || got != "@"+strings.TrimPrefix(v, "@") {
			t.Errorf("serviceURLConstant(%q) = %q, %v", v, got, err)
		}
	}
}

// describe prints the constant's bare name, as it does for ProxyHost, and that
// output re-stores the same CustomLocation.
func TestDescribeODataClient_ServiceUrlPrintsTheConstantName(t *testing.T) {
	stored := &model.ConsumedODataService{
		Name:         "Bug1073",
		ODataVersion: "OData4",
		HttpConfiguration: &model.HttpConfiguration{
			OverrideLocation: true,
			CustomLocation:   "@Odata.Bug1073_Location",
		},
	}
	var out bytes.Buffer
	if err := outputConsumedODataServiceMDL(&ExecContext{Output: &out}, stored, "Odata", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ServiceUrl: Odata.Bug1073_Location,") &&
		!strings.Contains(out.String(), "ServiceUrl: Odata.Bug1073_Location\n") {
		t.Errorf("describe should print the bare constant name, got:\n%s", out.String())
	}
	stmt := parseMDL(t, out.String()).Statements[0].(*ast.CreateODataClientStmt)
	if got, err := serviceURLConstant(stmt.ServiceUrl); err != nil || got != stored.HttpConfiguration.CustomLocation {
		t.Errorf("re-exec would store %q (%v), want %q", got, err, stored.HttpConfiguration.CustomLocation)
	}
}
