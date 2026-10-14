// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1288: ADD/REMOVE could only target a list variable, so the
// only way to append to a reference set was `change $Obj (Assoc = $X)`, which
// assigns the whole set and orphans what was there. `add … to $Obj/Assoc` is
// the Add/Remove member change Studio Pro's Change object dialog offers.
func TestAddRemoveAcceptAssociationTarget(t *testing.T) {
	input := `CREATE MICROFLOW Sales.AttachLine ($Order: Sales.Order, $Line: Sales.Line, $Lines: List of Sales.Line)
RETURNS Boolean
BEGIN
  ADD $Line TO $Order/Sales.Order_Line;
  ADD $Lines TO $Order/Sales.Order_Line COMMIT REFRESH;
  REMOVE $Line FROM $Order/Sales."Order_Line";
  RETURN true;
END;`

	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, err := range errs {
			t.Errorf("Parse error: %v", err)
		}
		return
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)

	add, ok := mf.Body[0].(*ast.AddToListStmt)
	if !ok {
		t.Fatalf("Body[0] = %T, want AddToListStmt", mf.Body[0])
	}
	if add.List != "Order" || add.Association != "Sales.Order_Line" || add.Item != "Line" {
		t.Fatalf("add = List %q Association %q Item %q, want Order / Sales.Order_Line / Line", add.List, add.Association, add.Item)
	}
	if add.Commit != ast.CommitNo || add.RefreshInClient {
		t.Fatalf("plain add carries Commit %v Refresh %v", add.Commit, add.RefreshInClient)
	}

	addAll := mf.Body[1].(*ast.AddToListStmt)
	if addAll.Association != "Sales.Order_Line" || addAll.Commit != ast.CommitYes || !addAll.RefreshInClient {
		t.Fatalf("add with modifiers = %+v", addAll)
	}

	rm, ok := mf.Body[2].(*ast.RemoveFromListStmt)
	if !ok {
		t.Fatalf("Body[2] = %T, want RemoveFromListStmt", mf.Body[2])
	}
	if rm.List != "Order" || rm.Association != "Sales.Order_Line" || rm.Item != "Line" {
		t.Fatalf("remove = List %q Association %q Item %q", rm.List, rm.Association, rm.Item)
	}
}

// The list forms keep their meaning: no association, List is the variable.
func TestAddRemoveListTargetHasNoAssociation(t *testing.T) {
	input := `CREATE MICROFLOW Sales.Collect ($Line: Sales.Line)
RETURNS Boolean
BEGIN
  DECLARE $Lines List of Sales.Line = empty;
  ADD $Line TO $Lines;
  REMOVE $Line FROM $Lines;
  RETURN true;
END;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if a := mf.Body[1].(*ast.AddToListStmt); a.List != "Lines" || a.Association != "" {
		t.Fatalf("add = %+v", a)
	}
	if r := mf.Body[2].(*ast.RemoveFromListStmt); r.List != "Lines" || r.Association != "" {
		t.Fatalf("remove = %+v", r)
	}
}

// A list operation has no commit/refresh setting, so the modifiers belong to
// the association form only.
func TestAddToListRefusesChangeModifiers(t *testing.T) {
	input := `CREATE MICROFLOW Sales.Collect ($Line: Sales.Line)
RETURNS Boolean
BEGIN
  DECLARE $Lines List of Sales.Line = empty;
  ADD $Line TO $Lines REFRESH;
  RETURN true;
END;`
	if _, errs := Build(input); len(errs) == 0 {
		t.Fatal("ADD … TO $List REFRESH parsed; a Change list activity has no refresh setting")
	}
}

// mendixlabs/mxcli#1473: the Add and Remove member changes have no spelling
// inside a `change` with other members — MDL had only `=`. `+=` and `-=` are
// those operators; `=` stays a set assignment.
func TestChangeMemberAddRemoveOperators(t *testing.T) {
	input := `CREATE MICROFLOW Sales.Rework ($Order: Sales.Order, $Line: Sales.Line, $Old: Sales.Line)
RETURNS Boolean
BEGIN
  CHANGE $Order (Number = 1, Sales.Order_Line += $Line, Sales."Order_Line" -= $Old) COMMIT;
  RETURN true;
END;`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse errors: %v", errs)
	}
	mf := prog.Statements[0].(*ast.CreateMicroflowStmt)
	ch, ok := mf.Body[0].(*ast.ChangeObjectStmt)
	if !ok {
		t.Fatalf("Body[0] = %T, want ChangeObjectStmt", mf.Body[0])
	}
	want := []struct {
		attr string
		op   ast.MemberChangeOp
	}{
		{"Number", ast.MemberSet},
		{"Sales.Order_Line", ast.MemberAdd},
		{"Sales.Order_Line", ast.MemberRemove},
	}
	if len(ch.Changes) != len(want) {
		t.Fatalf("Changes = %+v", ch.Changes)
	}
	for i, w := range want {
		if ch.Changes[i].Attribute != w.attr || ch.Changes[i].Op != w.op {
			t.Errorf("Changes[%d] = %q op %v, want %q op %v", i, ch.Changes[i].Attribute, ch.Changes[i].Op, w.attr, w.op)
		}
	}
	if ch.Commit != ast.CommitYes {
		t.Errorf("commit dropped")
	}
}

// A Create object activity's members are initial values: there is no set to
// add to or remove from, so the operators belong to `change` only.
func TestCreateRefusesAddRemoveOperators(t *testing.T) {
	for _, op := range []string{"+=", "-="} {
		input := `CREATE MICROFLOW Sales.New ($Line: Sales.Line)
RETURNS Boolean
BEGIN
  $Order = CREATE Sales.Order (Sales.Order_Line ` + op + ` $Line);
  RETURN true;
END;`
		if _, errs := Build(input); len(errs) == 0 {
			t.Errorf("create with %s parsed without an error", op)
		}
	}
}
