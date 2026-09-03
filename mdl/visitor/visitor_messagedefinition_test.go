// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func buildOne[T ast.Statement](t *testing.T, src string) T {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse error: %v", errs[0])
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(T)
	if !ok {
		t.Fatalf("got %T, want %T", prog.Statements[0], *new(T))
	}
	return stmt
}

// A message definition collection is a selection over the domain model, which is
// what makes it authorable where a mapping's other non-JSON sources are not
// (ako/mxcli#272). It is the source for 74 of the 327 mappings in the demo
// corpus.

func TestCreateMessageDefinitionCollection(t *testing.T) {
	s := buildOne[*ast.CreateMessageDefinitionCollectionStmt](t, `create message definition collection Sales.MD_Order
  folder 'Private/Messages'
(
  definition Order for Sales.Order as 'Orders' (
    OrderId,
    Total as 'GrandTotal',
    Sales.Order_Line/Sales.Line as 'Lines' ( Sku, Quantity ),
    Sales.Order_Customer/Sales.Customer ( Name )
  ),
  definition Line for Sales.Line ( Sku )
);`)

	if s.Name.String() != "Sales.MD_Order" || s.Folder != "Private/Messages" {
		t.Fatalf("name=%q folder=%q", s.Name.String(), s.Folder)
	}
	if len(s.Definitions) != 2 {
		t.Fatalf("got %d definitions, want 2", len(s.Definitions))
	}

	d := s.Definitions[0]
	// The definition's Name and its root's ExposedName are independent — 19 of
	// 56 real definitions are named something other than their entity.
	if d.Name != "Order" || d.Entity.String() != "Sales.Order" || d.ExposedName != "Orders" {
		t.Errorf("definition = %+v", d)
	}
	if len(d.Members) != 4 {
		t.Fatalf("got %d members, want 4", len(d.Members))
	}

	if d.Members[0].IsAssociation() || d.Members[0].Attribute != "OrderId" || d.Members[0].ExposedName != "" {
		t.Errorf("member 0 = %+v, want the attribute OrderId with no rename", d.Members[0])
	}
	if d.Members[1].ExposedName != "GrandTotal" {
		t.Errorf("member 1 exposed name = %q", d.Members[1].ExposedName)
	}

	// An association names its TARGET entity. That is load-bearing rather than
	// decorative: the stored MaxOccurs tracks the DIRECTION of traversal, not
	// the association's type, so the target is what makes the direction explicit.
	assoc := d.Members[2]
	if !assoc.IsAssociation() {
		t.Fatalf("member 2 is not an association: %+v", assoc)
	}
	if assoc.Association.String() != "Sales.Order_Line" || assoc.Entity.String() != "Sales.Line" {
		t.Errorf("association = %s -> %s", assoc.Association.String(), assoc.Entity.String())
	}
	if assoc.ExposedName != "Lines" || len(assoc.Members) != 2 {
		t.Errorf("association exposed=%q members=%d", assoc.ExposedName, len(assoc.Members))
	}
	if d.Members[3].ExposedName != "" || len(d.Members[3].Members) != 1 {
		t.Errorf("member 3 = %+v", d.Members[3])
	}
}

func TestCreateMessageDefinitionCollectionOrModify(t *testing.T) {
	s := buildOne[*ast.CreateMessageDefinitionCollectionStmt](t,
		`create or modify message definition collection S.MD ( definition A for S.A ( X ) );`)
	if !s.CreateOrModify {
		t.Error("OR MODIFY not recorded — a fresh document would break every WITH MESSAGE DEFINITION")
	}
}

func TestAlterMessageDefinitionCollection(t *testing.T) {
	add := buildOne[*ast.AlterMessageDefinitionCollectionStmt](t,
		`alter message definition collection S.MD add definition X for S.X as 'Xs' ( A );`)
	if add.Op != "ADD" || add.Definition == nil {
		t.Fatalf("add = %+v", add)
	}
	if add.Definition.Name != "X" || add.Definition.ExposedName != "Xs" || len(add.Definition.Members) != 1 {
		t.Errorf("added definition = %+v", add.Definition)
	}

	drop := buildOne[*ast.AlterMessageDefinitionCollectionStmt](t,
		`alter message definition collection S.MD drop definition if exists X;`)
	if drop.Op != "DROP" || drop.Target != "X" || !drop.IfExists {
		t.Errorf("drop = %+v", drop)
	}

	ren := buildOne[*ast.AlterMessageDefinitionCollectionStmt](t,
		`alter message definition collection S.MD rename definition X to Y;`)
	if ren.Op != "RENAME" || ren.Target != "X" || ren.NewName != "Y" {
		t.Errorf("rename = %+v", ren)
	}
}

// TestAlterMessageDefinitionSplitsTheThreePartName pins the address. A
// definition is referred to as Module.Collection.Definition — the same reference
// WITH MESSAGE DEFINITION takes — so the two cannot drift apart.
func TestAlterMessageDefinitionSplitsTheThreePartName(t *testing.T) {
	s := buildOne[*ast.AlterMessageDefinitionStmt](t,
		`alter message definition Sales.MD_Order.Order add member Total;`)
	if s.Collection.String() != "Sales.MD_Order" || s.Definition != "Order" {
		t.Fatalf("collection=%q definition=%q", s.Collection.String(), s.Definition)
	}
	if s.Op != "ADD" || s.Member == nil || s.Member.Attribute != "Total" {
		t.Errorf("stmt = %+v member=%+v", s, s.Member)
	}
}

func TestAlterMessageDefinitionRejectsATwoPartName(t *testing.T) {
	_, errs := Build(`alter message definition Sales.MD_Order add member Total;`)
	if len(errs) == 0 {
		t.Fatal("accepted a two-part name — the collection and definition would be indistinguishable")
	}
}

// TestAlterMessageDefinitionMemberPath pins the nested address. Members nest to
// depth 7 in the corpus, so reaching one is the common case, not an edge case.
func TestAlterMessageDefinitionMemberPath(t *testing.T) {
	drop := buildOne[*ast.AlterMessageDefinitionStmt](t,
		`alter message definition S.MD.D drop member Sku in Lines/Prices;`)
	if drop.Op != "DROP" || drop.Target != "Sku" {
		t.Fatalf("drop = %+v", drop)
	}
	// The path's segments and the operation's own identifier must not be
	// confused: both are IdentifierOrKeyword, and the path lives in its own
	// sub-rule precisely so they stay apart.
	if len(drop.Path) != 2 || drop.Path[0] != "Lines" || drop.Path[1] != "Prices" {
		t.Errorf("path = %v, want [Lines Prices]", drop.Path)
	}
}

// TestAlterMessageDefinitionSetIsNotARename pins that SET carries an exposed
// name and no model rename. ALTER ENTITY's RENAME ATTRIBUTE changes the model
// and rewrites every reference; this changes one element's ExposedName.
func TestAlterMessageDefinitionSetIsNotARename(t *testing.T) {
	s := buildOne[*ast.AlterMessageDefinitionStmt](t,
		`alter message definition S.MD.D set member Total as 'GrandTotal';`)
	if s.Op != "SET" || s.Target != "Total" || s.ExposedName != "GrandTotal" {
		t.Errorf("stmt = %+v", s)
	}
}

func TestDropMessageDefinitionCollection(t *testing.T) {
	s := buildOne[*ast.DropMessageDefinitionCollectionStmt](t,
		`drop message definition collection Sales.MD_Order;`)
	if s.Name.String() != "Sales.MD_Order" {
		t.Errorf("name = %q", s.Name.String())
	}
}
