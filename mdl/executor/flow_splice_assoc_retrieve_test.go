// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// ako/mxcli#1769 (mendixlabs/mxcli#1673): a fragment spliced into a stored
// flow is built knowing the variables the stored flow declares
// (storedVariables), and a retrieve over an association was left untyped
// there, though a database retrieve and a list parameter were typed. A find by
// member in a list retrieved over an association could not be qualified, so
// the fragment refused it — or, in a loop the fragment brought, wrote it as a
// find by the invalid expression `Code = …` (CE0117).

// orderFixture is the reported domain model: OrderLine_Order runs FROM
// OrderLine TO Order (ParentPointer = OrderLine, ChildPointer = Order), so a
// retrieve over it from an Order yields the list of its lines and one from an
// OrderLine yields its one order. Order_Tag is a reference set FROM Order TO
// Tag; Order_Invoice a one-to-one (owner Both) reference FROM Order TO
// Invoice; Order_Parent a reference from Order to itself.
func orderFixture(t *testing.T) (*ExecContext, *alterFlowContext) {
	t.Helper()
	mod := mkModule("Mod")
	attr := func(name string) *domainmodel.Attribute {
		return &domainmodel.Attribute{BaseElement: model.BaseElement{ID: nextID("attr")}, Name: name}
	}
	order := mkEntity(mod.ID, "Order")
	order.Attributes = []*domainmodel.Attribute{attr("Code")}
	line := mkEntity(mod.ID, "OrderLine")
	line.Attributes = []*domainmodel.Attribute{attr("Code"), attr("Status")}
	lookup := mkEntity(mod.ID, "Lookup")
	lookup.Attributes = []*domainmodel.Attribute{attr("Code")}
	tag := mkEntity(mod.ID, "Tag")
	tag.Attributes = []*domainmodel.Attribute{attr("Label")}
	invoice := mkEntity(mod.ID, "Invoice")
	invoice.Attributes = []*domainmodel.Attribute{attr("Number")}
	dm := mkDomainModel(mod.ID, order, line, lookup, tag, invoice)
	tags := mkAssociation(dm.ID, "Order_Tag", order.ID, tag.ID)
	tags.Type = domainmodel.AssociationTypeReferenceSet
	oneToOne := mkAssociation(dm.ID, "Order_Invoice", order.ID, invoice.ID)
	oneToOne.Owner = domainmodel.AssociationOwnerBoth
	dm.Associations = []*domainmodel.Association{
		mkAssociation(dm.ID, "OrderLine_Order", line.ID, order.ID),
		tags,
		oneToOne,
		mkAssociation(dm.ID, "Order_Parent", order.ID, order.ID),
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
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))

	obj := func(e string) microflows.DataType { return &microflows.ObjectType{EntityQualifiedName: "Mod." + e} }
	over := func(out, start, assoc string) mfmutator.Candidate {
		return mfmutator.Candidate{OutputVariable: out, Object: &microflows.ActionActivity{Action: &microflows.RetrieveAction{
			Source: &microflows.AssociationRetrieveSource{StartVariable: start, AssociationQualifiedName: "Mod." + assoc}}}}
	}
	loop := func(iterator, list string) mfmutator.Candidate {
		return mfmutator.Candidate{Object: &microflows.LoopedActivity{
			LoopSource: &microflows.IterableList{VariableName: iterator, ListVariableName: list}}}
	}
	a := &alterFlowContext{
		stmt: &ast.AlterFlowStmt{},
		mf: &microflows.Microflow{Parameters: []*microflows.MicroflowParameter{
			{Name: "Order", Type: obj("Order")},
			{Name: "Line", Type: obj("OrderLine")},
			{Name: "Tag", Type: obj("Tag")},
			{Name: "Invoice", Type: obj("Invoice")},
			{Name: "Lookups", Type: &microflows.ListType{EntityQualifiedName: "Mod.Lookup"}},
		}},
		entityNames: map[model.ID]string{order.ID: "Mod.Order", line.ID: "Mod.OrderLine"},
		cands: []mfmutator.Candidate{
			over("Lines", "Order", "OrderLine_Order"),        // from the TO end: the lines
			over("TheOrder", "Line", "OrderLine_Order"),      // from the FROM end: the order
			over("Tags", "Order", "Order_Tag"),               // reference set, forward
			over("TagOrders", "Tag", "Order_Tag"),            // reference set, reverse
			over("TheInvoice", "Order", "Order_Invoice"),     // one-to-one, forward
			over("InvoiceOrder", "Invoice", "Order_Invoice"), // one-to-one, reverse
			// Starts from a variable a later candidate types: the order of the
			// candidates is not the order a variable is typed in.
			over("SiblingLines", "LineOrder", "OrderLine_Order"),
			over("LineOrder", "Line", "OrderLine_Order"),
			over("Parent", "Order", "Order_Parent"),   // self-reference: from the FROM end (#1777)
			over("Nowhere", "Order", "Order_Missing"), // no such association
			over("Unstarted", "NoSuchVariable", "OrderLine_Order"),
			// A loop's iterator is an element of its list, and a retrieve in
			// the body can start from it.
			loop("EachLine", "Lines"),
			over("EachOrder", "EachLine", "OrderLine_Order"),
			// Two loops whose iterators share a name over different lists:
			// which one a fragment means depends on where it goes.
			loop("It", "Lines"),
			loop("It", "Tags"),
			over("ItOrder", "It", "OrderLine_Order"),
		},
	}
	return ctx, a
}

