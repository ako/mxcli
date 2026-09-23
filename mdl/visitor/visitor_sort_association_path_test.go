// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1152 — a sort column may navigate associations, one `/` per
// hop, with the final segment naming the attribute. Without the spelling,
// DESCRIBE had to drop the hop and the replay had to guess which association
// was meant.
func sortColumnsOf(t *testing.T, sortClause string) []ast.SortColumnDef {
	t.Helper()
	input := `CREATE MICROFLOW Sales.ListOrders ()
BEGIN
  RETRIEVE $Orders FROM Sales.Order ` + sortClause + `;
  RETURN;
END;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors for %q: %v", sortClause, errs)
	}
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("statement = %T, want *ast.CreateMicroflowStmt", prog.Statements[0])
	}
	retr, ok := mf.Body[0].(*ast.RetrieveStmt)
	if !ok {
		t.Fatalf("body[0] = %T, want *ast.RetrieveStmt", mf.Body[0])
	}
	return retr.SortColumns
}

func TestSortColumn_AssociationPathParses(t *testing.T) {
	cols := sortColumnsOf(t, "SORT BY Sales.Order_BillTo/Sales.Address.City ASC")
	if len(cols) != 1 {
		t.Fatalf("got %d sort columns, want 1", len(cols))
	}
	if got := strings.Join(cols[0].Associations, ","); got != "Sales.Order_BillTo" {
		t.Errorf("Associations = %q, want Sales.Order_BillTo", got)
	}
	if cols[0].Attribute != "Sales.Address.City" {
		t.Errorf("Attribute = %q, want Sales.Address.City — the LAST segment is the attribute", cols[0].Attribute)
	}
	if cols[0].Order != "ASC" {
		t.Errorf("Order = %q, want ASC", cols[0].Order)
	}
}

func TestSortColumn_MultipleHopsParse(t *testing.T) {
	cols := sortColumnsOf(t, "SORT BY Order_Customer/Customer_Country/Name DESC")
	if len(cols) != 1 {
		t.Fatalf("got %d sort columns, want 1", len(cols))
	}
	if got := strings.Join(cols[0].Associations, ","); got != "Order_Customer,Customer_Country" {
		t.Errorf("Associations = %q, want Order_Customer,Customer_Country", got)
	}
	if cols[0].Attribute != "Name" || cols[0].Order != "DESC" {
		t.Errorf("got {%q, %q}, want {Name, DESC}", cols[0].Attribute, cols[0].Order)
	}
}

// CONTROL: a sort with no hop must still parse to no hops and the whole name as
// the attribute. A change that read every qualifiedName as a hop would leave
// ordinary sorts with an empty attribute.
func TestSortColumn_PlainSortHasNoHops(t *testing.T) {
	for _, clause := range []string{"SORT BY Name ASC", "SORT BY Sales.Order.Name ASC"} {
		cols := sortColumnsOf(t, clause)
		if len(cols) != 1 {
			t.Fatalf("%s: got %d sort columns, want 1", clause, len(cols))
		}
		if len(cols[0].Associations) != 0 {
			t.Errorf("%s: Associations = %v, want none", clause, cols[0].Associations)
		}
		if cols[0].Attribute == "" {
			t.Errorf("%s: Attribute is empty", clause)
		}
	}
}

// Multiple sort columns, one with hops and one without, keep their own paths.
func TestSortColumn_MixedColumns(t *testing.T) {
	cols := sortColumnsOf(t, "SORT BY Sales.Order_BillTo/City ASC, OrderNo DESC")
	if len(cols) != 2 {
		t.Fatalf("got %d sort columns, want 2", len(cols))
	}
	if len(cols[0].Associations) != 1 || cols[0].Attribute != "City" {
		t.Errorf("col[0] = %+v, want one hop and City", cols[0])
	}
	if len(cols[1].Associations) != 0 || cols[1].Attribute != "OrderNo" {
		t.Errorf("col[1] = %+v, want no hops and OrderNo", cols[1])
	}
}
