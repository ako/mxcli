// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// A message definition is a selection over the domain model, so the executor's
// whole job is resolution. Two resolutions need the model and are the ones that
// can go silently wrong (ako/mxcli#272).

// mdFixture builds Order -> Customer (FROM Order, TO Customer), an OrderLine
// that inherits from a base, and the attributes each carries.
func mdFixture(t *testing.T) (*ExecContext, *model.MessageDefinitionCollection) {
	t.Helper()
	mod := &model.Module{Name: "Sales"}
	mod.ID = nextID("mod")

	mkAttr := func(name string, typ domainmodel.AttributeType) *domainmodel.Attribute {
		a := &domainmodel.Attribute{Name: name, Type: typ}
		a.ID = nextID("attr" + name)
		return a
	}
	base := &domainmodel.Entity{Name: "Base", Persistable: true}
	base.ID = nextID("base")
	base.Attributes = []*domainmodel.Attribute{mkAttr("Code", &domainmodel.StringAttributeType{})}

	order := &domainmodel.Entity{Name: "Order", Persistable: true, GeneralizationRef: "Sales.Base"}
	order.ID = nextID("order")
	order.Attributes = []*domainmodel.Attribute{
		mkAttr("OrderId", &domainmodel.LongAttributeType{}),
		mkAttr("Status", &domainmodel.EnumerationAttributeType{}),
		mkAttr("Total", &domainmodel.DecimalAttributeType{}),
	}
	customer := &domainmodel.Entity{Name: "Customer", Persistable: true}
	customer.ID = nextID("cust")
	customer.Attributes = []*domainmodel.Attribute{mkAttr("Name", &domainmodel.StringAttributeType{})}

	tag := &domainmodel.Entity{Name: "Tag", Persistable: true}
	tag.ID = nextID("tag")
	tag.Attributes = []*domainmodel.Attribute{mkAttr("Label", &domainmodel.StringAttributeType{})}

	// ParentID is the FROM entity (the FK owner); ChildID the TO entity.
	assoc := &domainmodel.Association{Name: "Order_Customer", ParentID: order.ID, ChildID: customer.ID,
		Type: domainmodel.AssociationTypeReference}
	assoc.ID = nextID("assoc")

	// The same shape as a set: one Order has many Tags, so BOTH directions are
	// lists. This is the association the corpus had no example of.
	tags := &domainmodel.Association{Name: "Order_Tag", ParentID: order.ID, ChildID: tag.ID,
		Type: domainmodel.AssociationTypeReferenceSet}
	tags.ID = nextID("tags")

	dm := &domainmodel.DomainModel{ContainerID: mod.ID,
		Entities:     []*domainmodel.Entity{base, order, customer, tag},
		Associations: []*domainmodel.Association{assoc, tags},
	}
	dm.ID = nextID("dm")
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)

	var written *model.MessageDefinitionCollection
	mb := &mock.MockBackend{
		IsConnectedFunc:                      func() bool { return true },
		ListModulesFunc:                      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:                  func(string) (*model.Module, error) { return mod, nil },
		ListDomainModelsFunc:                 func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:                   func(model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListMessageDefinitionCollectionsFunc: func() ([]*model.MessageDefinitionCollection, error) { return nil, nil },
		CreateMessageDefinitionCollectionFunc: func(c *model.MessageDefinitionCollection) error {
			written = c
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, written
}

func mdCreate(defs ...*ast.MessageDefinitionDef) *ast.CreateMessageDefinitionCollectionStmt {
	return &ast.CreateMessageDefinitionCollectionStmt{
		Name:        ast.QualifiedName{Module: "Sales", Name: "MD"},
		Definitions: defs,
	}
}

func attrMember(name string) *ast.MessageMemberDef { return &ast.MessageMemberDef{Attribute: name} }

func assocMember(assoc, entity string, kids ...*ast.MessageMemberDef) *ast.MessageMemberDef {
	return &ast.MessageMemberDef{
		Association: ast.QualifiedName{Module: "Sales", Name: assoc},
		Entity:      ast.QualifiedName{Module: "Sales", Name: entity},
		Members:     kids,
	}
}

// runCreate executes a CREATE and returns the collection the backend received.
func runCreate(t *testing.T, stmt *ast.CreateMessageDefinitionCollectionStmt) (*model.MessageDefinitionCollection, error) {
	t.Helper()
	var written *model.MessageDefinitionCollection
	ctx, _ := mdFixture(t)
	ctx.Backend.(*mock.MockBackend).CreateMessageDefinitionCollectionFunc =
		func(c *model.MessageDefinitionCollection) error { written = c; return nil }
	err := execCreateMessageDefinitionCollection(ctx, stmt)
	return written, err
}

// TestAssociationCardinalityFollowsTheDirection is the important one.
//
// MaxOccurs is not a function of the association's type — measured, all 927
// resolvable associations in the demo corpus are `Reference`, yet 526 store 1
// and 401 store -1. It tracks the direction of traversal, and getting it
// backwards exposes a list as a single object with NO build error behind it.
func TestAssociationCardinalityFollowsTheDirection(t *testing.T) {
	// Order is the FROM entity: reaching Customer follows the FK, so single.
	c, err := runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "OrderMsg",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Order"},
		Members: []*ast.MessageMemberDef{assocMember("Order_Customer", "Customer", attrMember("Name"))},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := c.Definitions[0].Root.Children[0].MaxOccurs; got != 1 {
		t.Errorf("Order -> Customer MaxOccurs = %d, want 1 (following the FK)", got)
	}

	// Customer is the TO entity: reaching Order is the reverse, so unbounded.
	c, err = runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "CustMsg",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Customer"},
		Members: []*ast.MessageMemberDef{assocMember("Order_Customer", "Order", attrMember("OrderId"))},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	child := c.Definitions[0].Root.Children[0]
	if child.MaxOccurs != -1 {
		t.Errorf("Customer -> Order MaxOccurs = %d, want -1 (the reverse)", child.MaxOccurs)
	}
	// ExposedItemName is set exactly when the element repeats — 461 of 461.
	if child.ExposedItemName != "Order" {
		t.Errorf("ExposedItemName = %q, want Order", child.ExposedItemName)
	}
}

// TestReferenceSetIsAListInBothDirections is the half the direction rule got
// wrong (ako/mxcli-rest FINDINGS #60).
//
// Direction alone is the right rule for a Reference and the wrong one for a
// ReferenceSet: a set is many in BOTH directions, so the forward traversal is a
// list too. The demo corpus the direction rule was measured on contains no
// ReferenceSet at all (927 of 927 are Reference), which is why it read as
// exceptionless.
//
// Unlike the Reference case, this one HAS a build error behind it: mxbuild
// reports CE6524 "The occurrence of '...' has changed" on the definition, and
// CE0295 on any object mapping element bound to it. Measured on
// ako/mxcli-rest's RestLab.RateSnapshot_ExchangeRate at 11.13.0, against a
// baseline of 0 errors.
func TestReferenceSetIsAListInBothDirections(t *testing.T) {
	// Forward: Order is the FROM entity, and reaching its Tags gives a list.
	c, err := runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "OrderMsg",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Order"},
		Members: []*ast.MessageMemberDef{assocMember("Order_Tag", "Tag", attrMember("Label"))},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	child := c.Definitions[0].Root.Children[0]
	if child.MaxOccurs != -1 {
		t.Errorf("Order -> Tag MaxOccurs = %d, want -1: a ReferenceSet is a list in the forward direction too", child.MaxOccurs)
	}
	// ExposedItemName is set exactly when the element repeats — 461 of 461 — so
	// the cardinality being wrong took the item name with it.
	if child.ExposedItemName != "Tag" {
		t.Errorf("ExposedItemName = %q, want Tag", child.ExposedItemName)
	}

	// CONTROL: the reverse was already right, and must stay right. If this were
	// the only assertion, a fix that returned -1 unconditionally would pass.
	c, err = runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "TagMsg",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Tag"},
		Members: []*ast.MessageMemberDef{assocMember("Order_Tag", "Order", attrMember("OrderId"))},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := c.Definitions[0].Root.Children[0].MaxOccurs; got != -1 {
		t.Errorf("Tag -> Order MaxOccurs = %d, want -1 (the reverse of a set)", got)
	}
}

// TestAssociationThatConnectsNeitherWayIsRefused pins the refusal. Defaulting
// to 1 would build, and be wrong.
func TestAssociationThatConnectsNeitherWayIsRefused(t *testing.T) {
	_, err := runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "Bad",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Base"},
		Members: []*ast.MessageMemberDef{assocMember("Order_Customer", "Customer")},
	}))
	if err == nil {
		t.Fatal("accepted an association that connects neither entity — the cardinality would be a guess")
	}
	if !strings.Contains(err.Error(), "does not connect") {
		t.Errorf("error should say the association does not connect the two: %v", err)
	}
}

