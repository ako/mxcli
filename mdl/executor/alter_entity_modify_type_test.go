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

// modifyTypeTestCtx builds an ExecContext over one entity with one Integer
// attribute, and reports what MODIFY ATTRIBUTE wrote back.
func modifyTypeTestCtx(t *testing.T) (*ExecContext, func() *domainmodel.Attribute) {
	t.Helper()
	mod := mkModule("App")
	ent := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: nextID("dm"),
		Name:        "OdooSession",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "UserID", Type: &domainmodel.IntegerAttributeType{}},
		},
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{ent},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, ent.ContainerID, dm.ID)

	var written *domainmodel.Attribute
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc: func(dmID model.ID, e *domainmodel.Entity) error {
			for _, a := range e.Attributes {
				if a.Name == "UserID" {
					written = a
				}
			}
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, func() *domainmodel.Attribute { return written }
}

// modifyToUnknownType runs MODIFY ATTRIBUTE against a type name that resolves to
// neither a primitive, an enumeration, nor an entity. The visitor maps any bare
// qualified name to TypeEnumeration (the TypeEnumeration/TypeEntity ambiguity),
// which is how a stray word reaches the executor as a type at all.
func modifyToUnknownType(ctx *ExecContext, typeName string) error {
	return execAlterEntity(ctx, &ast.AlterEntityStmt{
		Name:          ast.QualifiedName{Module: "App", Name: "OdooSession"},
		Operation:     ast.AlterEntityModifyAttribute,
		AttributeName: "UserID",
		DataType: ast.DataType{
			Kind:    ast.TypeEnumeration,
			EnumRef: &ast.QualifiedName{Name: typeName},
		},
	})
}

// TestModifyAttributeRejectsUnknownType is the regression test for
// mendixlabs/mxcli#910.
//
// `ALTER ENTITY … MODIFY ATTRIBUTE UserID SET DEFAULT NULL` parses as
// dataType=`SET` (the grammar's last dataType alternative is a bare
// qualifiedName, so any word is a candidate entity/enum reference) with
// `DEFAULT NULL` as a constraint. The executor then wrote an EnumerationAttribute
// whose enumeration does not exist — and the resulting .mpr cannot be loaded at
// all: `mx check` dies with
//
//	System.ArgumentNullException: Value cannot be null. (Parameter 'value')
//	  at EnumerationAttributeType.set_EnumerationId
//
// CREATE ENTITY has rejected exactly this since #552; MODIFY ATTRIBUTE never got
// the same guard, which is the whole bug.
func TestModifyAttributeRejectsUnknownType(t *testing.T) {
	ctx, written := modifyTypeTestCtx(t)

	err := modifyToUnknownType(ctx, "set")
	if err == nil {
		t.Fatal("MODIFY ATTRIBUTE accepted an unknown type — this writes an unloadable .mpr (#910)")
	}
	if !strings.Contains(err.Error(), "set") {
		t.Errorf("error %q does not name the offending type", err)
	}
	if w := written(); w != nil {
		t.Errorf("the attribute was written despite the error: type is now %v", w.Type)
	}
}

// TestModifyAttributeRejectsTypoedType covers the same defect reached by an
// ordinary typo rather than the SET DEFAULT NULL shape. It is the more likely
// way to hit it, and it corrupts the project identically.
func TestModifyAttributeRejectsTypoedType(t *testing.T) {
	ctx, written := modifyTypeTestCtx(t)

	if err := modifyToUnknownType(ctx, "Integr"); err == nil {
		t.Fatal("MODIFY ATTRIBUTE accepted a misspelled type name (#910)")
	}
	if w := written(); w != nil {
		t.Errorf("the attribute was written despite the error: type is now %v", w.Type)
	}
}

// TestModifyAttributeErrorNamesTheAlternatives keeps the refusal actionable: a
// user who typed SET DEFAULT NULL needs to be told what to type instead, or the
// error just moves the confusion.
func TestModifyAttributeErrorNamesTheAlternatives(t *testing.T) {
	ctx, _ := modifyTypeTestCtx(t)
	err := modifyToUnknownType(ctx, "set")
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "DROP DEFAULT") {
		t.Errorf("error %q does not point at DROP DEFAULT, the way to clear a default", err)
	}
}

