// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// bulkFixture: one user module with a persistent, a non-persistent and a view
// entity, plus a Marketplace module and System.
func bulkFixture() (mods []*model.Module, dms map[model.ID]*domainmodel.DomainModel) {
	sales := mkModule("Sales")
	admin := mkModule("Administration")
	admin.FromAppStore = true
	system := mkModule("System")

	order := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Order", Persistable: true}
	draft := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Draft", Persistable: false}
	report := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Report",
		Persistable: true, Source: "OqlViewEntitySource", OqlQuery: "select 1",
	}
	vehicle := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Vehicle", Persistable: true}
	truck := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Truck",
		Persistable: true, GeneralizationRef: "Sales.Vehicle",
	}
	account := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "Account", Persistable: true}
	user := &domainmodel.Entity{BaseElement: model.BaseElement{ID: nextID("ent")}, Name: "User", Persistable: true}

	return []*model.Module{sales, admin, system}, map[model.ID]*domainmodel.DomainModel{
		sales.ID:  mkDomainModel(sales.ID, order, draft, report, vehicle, truck),
		admin.ID:  mkDomainModel(admin.ID, account),
		system.ID: mkDomainModel(system.ID, user),
	}
}

func runBulk(t *testing.T, stmt *ast.AlterEntitiesStmt) (touched []string, out string) {
	t.Helper()
	mods, dms := bulkFixture()
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return mods, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dms[id], nil },
		UpdateEntityFunc: func(dmID model.ID, e *domainmodel.Entity) error {
			touched = append(touched, e.Name)
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	if err := execAlterEntities(ctx, stmt); err != nil {
		t.Fatalf("execAlterEntities: %v", err)
	}
	return touched, buf.String()
}

func addAudit() []*ast.AlterEntityStmt {
	return []*ast.AlterEntityStmt{{
		Operation:   ast.AlterEntityAddAttribute,
		Attribute:   &ast.Attribute{Name: "Note", Type: ast.DataType{Kind: ast.TypeString, Length: 10}},
		IfNotExists: true,
	}}
}

// WHERE PERSISTENT must exclude the non-persistent entity AND the view entity:
// a view's rows come from an OQL query, so it is neither, and treating it as
// persistent would aim a write at a document that cannot take one.
func TestAlterEntities_PersistentFilterExcludesViewAndNonPersistent(t *testing.T) {
	touched, _ := runBulk(t, &ast.AlterEntitiesStmt{
		Module: "Sales", Filter: ast.EntityFilterPersistent, Actions: addAudit(),
	})
	// Vehicle is in; Truck is not, because it inherits from Vehicle.
	want := map[string]bool{"Order": true, "Vehicle": true}
	if len(touched) != len(want) {
		t.Fatalf("touched %v, want %v — Draft is non-persistent, Report is a view, Truck inherits", touched, want)
	}
	for _, n := range touched {
		if !want[n] {
			t.Errorf("touched %q unexpectedly", n)
		}
	}
}

func TestAlterEntities_NonPersistentFilter(t *testing.T) {
	touched, _ := runBulk(t, &ast.AlterEntitiesStmt{
		Module: "Sales", Filter: ast.EntityFilterNonPersistent, Actions: addAudit(),
	})
	if len(touched) != 1 || touched[0] != "Draft" {
		t.Fatalf("touched %v, want only [Draft]", touched)
	}
}

// No WHERE: every entity in the named module, the view included — the filter is
// opt-in, and a statement that names no filter must not quietly apply one.
func TestAlterEntities_NoFilterTakesEveryEntityInTheModule(t *testing.T) {
	touched, _ := runBulk(t, &ast.AlterEntitiesStmt{Module: "Sales", Actions: addAudit()})
	// Order, Draft, Vehicle. Report is a view (never a target) and Truck
	// inherits from Vehicle.
	if len(touched) != 3 {
		t.Fatalf("touched %v, want 3 (Order, Draft, Vehicle)", touched)
	}
	for _, n := range touched {
		if n == "Report" {
			t.Errorf("a view entity must never be a target — CE6770")
		}
		if n == "Truck" {
			t.Errorf("a specialization of a target must be skipped — CE0069")
		}
	}
}