// TestStoredVariables_AssociationRetrieveIsTyped: the output of a stored
// retrieve over an association has the type the association gives it from
// the end it starts at — a list of the FROM entity from the TO end, the one
// TO object from the FROM end, a list either way over a reference set, one
// object either way over a one-to-one, and over a self-reference what the FROM
// end holds, which is how Mendix reads it (ako/mxcli#1777). What cannot be
// established stays untyped, so a member of it is refused, not guessed.
func TestStoredVariables_AssociationRetrieveIsTyped(t *testing.T) {
	ctx, a := orderFixture(t)
	varTypes, declared := a.storedVariables(ctx)
	want := map[string]string{
		"Lines":        "List of Mod.OrderLine",
		"TheOrder":     "Mod.Order",
		"Tags":         "List of Mod.Tag",
		"TagOrders":    "List of Mod.Order",
		"TheInvoice":   "Mod.Invoice",
		"InvoiceOrder": "Mod.Order",
		"SiblingLines": "List of Mod.OrderLine",
		"LineOrder":    "Mod.Order",
		"EachLine":     "Mod.OrderLine",
		"EachOrder":    "Mod.Order",
		"Parent":       "Mod.Order",
	}
	for v, w := range want {
		if got := varTypes[v]; got != w {
			t.Errorf("$%s: typed %q, want %q", v, got, w)
		}
	}
	for _, v := range []string{"Nowhere", "Unstarted", "ItOrder"} {
		if got, ok := varTypes[v]; ok {
			t.Errorf("$%s: typed %q, want it left untyped", v, got)
		}
		if declared[v] != "Unknown" {
			t.Errorf("$%s: declared %q, want Unknown", v, declared[v])
		}
	}
	if got, ok := varTypes["It"]; ok {
		t.Errorf("$It, the iterator of two loops over different lists: typed %q, want it left untyped", got)
	}
}

// TestSpliceFragment_ReplacedLoopFindsOverAssociationList is the reported
// alter: a replaced loop whose body finds in a list retrieved over an
// association before the loop is written as a find by attribute, the way
// `create microflow` writes it — not as a find by the expression `Code = …`.
func TestSpliceFragment_ReplacedLoopFindsOverAssociationList(t *testing.T) {
	ctx, a := orderFixture(t)
	frag, err := a.buildFragment(ctx, parsedFlowBody(t, `mdl 1;
create microflow Mod.Fragment ()
begin
  loop $Lookup in $Lookups
  begin
    $Line = find $Lines by Code = $Lookup/Code;
    change $TheOrder (Code = $Lookup/Code);
  end loop;
end;`))
	if err != nil {
		t.Fatalf("the replaced loop: %v", err)
	}
	ops := fragmentListOperations(frag.Objects)
	if len(ops) != 1 {
		t.Fatalf("want one list operation, got %d", len(ops))
	}
	f, ok := ops[0].(*microflows.FindByAttributeOperation)
	if !ok || f.Attribute != "Mod.OrderLine.Code" || f.Expression != "$Lookup/Code" {
		t.Fatalf("the find was written as %#v, want a find by attribute Mod.OrderLine.Code = $Lookup/Code", ops[0])
	}
	var changed []string
	for _, obj := range frag.Objects {
		loop, ok := obj.(*microflows.LoopedActivity)
		if !ok {
			continue
		}
		for _, inner := range loop.ObjectCollection.Objects {
			if act, ok := inner.(*microflows.ActionActivity); ok {
				if ch, ok := act.Action.(*microflows.ChangeObjectAction); ok {
					for _, m := range ch.Changes {
						changed = append(changed, m.AttributeQualifiedName)
					}
				}
			}
		}
	}
	if strings.Join(changed, ",") != "Mod.Order.Code" {
		t.Errorf("the change over the forward retrieve wrote %v, want [Mod.Order.Code]", changed)
	}
}
