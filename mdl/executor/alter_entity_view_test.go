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

// viewAlterTestCtx builds an ExecContext over Shop.SaleStats (a view entity, as
// mxcli and Studio Pro store one) and Shop.Sale (a persistent entity), wired so
// both execAlterEntity and the check-time validator can resolve them.
func viewAlterTestCtx(t *testing.T) (*ExecContext, *bool) {
	t.Helper()
	mod := mkModule("Shop")
	dmID := nextID("dm")
	stats := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: dmID,
		Name:        "SaleStats",
		Persistable: true,
		Source:      "DomainModels$OqlViewEntitySource",
		OqlQuery:    "select s.CustomerName as CustomerName, sum(s.Amount) as Total from Shop.Sale as s group by s.CustomerName",
		Attributes: []*domainmodel.Attribute{
			{Name: "CustomerName"},
			{Name: "Total"},
		},
	}
	sale := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: dmID,
		Name:        "Sale",
		Persistable: true,
		Attributes:  []*domainmodel.Attribute{{Name: "Amount"}, {Name: "CustomerName"}},
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: dmID},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{stats, sale},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)

	updated := false
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc:     func(dmID model.ID, e *domainmodel.Entity) error { updated = true; return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, &updated
}

func viewAlterStmts(entity string) map[string]*ast.AlterEntityStmt {
	qn := ast.QualifiedName{Module: "Shop", Name: entity}
	return map[string]*ast.AlterEntityStmt{
		"add": {
			Name: qn, Operation: ast.AlterEntityAddAttribute,
			Attribute: &ast.Attribute{Name: "Region", Type: ast.DataType{Kind: ast.TypeString, Length: 200}},
		},
		"drop": {Name: qn, Operation: ast.AlterEntityDropAttribute, AttributeName: "CustomerName"},
	}
}

// mendixlabs/mxcli#1173: ADD ATTRIBUTE on a view entity wrote a
// DomainModels$StoredValue attribute with no OQL column behind it, reported
// "Added attribute 'Region' to entity …" with exit 0, and Studio Pro then
// refused the model with CE6770 "View Entity is out of sync with the OQL
// Query." DROP ATTRIBUTE leaves a query column with no attribute, the same
// error measured on 11.12.1. Both must be refused before anything is written.
func TestAlterEntity_RefusesAttributeSetChangeOnViewEntity(t *testing.T) {
	for op, stmt := range viewAlterStmts("SaleStats") {
		t.Run(op, func(t *testing.T) {
			ctx, updated := viewAlterTestCtx(t)
			err := execAlterEntity(ctx, stmt)
			if err == nil {
				t.Fatalf("%s attribute on a view entity was accepted — mxbuild reports CE6770", op)
			}
			if *updated {
				t.Error("the entity was written despite the refusal")
			}
			msg := err.Error()
			for _, want := range []string{"view entity", "create or modify view entity", "CE6770"} {
				if !strings.Contains(msg, want) {
					t.Errorf("refusal does not mention %q:\n%s", want, msg)
				}
			}
		})
	}
}

// The same refusal at check time: `mxcli check -p --references` passed the
// reported script, so the error surfaced only after exec had written it.
func TestValidateAlterEntity_RefusesAttributeSetChangeOnViewEntity(t *testing.T) {
	for op, stmt := range viewAlterStmts("SaleStats") {
		t.Run(op, func(t *testing.T) {
			ctx, _ := viewAlterTestCtx(t)
			if err := validateWithContext(ctx, stmt, newScriptContext()); err == nil {
				t.Fatalf("check passed %s attribute on a stored view entity", op)
			}
		})
	}
	t.Run("view entity created by the script", func(t *testing.T) {
		ctx, _ := viewAlterTestCtx(t)
		sc := newScriptContext()
		sc.entities["Shop.Fresh"] = true
		sc.viewEntities["Shop.Fresh"] = true
		if err := validateWithContext(ctx, viewAlterStmts("Fresh")["add"], sc); err == nil {
			t.Fatal("check passed ADD ATTRIBUTE on a view entity the script creates")
		}
	})
}

// CONTROL: the same statements on a persistent entity still go through, and
// RENAME ATTRIBUTE on a view entity — measured clean on 11.12.1, because the
// OqlViewValue binds the column by its Reference, not by the attribute name —
// is not caught by the refusal.
func TestAlterEntity_ViewRefusalLeavesOtherAltersAlone(t *testing.T) {
	for op, stmt := range viewAlterStmts("Sale") {
		t.Run("persistent "+op, func(t *testing.T) {
			ctx, updated := viewAlterTestCtx(t)
			if err := validateWithContext(ctx, stmt, newScriptContext()); err != nil {
				t.Fatalf("check refused %s attribute on a persistent entity: %v", op, err)
			}
			if op == "drop" {
				return // DROP consults the catalog for references; the check-time path is what matters here
			}
			if err := execAlterEntity(ctx, stmt); err != nil {
				t.Fatalf("exec refused %s attribute on a persistent entity: %v", op, err)
			}
			if !*updated {
				t.Error("expected the persistent entity to be written")
			}
		})
	}
	t.Run("rename on view", func(t *testing.T) {
		ctx, _ := viewAlterTestCtx(t)
		stmt := &ast.AlterEntityStmt{
			Name:          ast.QualifiedName{Module: "Shop", Name: "SaleStats"},
			Operation:     ast.AlterEntityRenameAttribute,
			AttributeName: "Total", NewName: "GrandTotal",
		}
		if err := validateWithContext(ctx, stmt, newScriptContext()); err != nil {
			t.Fatalf("check refused RENAME ATTRIBUTE on a view entity: %v", err)
		}
	})
}