// TestPrimitiveTypeIsMappedNotPassedThrough pins the three non-identity
// mappings. A pass-through stores Long and AutoNumber where Mendix stores
// Integer, and Enumeration where it stores String — 279 elements in the corpus,
// and the round trip against ako/TestApp caught it on a Long.
func TestPrimitiveTypeIsMappedNotPassedThrough(t *testing.T) {
	c, err := runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:   "M",
		Entity: ast.QualifiedName{Module: "Sales", Name: "Order"},
		Members: []*ast.MessageMemberDef{
			attrMember("OrderId"), attrMember("Status"), attrMember("Total"),
		},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	want := map[string]string{"OrderId": "Integer", "Status": "String", "Total": "Decimal"}
	for _, m := range c.Definitions[0].Root.Children {
		if got := m.PrimitiveType; got != want[m.OriginalName] {
			t.Errorf("%s PrimitiveType = %q, want %q", m.OriginalName, got, want[m.OriginalName])
		}
	}
}

// TestInheritedAttributeResolvesToItsDeclaringEntity pins the other
// model-dependent resolution. 398 of 3,697 exposed attributes in the corpus are
// inherited, and qualifying one against the entity that merely uses it is
// CE1613.
func TestInheritedAttributeResolvesToItsDeclaringEntity(t *testing.T) {
	c, err := runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "M",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Order"},
		Members: []*ast.MessageMemberDef{attrMember("Code")},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := c.Definitions[0].Root.Children[0].Attribute; got != "Sales.Base.Code" {
		t.Errorf("Attribute = %q, want Sales.Base.Code — Code is declared by Base, not Order", got)
	}
}

