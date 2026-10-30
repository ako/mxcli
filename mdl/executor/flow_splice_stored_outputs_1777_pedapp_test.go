// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1777 on the Studio Pro-authored PedApp, through the real codec: a
// fragment an alter inserts names a member of a stored list operation's, Java
// action's or self-reference retrieve's output, unqualified. It was refused
// ("cannot qualify member …") with nothing written; it is written qualified,
// as `create microflow` writes it, and check predicts it.

const storedShapesSetup = `mdl 1;
create module MyModule;
create persistent entity MyModule.Order (Code: String(50));
create persistent entity MyModule.OrderLine (Code: String(50), Status: String(20));
create association MyModule.OrderLine_Order from MyModule.OrderLine to MyModule.Order;
create association MyModule.Order_Parent from MyModule.Order to MyModule.Order;
create association MyModule.Order_Related from MyModule.Order to MyModule.Order type ReferenceSet;
create java action MyModule.JA_GetOrder (Code: String) returns MyModule.Order as $$
  return null;
$$;
create java action MyModule.JA_GetLines (Code: String) returns list of MyModule.OrderLine as $$
  return new java.util.ArrayList<>();
$$;
create java action MyModule.JA_Shuffle<T> (Items: list of T) returns list of T as $$
  return Items;
$$;
create microflow MyModule.SUB_Shapes ($Order: MyModule.Order, $Lines: List of MyModule.OrderLine)
begin
  $Open = filter $Lines by Status = 'Open';
  $Sorted = sort $Lines by Code asc;
  $First = head $Lines;
  $Got = call java action MyModule.JA_GetOrder(Code = 'A');
  $GotLines = call java action MyModule.JA_GetLines(Code = 'A');
  $Shuffled = call java action MyModule.JA_Shuffle(Items = $Lines);
  retrieve $Parent from $Order/MyModule.Order_Parent;
  retrieve $Related from $Order/MyModule.Order_Related;
  log info node 'Shapes' 'done';
end;`

// storedShapesFlow returns the stored MyModule.SUB_Shapes.
func storedShapesFlow(t *testing.T, exec *Executor) *microflows.Microflow {
	t.Helper()
	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	all, err := ctx.Backend.ListMicroflows()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range all {
		if m.Name == "SUB_Shapes" && h.GetModuleName(h.FindModuleID(m.ContainerID)) == "MyModule" {
			mf, err := ctx.Backend.GetMicroflow(m.ID)
			if err != nil {
				t.Fatal(err)
			}
			return mf
		}
	}
	t.Fatal("MyModule.SUB_Shapes not found")
	return nil
}

// memberWrittenFor is the member the stored activity outputting (a list
// operation) or changing (a change) variable writes, or "".
func memberWrittenFor(mf *microflows.Microflow, variable string) string {
	for _, obj := range mf.ObjectCollection.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch x := act.Action.(type) {
		case *microflows.ChangeObjectAction:
			if x.ChangeVariable == variable && len(x.Changes) == 1 {
				return x.Changes[0].AttributeQualifiedName
			}
		case *microflows.ListOperationAction:
			if x.OutputVariable != variable {
				continue
			}
			switch op := x.Operation.(type) {
			case *microflows.FindByAttributeOperation:
				return op.Attribute
			case *microflows.FilterByAttributeOperation:
				return op.Attribute
			}
		}
	}
	return ""
}

func TestAlterMicroflow_MembersOfStoredOutputsAreQualified(t *testing.T) {
	cases := []struct {
		name, stmt, variable, want string
	}{
		{"a filter's output", `$OpenA = find $Open by Code = 'A';`, "OpenA", "MyModule.OrderLine.Code"},
		{"a sort's output", `$SortedB = filter $Sorted by Code = 'B';`, "SortedB", "MyModule.OrderLine.Code"},
		{"a head's output", `change $First (Status = 'Seen');`, "First", "MyModule.OrderLine.Status"},
		{"a Java action's object", `change $Got (Code = 'X');`, "Got", "MyModule.Order.Code"},
		{"a Java action's list", `$GotOpen = find $GotLines by Status = 'Open';`, "GotOpen", "MyModule.OrderLine.Status"},
		{"a Java action's list of its type parameter", `$ShuffledA = find $Shuffled by Code = 'A';`, "ShuffledA", "MyModule.OrderLine.Code"},
		{"a self-reference's object", `change $Parent (Code = 'P');`, "Parent", "MyModule.Order.Code"},
		{"a self reference set's list", `$RelatedA = find $Related by Code = 'A';`, "RelatedA", "MyModule.Order.Code"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			exec, out, _ := openPedAppCopy(t)
			if err := agreeExec(t, exec, storedShapesSetup); err != nil {
				t.Fatalf("setup: %v\n%s", err, out.String())
			}
			predictedAgree(t, exec, "mdl 1;\nalter microflow MyModule.SUB_Shapes {\n  insert after $Related begin "+
				c.stmt+" end;\n};", "")
			if got := memberWrittenFor(storedShapesFlow(t, exec), c.variable); got != c.want {
				t.Errorf("%s wrote %q, want %q", c.stmt, got, c.want)
			}
		})
	}
}
