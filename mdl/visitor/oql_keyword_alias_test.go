// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1174: an OQL source alias that happens to be an MDL keyword
// did not parse, so DESCRIBE output of a view that Studio Pro accepts could not
// be fed back to exec:
//
//	line 7:62 mismatched input 'ROLE' expecting IDENTIFIER
//	line 9:0 mismatched input ')' expecting {SELECT, HAVING}
//
// MDL's keywords are not OQL's, so the alias is legal in the model; after an
// explicit AS nothing but an alias can follow, so any keyword is accepted there.

func parseOQLView(t *testing.T, oql string) *ast.OQLParsed {
	t.Helper()
	src := "create view entity MyFirstModule.SaleStats (\n  Total: Integer\n) as (\n" + oql + "\n);"
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	s, ok := prog.Statements[0].(*ast.CreateViewEntityStmt)
	if !ok {
		t.Fatalf("not a CREATE VIEW ENTITY: %T", prog.Statements[0])
	}
	if s.Query.Parsed == nil {
		t.Fatal("no structured parse")
	}
	return s.Query.Parsed
}

func TestOQLKeywordAlias_JoinAsRole(t *testing.T) {
	// The reported query, verbatim.
	p := parseOQLView(t, `  SELECT
    SUM(s.Amount) as Total
  FROM MyFirstModule.Sale as s
  LEFT JOIN s/MyFirstModule.Sale_UserRole/System.UserRole AS ROLE
  GROUP BY ROLE.Name`)
	if len(p.Tables) != 2 || p.Tables[1].Alias != "ROLE" {
		t.Fatalf("tables = %+v, want the join aliased ROLE", p.Tables)
	}
	if p.GroupBy != "ROLE.Name" {
		t.Errorf("group by = %q", p.GroupBy)
	}
}

func TestOQLKeywordAlias_FromSubqueryAndPathStart(t *testing.T) {
	// A keyword alias on the FROM table, on a derived table, and as the start
	// of a later association path — every place a source alias is written.
	p := parseOQLView(t, `  select sum(role.Amount) as Total
  from MyFirstModule.Sale as role
  join role/MyFirstModule.Sale_Customer/MyFirstModule.Customer as Status
  join (select c.ID as Id from MyFirstModule.Customer as c) as Value on Value.Id = Status.ID`)
	want := []string{"role", "Status", "Value"}
	if len(p.Tables) != len(want) {
		t.Fatalf("tables = %+v", p.Tables)
	}
	for i, w := range want {
		if p.Tables[i].Alias != w {
			t.Errorf("table %d alias = %q, want %q", i, p.Tables[i].Alias, w)
		}
	}
}

func TestOQLKeywordAlias_BareKeywordIsStillAClause(t *testing.T) {
	// Control: without AS, a keyword after the source must stay the next clause.
	// `from M.Sale s left join …` is a LEFT JOIN, not a table aliased LEFT.
	p := parseOQLView(t, `  select sum(s.Amount) as Total
  from MyFirstModule.Sale s
  left join s/MyFirstModule.Sale_Customer/MyFirstModule.Customer c
  where s.Amount > 0`)
	if len(p.Tables) != 2 || p.Tables[0].Alias != "s" || p.Tables[1].Alias != "c" ||
		p.Tables[1].JoinType != "left join" {
		t.Fatalf("tables = %+v", p.Tables)
	}
	if p.Where != "s.Amount > 0" {
		t.Errorf("where = %q", p.Where)
	}
}