// TestModifyAttributeAcceptsAPrimitive guards against the fix over-reaching: an
// ordinary retype must still work.
func TestModifyAttributeAcceptsAPrimitive(t *testing.T) {
	ctx, written := modifyTypeTestCtx(t)

	err := execAlterEntity(ctx, &ast.AlterEntityStmt{
		Name:          ast.QualifiedName{Module: "App", Name: "OdooSession"},
		Operation:     ast.AlterEntityModifyAttribute,
		AttributeName: "UserID",
		DataType:      ast.DataType{Kind: ast.TypeString, Length: 100},
	})
	if err != nil {
		t.Fatalf("a legitimate retype was rejected: %v", err)
	}
	if written() == nil {
		t.Fatal("a legitimate retype was not written")
	}
}

// dropDefaultTestCtx builds a context over one attribute carrying the given
// value, and reports what was written back.
func dropDefaultTestCtx(t *testing.T, val *domainmodel.AttributeValue) (*ExecContext, func() *domainmodel.Attribute) {
	t.Helper()
	mod := mkModule("App")
	ent := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: nextID("dm"),
		Name:        "OdooSession",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "UserID", Type: &domainmodel.IntegerAttributeType{}, Value: val},
		},
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{ent},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, ent.ContainerID, dm.ID)

	var written *domainmodel.Attribute
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc: func(dmID model.ID, e *domainmodel.Entity) error {
			for _, a := range e.Attributes {
				if a.Name == "UserID" {
					written = a
				}
			}
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, func() *domainmodel.Attribute { return written }
}

func dropDefault(ctx *ExecContext, attr string) error {
	return execAlterEntity(ctx, &ast.AlterEntityStmt{
		Name:          ast.QualifiedName{Module: "App", Name: "OdooSession"},
		Operation:     ast.AlterEntityDropDefault,
		AttributeName: attr,
	})
}

// TestDropDefaultClearsTheValueAndKeepsTheType is the other half of #910: the
// reporter wanted to clear a default, and the only spelling that looked right
// destroyed the project. DROP DEFAULT does it without touching the type.
func TestDropDefaultClearsTheValueAndKeepsTheType(t *testing.T) {
	ctx, written := dropDefaultTestCtx(t, &domainmodel.AttributeValue{DefaultValue: "7"})

	if err := dropDefault(ctx, "UserID"); err != nil {
		t.Fatalf("DROP DEFAULT: %v", err)
	}
	w := written()
	if w == nil {
		t.Fatal("nothing was written")
	}
	if w.Value != nil {
		t.Errorf("the default survived: %+v", w.Value)
	}
	if _, ok := w.Type.(*domainmodel.IntegerAttributeType); !ok {
		t.Errorf("the attribute type changed to %T — DROP DEFAULT must not retype", w.Type)
	}
}

// TestDropDefaultRefusesACalculatedAttribute pins that a CalculatedValue is not
// treated as a default. Clearing it would quietly convert a calculated attribute
// into a plain stored one — a different operation, and a lossy one.
func TestDropDefaultRefusesACalculatedAttribute(t *testing.T) {
	ctx, written := dropDefaultTestCtx(t, &domainmodel.AttributeValue{Type: "CalculatedValue"})

	err := dropDefault(ctx, "UserID")
	if err == nil {
		t.Fatal("DROP DEFAULT silently cleared a calculated attribute's value")
	}
	if !strings.Contains(err.Error(), "calculated") {
		t.Errorf("error %q does not explain that the attribute is calculated", err)
	}
	if written() != nil {
		t.Error("the attribute was written despite the refusal")
	}
}

func TestDropDefaultOnMissingAttribute(t *testing.T) {
	ctx, _ := dropDefaultTestCtx(t, &domainmodel.AttributeValue{DefaultValue: "7"})
	if err := dropDefault(ctx, "NoSuchAttr"); err == nil {
		t.Fatal("DROP DEFAULT on a non-existent attribute succeeded")
	}
}

// TestDropDefaultOnAnAttributeWithoutOne is a no-op, not an error: clearing a
// default that is already absent is the state the user asked for.
func TestDropDefaultOnAnAttributeWithoutOne(t *testing.T) {
	ctx, written := dropDefaultTestCtx(t, nil)
	if err := dropDefault(ctx, "UserID"); err != nil {
		t.Fatalf("DROP DEFAULT on an attribute with no default: %v", err)
	}
	if w := written(); w == nil || w.Value != nil {
		t.Errorf("expected a written attribute with no value, got %+v", w)
	}
}