// An unscoped sweep must not edit System or a Marketplace module: an upgrade
// replaces the module and takes the attribute with it, so the write is undone
// later rather than refused now.
func TestAlterEntities_UnscopedSweepSkipsSystemAndMarketplace(t *testing.T) {
	touched, out := runBulk(t, &ast.AlterEntitiesStmt{Filter: ast.EntityFilterPersistent, Actions: addAudit()})
	for _, name := range touched {
		if name == "Account" || name == "User" {
			t.Errorf("touched %q — a System/Marketplace entity must be skipped on an unscoped sweep", name)
		}
	}
	if len(touched) != 2 {
		t.Fatalf("touched %v, want 2 (Order, Vehicle)", touched)
	}
	if !strings.Contains(out, "Skipped 2 System/Marketplace module(s)") {
		t.Errorf("the skip must be reported, not silent; output was:\n%s", out)
	}
}

// Naming the module explicitly is taken as meaning it — otherwise a deliberate
// edit to a Marketplace module would be impossible rather than merely guarded.
func TestAlterEntities_NamedMarketplaceModuleIsAllowed(t *testing.T) {
	touched, _ := runBulk(t, &ast.AlterEntitiesStmt{Module: "Administration", Actions: addAudit()})
	if len(touched) != 1 || touched[0] != "Account" {
		t.Fatalf("touched %v, want [Account] — an explicitly named module is not skipped", touched)
	}
}

func TestAlterEntities_UnknownModuleIsAnError(t *testing.T) {
	mods, dms := bulkFixture()
	mb := &mock.MockBackend{
		IsConnectedFunc:    func() bool { return true },
		ListModulesFunc:    func() ([]*model.Module, error) { return mods, nil },
		GetDomainModelFunc: func(id model.ID) (*domainmodel.DomainModel, error) { return dms[id], nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	err := execAlterEntities(ctx, &ast.AlterEntitiesStmt{Module: "NoSuchModule", Actions: addAudit()})
	if err == nil {
		t.Fatal("a misspelled module must be an error, not a silent no-op that reports 0 entities")
	}
	if !strings.Contains(err.Error(), "NoSuchModule") {
		t.Errorf("error should name the module, got: %v", err)
	}
}

// A view entity is out of reach even when no filter is given: its columns come
// from its OQL select list, and an added attribute is CE6770.
func TestAlterEntities_ViewEntityIsNeverATarget(t *testing.T) {
	for _, f := range []ast.EntityPersistenceFilter{
		ast.EntityFilterAll, ast.EntityFilterPersistent, ast.EntityFilterNonPersistent,
	} {
		touched, _ := runBulk(t, &ast.AlterEntitiesStmt{Module: "Sales", Filter: f, Actions: addAudit()})
		for _, n := range touched {
			if n == "Report" {
				t.Errorf("filter %v: touched the view entity Report", f)
			}
		}
	}
}

// Adding the same attribute to a generalization AND its specialization is
// CE0069 "Duplicate member name". The parent is kept, the child dropped -- the
// child inherits the member, so the author's intent is still satisfied.
func TestAlterEntities_SpecializationOfATargetIsSkipped(t *testing.T) {
	touched, _ := runBulk(t, &ast.AlterEntitiesStmt{
		Module: "Sales", Filter: ast.EntityFilterPersistent, Actions: addAudit(),
	})
	var sawVehicle, sawTruck bool
	for _, n := range touched {
		sawVehicle = sawVehicle || n == "Vehicle"
		sawTruck = sawTruck || n == "Truck"
	}
	if !sawVehicle {
		t.Error("the generalization must be touched")
	}
	if sawTruck {
		t.Error("the specialization must be skipped — it inherits the member (CE0069)")
	}
}
