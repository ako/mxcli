// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A DateTime attribute's Studio Pro "Localize" setting is spelled as a
// constraint after the type (#1373): `not localized` is LocalizeDate = false,
// `localized` states the default, and no clause leaves it unstated.
func TestDateTimeLocalizedConstraint_CreateEntity(t *testing.T) {
	prog, errs := Build(`create persistent entity Shop.Order (
  OrderDate: DateTime,
  BirthDate: DateTime not localized,
  ShipDate: DateTime localized not null,
  Due: DateTime not null not localized
);`)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	want := map[string]ast.LocalizeSpec{
		"OrderDate": ast.LocalizeUnstated,
		"BirthDate": ast.LocalizeNotLocalized,
		"ShipDate":  ast.LocalizeLocalized,
		"Due":       ast.LocalizeNotLocalized,
	}
	for _, a := range s.Attributes {
		if a.Type.Localize != want[a.Name] {
			t.Errorf("%s Localize = %v, want %v", a.Name, a.Type.Localize, want[a.Name])
		}
	}
	// `not localized` must not be mistaken for `not null`.
	for _, a := range s.Attributes {
		if wantNN := a.Name == "ShipDate" || a.Name == "Due"; a.NotNull != wantNN {
			t.Errorf("%s NotNull = %v, want %v", a.Name, a.NotNull, wantNN)
		}
	}
}

func TestDateTimeLocalizedConstraint_AlterEntity(t *testing.T) {
	cases := map[string]ast.LocalizeSpec{
		`alter entity Shop.Order modify attribute BirthDate: DateTime not localized;`: ast.LocalizeNotLocalized,
		`alter entity Shop.Order modify attribute BirthDate: DateTime localized;`:     ast.LocalizeLocalized,
		`alter entity Shop.Order modify attribute BirthDate: DateTime;`:               ast.LocalizeUnstated,
	}
	for src, want := range cases {
		prog, errs := Build(src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", src, errs)
		}
		s := prog.Statements[0].(*ast.AlterEntityStmt)
		if s.DataType.Localize != want {
			t.Errorf("%s: Localize = %v, want %v", src, s.DataType.Localize, want)
		}
	}
	prog, errs := Build(`alter entity Shop.Order add attribute Born: DateTime not localized;`)
	if len(errs) > 0 {
		t.Fatalf("add attribute: parse: %v", errs)
	}
	if got := prog.Statements[0].(*ast.AlterEntityStmt).Attribute.Type.Localize; got != ast.LocalizeNotLocalized {
		t.Errorf("add attribute: Localize = %v, want not localized", got)
	}
}

// LocalizeDate exists only on DomainModels$DateTimeAttributeType. On any other
// type the clause has nowhere to go, so it is refused rather than dropped.
func TestDateTimeLocalizedConstraint_RefusedOnOtherTypes(t *testing.T) {
	for _, src := range []string{
		`create persistent entity Shop.Order (Count: Integer not localized);`,
		`create persistent entity Shop.Order (Name: String(20) localized);`,
		`alter entity Shop.Order modify attribute Count: Integer not localized;`,
		`alter entity Shop.Order add attribute Name: String(20) not localized;`,
	} {
		_, errs := Build(src)
		if len(errs) == 0 {
			t.Errorf("%s: accepted, want an error", src)
			continue
		}
		if !strings.Contains(errs[0].Error(), "applies only to a DateTime attribute") {
			t.Errorf("%s: error %q does not explain the refusal", src, errs[0])
		}
	}
	// Control: the deprecated `date` is a DateTime, so it takes the clause.
	if _, errs := Build(`create persistent entity Shop.Order (D: Date not localized);`); len(errs) > 0 {
		t.Errorf("Date not localized refused: %v", errs)
	}
}

// LOCALIZED is a keyword now; it must stay usable as a name.
func TestLocalizedStaysAnIdentifier(t *testing.T) {
	if _, errs := Build(`create persistent entity Shop.Localized (Localized: Boolean);`); len(errs) > 0 {
		t.Errorf("`Localized` as a name: %v", errs)
	}
}
