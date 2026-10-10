// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// notLocalizedFixture is module Shop with entity Person: BirthDate stored
// non-localized (Studio Pro's "Localize" unticked), CreatedAt localized. It
// records every entity the backend is asked to create or update.
type notLocalizedFixture struct {
	ctx     *ExecContext
	out     *bytes.Buffer
	written []*domainmodel.Entity
}

func newNotLocalizedFixture(t *testing.T) *notLocalizedFixture {
	t.Helper()
	mod := mkModule("Shop")
	person := mkEntity(mod.ID, "Person")
	person.Persistable = true
	person.Attributes = []*domainmodel.Attribute{
		{BaseElement: model.BaseElement{ID: "attr-birth"}, Name: "BirthDate", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: false}},
		{BaseElement: model.BaseElement{ID: "attr-created"}, Name: "CreatedAt", Type: &domainmodel.DateTimeAttributeType{LocalizeDate: true}},
	}
	dm := mkDomainModel(mod.ID, person)
	f := &notLocalizedFixture{}
	record := func(_ model.ID, e *domainmodel.Entity) error {
		f.written = append(f.written, e)
		return nil
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		CreateEntityFunc:     record,
		UpdateEntityFunc:     record,
	}
	f.ctx, f.out = newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	return f
}

// localizeOf returns the LocalizeDate the last write gave the named attribute.
func (f *notLocalizedFixture) localizeOf(t *testing.T, attr string) bool {
	t.Helper()
	if len(f.written) == 0 {
		t.Fatal("nothing was written")
	}
	e := f.written[len(f.written)-1]
	for _, a := range e.Attributes {
		if a.Name == attr {
			dt, ok := a.Type.(*domainmodel.DateTimeAttributeType)
			if !ok {
				t.Fatalf("%s is %T, want DateTime", attr, a.Type)
			}
			return dt.LocalizeDate
		}
	}
	t.Fatalf("%s.%s was not written", e.Name, attr)
	return false
}

func (f *notLocalizedFixture) exec(t *testing.T, src string) {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse %q: %v", src, errs)
	}
	for _, stmt := range prog.Statements {
		var err error
		switch s := stmt.(type) {
		case *ast.CreateEntityStmt:
			err = execCreateEntity(f.ctx, s)
		case *ast.AlterEntityStmt:
			err = execAlterEntity(f.ctx, s)
		default:
			t.Fatalf("unexpected statement %T", stmt)
		}
		if err != nil {
			t.Fatalf("exec %q: %v", src, err)
		}
	}
}

// A new attribute: `not localized` is false, no clause and `localized` are
// Mendix's default, true (#1373).
func TestCreateEntity_NotLocalizedReachesBackend(t *testing.T) {
	f := newNotLocalizedFixture(t)
	f.exec(t, `create persistent entity Shop.Order (
  OrderDate: DateTime,
  BirthDate: DateTime not localized,
  ShipDate: DateTime localized
);`)
	for attr, want := range map[string]bool{"OrderDate": true, "BirthDate": false, "ShipDate": true} {
		if got := f.localizeOf(t, attr); got != want {
			t.Errorf("Order.%s LocalizeDate = %v, want %v", attr, got, want)
		}
	}
}

// `create or modify` that does not state it keeps the stored value (#743); one
// that states it wins over the stored value in both directions (#1373).
func TestCreateOrModifyEntity_StatedLocalizeWinsOverCarry(t *testing.T) {
	cases := []struct {
		name, body          string
		wantBirth, wantCrea bool
	}{
		{"unstated carries the stored values", "BirthDate: DateTime, CreatedAt: DateTime", false, true},
		{"stated flips both", "BirthDate: DateTime localized, CreatedAt: DateTime not localized", true, false},
		{"stated equal to stored", "BirthDate: DateTime not localized, CreatedAt: DateTime localized", false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newNotLocalizedFixture(t)
			f.exec(t, "create or modify persistent entity Shop.Person ("+c.body+");")
			if got := f.localizeOf(t, "BirthDate"); got != c.wantBirth {
				t.Errorf("BirthDate LocalizeDate = %v, want %v", got, c.wantBirth)
			}
			if got := f.localizeOf(t, "CreatedAt"); got != c.wantCrea {
				t.Errorf("CreatedAt LocalizeDate = %v, want %v", got, c.wantCrea)
			}
		})
	}
}

