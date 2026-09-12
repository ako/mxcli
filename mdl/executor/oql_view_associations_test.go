// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A view entity's association is declared by an `<alias>.ID` select column and
// by nothing else. These tests cover the recognition half — which column is an
// association, which entity it points at, and what that does to the alignment
// between columns and declared attributes. The writing half (the
// OqlViewAssociationSource, without which mxbuild reports CE6771) is measured
// end to end; see mdl-examples/bug-tests/view-entity-association.mdl.

func TestViewAssociationColumns_RecognisesTheIDColumn(t *testing.T) {
	cols := viewAssociationColumns(
		`select o.ID as persistent_order, o.OrderDate as order_date from Sales."Order" as o`)
	if len(cols) != 1 {
		t.Fatalf("got %d association columns, want 1: %+v", len(cols), cols)
	}
	if cols[0].Name != "persistent_order" {
		t.Errorf("Name = %q, want persistent_order — the select alias IS the association name", cols[0].Name)
	}
	// The quoted source entity is the reported example: `Order` is an OQL
	// reserved word, so it can only be written quoted, and the alias map has to
	// see through the quotes or the column is not recognised at all.
	if cols[0].Entity != "Sales.Order" {
		t.Errorf("Entity = %q, want Sales.Order", cols[0].Entity)
	}
}

func TestViewAssociationColumns_ResolvesAnAssociationPathJoin(t *testing.T) {
	// The ordinary way to reach a related entity in Mendix OQL. The alias map
	// used to match only `join Module.Entity as x`, so `m` resolved to nothing
	// and `m.ID` was invisible.
	cols := viewAssociationColumns(
		`from Trends.Reading as r join r/Trends.Reading_Meter/Trends.Meter as m ` +
			`group by m.ID select m.ID as MeterRef, sum(r.Kwh) as TotalKwh`)
	if len(cols) != 1 || cols[0].Entity != "Trends.Meter" || cols[0].Name != "MeterRef" {
		t.Fatalf("got %+v, want one column MeterRef -> Trends.Meter", cols)
	}
}

// TestViewAssociationColumns_LeavesEverythingElseAlone is the control, and it
// carries the design decision. `cast(m.ID as string) as MeterId` is a STRING
// ATTRIBUTE holding the object id as text — a legitimate and often better
// design (one statement instead of two, no objects materialised in the client),
// and treating it as an association would write a member the author did not ask
// for.
func TestViewAssociationColumns_LeavesEverythingElseAlone(t *testing.T) {
	for _, oql := range []string{
		`select cast(m.ID as string) as MeterId from Trends.Meter as m`,
		`select m.MeterCode as MeterCode from Trends.Meter as m`,
		`select sum(r.Kwh) as TotalKwh from Trends.Reading as r`,
		// An unresolvable alias: guessing an entity would write a dangling
		// reference, so this is not an association column.
		`select x.ID as Something from Trends.Meter as m`,
		// No alias at all — MDL030 reports that; there is no name to use.
		`select m.ID from Trends.Meter as m`,
	} {
		if cols := viewAssociationColumns(oql); len(cols) != 0 {
			t.Errorf("%s\n  -> unexpectedly read as an association: %+v", oql, cols)
		}
	}
}

// TestIDColumnDoesNotShiftTheAttributeAlignment is reported symptom 1. Columns
// are matched to declared attributes BY POSITION, so an unrecognised id column
// does not merely go unchecked — it moves every attribute after it onto its
// neighbour's expression, and the resulting message describes a script nobody
// wrote.
func TestIDColumnDoesNotShiftTheAttributeAlignment(t *testing.T) {
	attrs := []ast.ViewAttribute{
		{Name: "MeterCode", Type: ast.DataType{Kind: ast.TypeString, Length: 200}},
		{Name: "Readings", Type: ast.DataType{Kind: ast.TypeInteger}},
		{Name: "TotalKwh", Type: ast.DataType{Kind: ast.TypeDecimal}},
	}
	const body = `cast(m.Code as string) as MeterCode, count(r.Kwh) as Readings, avg(r.Kwh) as TotalKwh`
	const from = ` from Trends.Reading as r join r/Trends.R_M/Trends.Meter as m`

	for _, c := range []struct{ name, oql string }{
		{"id first", `select m.ID as MeterRef, ` + body + from},
		{"id last", `select ` + body + `, m.ID as MeterRef` + from},
		{"no id column (control)", `select ` + body + from},
	} {
		if v := ValidateOQLTypes(c.oql, attrs); len(v) != 0 {
			t.Errorf("%s: %d spurious violation(s): %s", c.name, len(v), v[0].Message)
		}
	}

	// And the count check agrees: three attributes, three attribute columns,
	// however many association columns sit among them.
	cols := attributeSelectColumns(
		`select m.ID as MeterRef, `+body+from,
		parseSelectColumns(extractSelectClause(`select m.ID as MeterRef, `+body+from)))
	if len(cols) != 3 {
		t.Errorf("attribute columns = %d, want 3: %q", len(cols), cols)
	}
}

// TestIDColumnStillNeedsAnAlias: the alias is the association's name, so the
// MDL030 "no as alias" rule has to keep applying to an id column. Skipping
// association columns from the TYPE alignment must not skip them from the
// syntax rules.
func TestIDColumnStillNeedsAnAlias(t *testing.T) {
	ids := oqlRuleIDs(`select m.ID, m.MeterCode as MeterCode from Trends.Meter as m`)
	if !hasOQLRule(ids, "MDL030") {
		t.Errorf("an id column with no alias was accepted: %v", ids)
	}
}

func TestViewEntityAssociationRefusal_PointsAtTheColumnForm(t *testing.T) {
	err := viewEntityAssociationRefusal("Trends.MeterRef", "Trends.MeterTotalsVE")
	msg := err.Error()
	for _, want := range []string{"CE6771", "select t.ID as MeterRef"} {
		if !strings.Contains(msg, want) {
			t.Errorf("refusal does not mention %q:\n%s", want, msg)
		}
	}
	// The old advice was a workaround for a capability mxcli now has; it must
	// not survive, or the message sends the author away from the real answer.
	if strings.Contains(msg, "non-persistent entity") {
		t.Errorf("refusal still recommends the pre-fix workaround:\n%s", msg)
	}
}

func TestShortAssociationName(t *testing.T) {
	if got := shortAssociationName("Mod.Assoc"); got != "Assoc" {
		t.Errorf("got %q, want Assoc", got)
	}
	if got := shortAssociationName("Assoc"); got != "Assoc" {
		t.Errorf("got %q, want Assoc", got)
	}
}
