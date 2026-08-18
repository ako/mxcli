// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// calcCtx wires a backend holding one module and the given microflows.
func calcCtx(t *testing.T, mfs []*microflows.Microflow) *ExecContext {
	t.Helper()
	const moduleID = model.ID("module-1")
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) {
			return []*model.Module{{
				BaseElement: model.BaseElement{ID: moduleID},
				Name:        "MyFirstModule",
			}}, nil
		},
		ListMicroflowsFunc: func() ([]*microflows.Microflow, error) { return mfs, nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	return ctx
}

func calcMicroflow(name string, ret microflows.DataType, params ...microflows.DataType) *microflows.Microflow {
	mf := &microflows.Microflow{
		BaseElement: model.BaseElement{ID: model.ID("mf-" + name)},
		ContainerID: model.ID("module-1"),
		Name:        name,
		ReturnType:  ret,
	}
	for i, p := range params {
		mf.Parameters = append(mf.Parameters, &microflows.MicroflowParameter{
			Name: string(rune('A' + i)),
			Type: p,
		})
	}
	return mf
}

func intAttr() ast.DataType { return ast.DataType{Kind: ast.TypeInteger} }

// TestResolveCalculatedValue_PassEntityFollowsSignature pins the two shapes
// mxbuild accepts (both measured at 0 errors on 11.13.0): a microflow taking
// the owning entity stores PassEntity=true, a parameterless one stores false.
// Hardcoding either produces a binding the build rejects.
func TestResolveCalculatedValue_PassEntityFollowsSignature(t *testing.T) {
	order := &microflows.ObjectType{EntityQualifiedName: "MyFirstModule.Order"}
	ctx := calcCtx(t, []*microflows.Microflow{
		calcMicroflow("CalcWithEntity", &microflows.IntegerType{}, order),
		calcMicroflow("CalcNoParam", &microflows.IntegerType{}),
	})

	qn := ast.QualifiedName{Module: "MyFirstModule", Name: "CalcWithEntity"}
	v, err := resolveCalculatedValue(ctx, &qn, "MyFirstModule.Order", "Total", intAttr())
	if err != nil {
		t.Fatalf("entity-parameter microflow rejected: %v", err)
	}
	if !v.PassEntity {
		t.Error("PassEntity = false for a microflow taking the owning entity")
	}

	qn = ast.QualifiedName{Module: "MyFirstModule", Name: "CalcNoParam"}
	v, err = resolveCalculatedValue(ctx, &qn, "MyFirstModule.Order", "Total", intAttr())
	if err != nil {
		t.Fatalf("parameterless microflow rejected, but mxbuild accepts it: %v", err)
	}
	if v.PassEntity {
		t.Error("PassEntity = true for a parameterless microflow")
	}
}

// TestResolveCalculatedValue_RefusesWrongEntityParameter — mxbuild reports this
// as CE7247 "Microflow parameter 'X' should be of type Module.Entity", so it is
// refused before the write rather than discovered at build time.
func TestResolveCalculatedValue_RefusesWrongEntityParameter(t *testing.T) {
	other := &microflows.ObjectType{EntityQualifiedName: "MyFirstModule.Other"}
	ctx := calcCtx(t, []*microflows.Microflow{
		calcMicroflow("CalcWrongEntity", &microflows.IntegerType{}, other),
	})

	qn := ast.QualifiedName{Module: "MyFirstModule", Name: "CalcWrongEntity"}
	_, err := resolveCalculatedValue(ctx, &qn, "MyFirstModule.Order", "Total", intAttr())
	if err == nil {
		t.Fatal("a microflow taking the wrong entity was accepted (mxbuild: CE7247)")
	}
	if !strings.Contains(err.Error(), "MyFirstModule.Order") {
		t.Errorf("error should name the entity the microflow must take, got: %v", err)
	}
}

// TestResolveCalculatedValue_RefusesWrongReturnType — mxbuild: CE7247
// "Microflow return type should be Integer/Long."
func TestResolveCalculatedValue_RefusesWrongReturnType(t *testing.T) {
	order := &microflows.ObjectType{EntityQualifiedName: "MyFirstModule.Order"}
	ctx := calcCtx(t, []*microflows.Microflow{
		calcMicroflow("CalcString", &microflows.StringType{}, order),
	})

	qn := ast.QualifiedName{Module: "MyFirstModule", Name: "CalcString"}
	if _, err := resolveCalculatedValue(ctx, &qn, "MyFirstModule.Order", "Total", intAttr()); err == nil {
		t.Fatal("a String-returning microflow was bound to an Integer attribute (mxbuild: CE7247)")
	}
}

// TestResolveCalculatedValue_IntegerLongAreOneFamily is the case that made the
// first version of this rule wrong: Mendix's own message is "should be
// Integer/Long", and a Long-returning microflow on an Integer attribute builds
// at 0 errors — so a strict equality check refuses valid MDL.
func TestResolveCalculatedValue_IntegerLongAreOneFamily(t *testing.T) {
	order := &microflows.ObjectType{EntityQualifiedName: "MyFirstModule.Order"}
	ctx := calcCtx(t, []*microflows.Microflow{
		calcMicroflow("CalcLong", &microflows.LongType{}, order),
	})

	qn := ast.QualifiedName{Module: "MyFirstModule", Name: "CalcLong"}
	if _, err := resolveCalculatedValue(ctx, &qn, "MyFirstModule.Order", "Total", intAttr()); err != nil {
		t.Fatalf("Long return on an Integer attribute refused, but mxbuild accepts it: %v", err)
	}
}

// TestResolveCalculatedValue_BareCalculatedIsUnbound — `CALCULATED` with no
// microflow is the "not yet wired" state Studio Pro also allows.
func TestResolveCalculatedValue_BareCalculatedIsUnbound(t *testing.T) {
	ctx := calcCtx(t, nil)
	v, err := resolveCalculatedValue(ctx, nil, "MyFirstModule.Order", "Total", intAttr())
	if err != nil {
		t.Fatalf("bare CALCULATED rejected: %v", err)
	}
	if v.Type != "CalculatedValue" || v.MicroflowName != "" {
		t.Errorf("bare CALCULATED should be an unbound CalculatedValue, got %+v", v)
	}
}