// TestUnknownAttributeNamesWhatExists pins the shape #882 established: a typo
// says what would have worked.
func TestUnknownAttributeNamesWhatExists(t *testing.T) {
	_, err := runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "M",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Customer"},
		Members: []*ast.MessageMemberDef{attrMember("Nope")},
	}))
	if err == nil {
		t.Fatal("accepted an attribute the entity does not have")
	}
	if !strings.Contains(err.Error(), "Name") {
		t.Errorf("error should list the attributes that exist: %v", err)
	}
}

// TestRootExposedNameDefaultsToTheEntityName pins that mxcli does not guess
// English. Studio Pro pluralises here; reproducing that needs -y -> -ies and an
// already-plural detector, so `as 'Orders'` says it instead — the same
// conclusion as array-item naming.
func TestRootExposedNameDefaultsToTheEntityName(t *testing.T) {
	c, err := runCreate(t, mdCreate(&ast.MessageDefinitionDef{
		Name:    "M",
		Entity:  ast.QualifiedName{Module: "Sales", Name: "Order"},
		Members: []*ast.MessageMemberDef{attrMember("OrderId")},
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	root := c.Definitions[0].Root
	if root.ExposedName != "Order" || root.ExposedItemName != "Order" {
		t.Errorf("root exposed=%q item=%q, want Order/Order", root.ExposedName, root.ExposedItemName)
	}
	if root.MaxOccurs != -1 {
		t.Errorf("root MaxOccurs = %d, want -1 — a definition root always repeats (56/56)", root.MaxOccurs)
	}
}