// `alter entity … modify attribute` rebuilt the type from the statement, so a
// stored non-localized DateTime came back localized whenever the attribute was
// modified at all. Unstated now carries the stored value; stated wins.
func TestAlterModifyAttribute_LocalizeDate(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{`alter entity Shop.Person modify attribute BirthDate: DateTime;`, false},
		{`alter entity Shop.Person modify attribute BirthDate: DateTime not null;`, false},
		{`alter entity Shop.Person modify attribute BirthDate: DateTime localized;`, true},
		{`alter entity Shop.Person modify attribute CreatedAt: DateTime not localized;`, false},
	}
	for _, c := range cases {
		t.Run(c.src, func(t *testing.T) {
			f := newNotLocalizedFixture(t)
			f.exec(t, c.src)
			attr := "BirthDate"
			if strings.Contains(c.src, "CreatedAt") {
				attr = "CreatedAt"
			}
			if got := f.localizeOf(t, attr); got != c.want {
				t.Errorf("%s LocalizeDate = %v, want %v", attr, got, c.want)
			}
		})
	}
}

// describe prints ` not localized` for LocalizeDate = false and nothing for the
// default, and executing its output on a fresh model gives the same values: the
// describe → exec round trip.
func TestDescribeEntity_NotLocalizedRoundTrips(t *testing.T) {
	f := newNotLocalizedFixture(t)
	if err := describeEntity(f.ctx, ast.QualifiedName{Module: "Shop", Name: "Person"}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	out := f.out.String()
	if !strings.Contains(out, "BirthDate: DateTime not localized") {
		t.Errorf("describe does not say BirthDate is not localized:\n%s", out)
	}
	if strings.Contains(out, "CreatedAt: DateTime localized") || strings.Contains(out, "CreatedAt: DateTime not localized") {
		t.Errorf("describe states the default on CreatedAt:\n%s", out)
	}

	// Execute the described MDL as a NEW entity, so nothing can be carried
	// from the stored one: the values come from the text alone.
	script := strings.Replace(out, "entity Shop.Person", "entity Shop.PersonCopy", 1)
	g := newNotLocalizedFixture(t)
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("describe output does not parse: %v\n%s", errs, script)
	}
	for _, stmt := range prog.Statements {
		if s, ok := stmt.(*ast.CreateEntityStmt); ok {
			if err := execCreateEntity(g.ctx, s); err != nil {
				t.Fatalf("exec describe output: %v", err)
			}
		}
	}
	if got := g.localizeOf(t, "BirthDate"); got {
		t.Error("round trip: BirthDate came back localized")
	}
	if got := g.localizeOf(t, "CreatedAt"); !got {
		t.Error("round trip: CreatedAt came back not localized")
	}
}

// On a view entity LocalizeDate is derived from the OQL source (#1297); a
// stated clause wins over the derivation, like everywhere else.
func TestCreateViewEntity_StatedLocalizeWinsOverDerived(t *testing.T) {
	for clause, want := range map[string]bool{"": false, " localized": true, " not localized": false} {
		ctx, created := localizeFixture(t)
		s := parseViewStmt(t, `create view entity Shop.SalePerDay (
  D: DateTime`+clause+`
) as (
  select s.SaleDate as D
  from Shop.Sale as s
);`)
		if err := execCreateViewEntity(ctx, s); err != nil {
			t.Fatalf("exec: %v", err)
		}
		if got := (*created)[0].Attributes[0].Type.(*domainmodel.DateTimeAttributeType).LocalizeDate; got != want {
			t.Errorf("clause %q: LocalizeDate = %v, want %v", clause, got, want)
		}
	}
}
