// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1777, the follow-up to #1769: a fragment spliced into a stored
// flow starts from the variables the stored flow declares (storedVariables),
// and three kinds were left untyped there although `create microflow` types
// them — the output of a list operation, of a Java action call, and of a
// retrieve over a self-reference. A member of one was refused ("cannot qualify
// member …: the entity its variable holds is not known here") and nothing was
// written.

// storedOutputsFixture is a stored flow over Mod.Order (Code) and
// Mod.OrderLine (Code, Status). Order_Parent is a reference from Order to
// itself, Order_Related a reference set from Order to itself. The Java actions
// return an Order (GetOrder), a list of lines (GetLines), a list of their type
// parameter (Shuffle, typed from its list argument) and an Integer (CountLines).
func storedOutputsFixture(t *testing.T) (*ExecContext, *alterFlowContext) {
	t.Helper()
	mod := mkModule("Mod")
	attr := func(name string) *domainmodel.Attribute {
		return &domainmodel.Attribute{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: name}
	}
	order := mkEntity(mod.ID, "Order")
	order.Attributes = []*domainmodel.Attribute{attr("Code")}
	line := mkEntity(mod.ID, "OrderLine")
	line.Attributes = []*domainmodel.Attribute{attr("Code"), attr("Status")}
	tag := mkEntity(mod.ID, "Tag")
	tag.Attributes = []*domainmodel.Attribute{attr("Label")}
	dm := mkDomainModel(mod.ID, order, line, tag)
	related := mkAssociation(dm.ID, "Order_Related", order.ID, order.ID)
	related.Type = domainmodel.AssociationTypeReferenceSet
	dm.Associations = []*domainmodel.Association{
		mkAssociation(dm.ID, "OrderLine_Order", line.ID, order.ID),
		mkAssociation(dm.ID, "Order_Parent", order.ID, order.ID),
		related,
	}
	javaActions := map[string]*javaactions.JavaAction{
		"Mod.GetOrder":   {Name: "GetOrder", ReturnType: &javaactions.EntityType{Entity: "Mod.Order"}},
		"Mod.GetLines":   {Name: "GetLines", ReturnType: &javaactions.ListType{Entity: "Mod.OrderLine"}},
		"Mod.Shuffle":    {Name: "Shuffle", ReturnType: &javaactions.ListType{TypeParameter: "T"}},
		"Mod.CountLines": {Name: "CountLines", ReturnType: &javaactions.IntegerType{}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetModuleByNameFunc: func(name string) (*model.Module, error) {
			if name == "Mod" {
				return mod, nil
			}
			return nil, nil
		},
		GetDomainModelFunc: func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ReadJavaActionByNameFunc: func(qn string) (*javaactions.JavaAction, error) {
			if ja, ok := javaActions[qn]; ok {
				return ja, nil
			}
			return nil, fmt.Errorf("java action %s not found", qn)
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	act := func(out string, a microflows.MicroflowAction) mfmutator.Candidate {
		return mfmutator.Candidate{OutputVariable: out, Object: &microflows.ActionActivity{Action: a}}
	}
	listOp := func(out string, op microflows.ListOperation) mfmutator.Candidate {
		return act(out, &microflows.ListOperationAction{OutputVariable: out, Operation: op})
	}
	aggregate := func(out string, fn microflows.AggregateFunction, reduce microflows.DataType) mfmutator.Candidate {
		return act(out, &microflows.AggregateListAction{InputVariable: "Lines", OutputVariable: out, Function: fn, ReduceReturnType: reduce})
	}
	java := func(out, action string, args ...string) mfmutator.Candidate {
		var mappings []*microflows.JavaActionParameterMapping
		for i, arg := range args {
			mappings = append(mappings, &microflows.JavaActionParameterMapping{
				Parameter: fmt.Sprintf("%s.P%d", action, i),
				Value:     &microflows.BasicCodeActionParameterValue{Argument: arg},
			})
		}
		return act(out, &microflows.JavaActionCallAction{JavaAction: action, ParameterMappings: mappings,
			ResultVariableName: out, UseReturnVariable: true})
	}
	over := func(out, start, assoc string) mfmutator.Candidate {
		return act(out, &microflows.RetrieveAction{OutputVariable: out,
			Source: &microflows.AssociationRetrieveSource{StartVariable: start, AssociationQualifiedName: "Mod." + assoc}})
	}
	a := &alterFlowContext{
		stmt: &ast.AlterFlowStmt{},
		mf: &microflows.Microflow{Parameters: []*microflows.MicroflowParameter{
			{Name: "Order", Type: &microflows.ObjectType{EntityQualifiedName: "Mod.Order"}},
			{Name: "Lines", Type: &microflows.ListType{EntityQualifiedName: "Mod.OrderLine"}},
			{Name: "More", Type: &microflows.ListType{EntityQualifiedName: "Mod.OrderLine"}},
			{Name: "Tags", Type: &microflows.ListType{EntityQualifiedName: "Mod.Tag"}},
		}},
		entityNames: map[model.ID]string{order.ID: "Mod.Order", line.ID: "Mod.OrderLine", tag.ID: "Mod.Tag"},
		cands: []mfmutator.Candidate{
			// List operations that keep the list.
			listOp("Filtered", &microflows.FilterByAttributeOperation{ListVariable: "Lines", Attribute: "Mod.OrderLine.Status", Expression: "'Open'"}),
			listOp("FilteredExpr", &microflows.FilterOperation{ListVariable: "Lines", Expression: "true"}),
			listOp("Rest", &microflows.TailOperation{ListVariable: "Lines"}),
			listOp("Page", &microflows.ListRangeOperation{ListVariable: "Lines", LimitExpression: "10"}),
			listOp("Joined", &microflows.UnionOperation{ListVariable1: "Lines", ListVariable2: "More"}),
			listOp("Common", &microflows.IntersectOperation{ListVariable1: "Lines", ListVariable2: "More"}),
			listOp("Diff", &microflows.SubtractOperation{ListVariable1: "Lines", ListVariable2: "More"}),
			// List operations that pick one object.
			listOp("First", &microflows.HeadOperation{ListVariable: "Lines"}),
			listOp("Found", &microflows.FindByAttributeOperation{ListVariable: "Lines", Attribute: "Mod.OrderLine.Code", Expression: "'A'"}),
			listOp("FoundExpr", &microflows.FindOperation{ListVariable: "Lines", Expression: "true"}),
			// Reads a list a later candidate types: the order of the
			// candidates is not the order a variable is typed in.
			listOp("FirstSorted", &microflows.HeadOperation{ListVariable: "Sorted"}),
			listOp("Sorted", &microflows.SortOperation{ListVariable: "Lines"}),
			// A Boolean, which holds no entity.
			listOp("Has", &microflows.ContainsOperation{ListVariable: "Lines", ObjectVariable: "First"}),
			listOp("Same", &microflows.EqualsOperation{ListVariable1: "Lines", ListVariable2: "More"}),
			// Aggregates hold a primitive.
			aggregate("Count", microflows.AggregateFunctionCount, nil),
			aggregate("Mean", microflows.AggregateFunctionAverage, nil),
			aggregate("Total", microflows.AggregateFunctionSum, nil),
			aggregate("AnyOpen", microflows.AggregateFunctionAny, &microflows.BooleanType{}),
			aggregate("Folded", microflows.AggregateFunctionReduce, &microflows.StringType{}),
			// Java action calls.
			java("Got", "Mod.GetOrder"),
			java("GotLines", "Mod.GetLines"),
			java("Shuffled", "Mod.Shuffle", "$Lines"),
			java("Number", "Mod.CountLines", "$Lines"),
			java("Lost", "Mod.NoSuchAction"),
			// Retrieves over a self-reference, and one started from it.
			over("Parent", "Order", "Order_Parent"),
			over("Grandparent", "Parent", "Order_Parent"),
			over("Related", "Order", "Order_Related"),
			over("ParentLines", "Parent", "OrderLine_Order"),
			// Two origins that give one type.
			listOp("Pick", &microflows.HeadOperation{ListVariable: "Lines"}),
			listOp("Pick", &microflows.FindOperation{ListVariable: "Lines", Expression: "true"}),
			// Two origins that give different types: which one a fragment
			// means depends on where it goes (two loop bodies can each declare
			// it).
			listOp("Dup", &microflows.HeadOperation{ListVariable: "Lines"}),
			listOp("Dup", &microflows.HeadOperation{ListVariable: "Tags"}),
			java("Mixed", "Mod.GetOrder"),
			listOp("Mixed", &microflows.HeadOperation{ListVariable: "Lines"}),
			listOp("DupFirst", &microflows.HeadOperation{ListVariable: "Dup"}),
		},
	}
	return ctx, a
}

// TestStoredVariables_ListOperationJavaAndSelfReferenceAreTyped: each output
// has the type `create microflow` gives it — a list operation's that of its
// input list (the list, or for head and find its element), a Java action's its
// declared return type (or, for a list of its type parameter, its list
// argument's), a retrieve over a self-reference what Mendix reads it as — and
// what cannot be established stays untyped.
func TestStoredVariables_ListOperationJavaAndSelfReferenceAreTyped(t *testing.T) {
	ctx, a := storedOutputsFixture(t)
	varTypes, declared := a.storedVariables(ctx)
	want := map[string]string{
		"Filtered":     "List of Mod.OrderLine",
		"FilteredExpr": "List of Mod.OrderLine",
		"Rest":         "List of Mod.OrderLine",
		"Page":         "List of Mod.OrderLine",
		"Joined":       "List of Mod.OrderLine",
		"Common":       "List of Mod.OrderLine",
		"Diff":         "List of Mod.OrderLine",
		"Sorted":       "List of Mod.OrderLine",
		"First":        "Mod.OrderLine",
		"Found":        "Mod.OrderLine",
		"FoundExpr":    "Mod.OrderLine",
		"FirstSorted":  "Mod.OrderLine",
		"Got":          "Mod.Order",
		"GotLines":     "List of Mod.OrderLine",
		"Shuffled":     "List of Mod.OrderLine",
		"Parent":       "Mod.Order",
		"Grandparent":  "Mod.Order",
		"Related":      "List of Mod.Order",
		"ParentLines":  "List of Mod.OrderLine",
		"Pick":         "Mod.OrderLine",
	}
	for v, w := range want {
		if got := varTypes[v]; got != w {
			t.Errorf("$%s: typed %q, want %q", v, got, w)
		}
		if d, ok := declared[v]; ok {
			t.Errorf("$%s: typed and also declared %q", v, d)
		}
	}
	wantDeclared := map[string]string{
		"Has":      "Unknown",
		"Same":     "Unknown",
		"Count":    "Integer",
		"Mean":     "Decimal",
		"Total":    "Unknown",
		"AnyOpen":  "Boolean",
		"Folded":   "String",
		"Number":   "Unknown",
		"Lost":     "Unknown",
		"Dup":      "Unknown",
		"Mixed":    "Unknown",
		"DupFirst": "Unknown",
	}
	for v, w := range wantDeclared {
		if got, ok := varTypes[v]; ok {
			t.Errorf("$%s: typed %q, want it left untyped", v, got)
		}
		if declared[v] != w {
			t.Errorf("$%s: declared %q, want %q", v, declared[v], w)
		}
	}
}

// fragmentMembers builds src's body as a spliced fragment and returns the
// member each list operation and change in it writes, in order.
func fragmentMembers(t *testing.T, ctx *ExecContext, a *alterFlowContext, src string) ([]string, error) {
	t.Helper()
	frag, err := a.buildFragment(ctx, parsedFlowBody(t, "mdl 1;\ncreate microflow Mod.Fragment ()\nbegin\n"+src+"\nend;"))
	if err != nil {
		return nil, err
	}
	var members []string
	for _, obj := range frag.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		switch x := act.Action.(type) {
		case *microflows.ChangeObjectAction:
			for _, m := range x.Changes {
				members = append(members, m.AttributeQualifiedName)
			}
		case *microflows.ListOperationAction:
			switch op := x.Operation.(type) {
			case *microflows.FindByAttributeOperation:
				members = append(members, op.Attribute)
			case *microflows.FilterByAttributeOperation:
				members = append(members, op.Attribute)
			case *microflows.SortOperation:
				for _, s := range op.Sorting {
					members = append(members, s.AttributeQualifiedName)
				}
			default:
				members = append(members, fmt.Sprintf("%T", op))
			}
		}
	}
	return members, nil
}

// TestSpliceFragment_MembersOfStoredOutputsAreQualified is the reported
// shape: a fragment naming a member of a stored list operation's, Java
// action's or self-reference retrieve's output, unqualified, writes it
// qualified with the entity the variable holds, as `create microflow` does.
func TestSpliceFragment_MembersOfStoredOutputsAreQualified(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		{"find in a filtered list", `$F = find $Filtered by Code = 'A';`, []string{"Mod.OrderLine.Code"}},
		{"filter a sorted list", `$F = filter $Sorted by Status = 'Open';`, []string{"Mod.OrderLine.Status"}},
		{"sort a union", `$S = sort $Joined by Code asc;`, []string{"Mod.OrderLine.Code"}},
		{"find in a tail", `$F = find $Rest by Code = 'A';`, []string{"Mod.OrderLine.Code"}},
		{"change a head", `change $First (Code = 'B');`, []string{"Mod.OrderLine.Code"}},
		{"change a find", `change $Found (Status = 'Done');`, []string{"Mod.OrderLine.Status"}},
		{"change a head of a sorted list", `change $FirstSorted (Code = 'B');`, []string{"Mod.OrderLine.Code"}},
		{"change a Java action's object", `change $Got (Code = 'B');`, []string{"Mod.Order.Code"}},
		{"find in a Java action's list", `$F = find $GotLines by Status = 'Open';`, []string{"Mod.OrderLine.Status"}},
		{"find in a Java action's list of its type parameter", `$F = find $Shuffled by Code = 'A';`, []string{"Mod.OrderLine.Code"}},
		{"change a self-reference's object", `change $Parent (Code = 'B');`, []string{"Mod.Order.Code"}},
		{"change over a self-reference twice", `change $Grandparent (Code = 'B');`, []string{"Mod.Order.Code"}},
		{"find in a self reference set", `$F = find $Related by Code = 'A';`, []string{"Mod.Order.Code"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, a := storedOutputsFixture(t)
			got, err := fragmentMembers(t, ctx, a, c.src)
			if err != nil {
				t.Fatalf("%s: %v", c.src, err)
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("%s wrote %v, want %v", c.src, got, c.want)
			}
		})
	}
}

// TestSpliceFragment_MembersOfConflictingOutputsAreRefused: a name two stored
// origins give different types stays untyped, and a member of it is refused
// rather than qualified with a guessed entity — as is one read from it.
// Control: the same member of a name two origins give one type is written.
func TestSpliceFragment_MembersOfConflictingOutputsAreRefused(t *testing.T) {
	for _, src := range []string{
		`change $Dup (Code = 'B');`,
		`change $Mixed (Code = 'B');`,
		`change $DupFirst (Code = 'B');`,
		`change $Lost (Code = 'B');`,
	} {
		ctx, a := storedOutputsFixture(t)
		if _, err := fragmentMembers(t, ctx, a, src); err == nil || !strings.Contains(err.Error(), "cannot qualify member") {
			t.Errorf("%s: want a refusal, got %v", src, err)
		}
	}
	ctx, a := storedOutputsFixture(t)
	got, err := fragmentMembers(t, ctx, a, `change $Pick (Code = 'B');`)
	if err != nil || strings.Join(got, ",") != "Mod.OrderLine.Code" {
		t.Errorf("change $Pick: got %v, %v; want [Mod.OrderLine.Code]", got, err)
	}
}
