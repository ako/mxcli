// SPDX-License-Identifier: Apache-2.0

// A call to a VOID microflow or nanoflow is the same case as #953's void
// Java/JavaScript action: Studio Pro keeps an output name on it (stored with
// UseReturnVariable=true), and that name declares no variable. Measured on
// mxbuild 10.24.15 and 11.13.0, fresh project, one microflow per case:
//
//	two calls to a void microflow, same output name     0 errors
//	void microflow call `$W = …`, then `declare $W`      0 errors
//	two calls to a void nanoflow, same output name      0 errors
//	void microflow call `$Z = …`, then reading `$Z`      CE0109 "Undefined variable 'Z'"
//	two calls to a String microflow, same output name   CE0111 (control)
//
// Evora Factory Management (Mendix 10.24, 0 errors under mx check) carries the
// shape in OIDC.webCallback, OIDC.SUB_HandleUserProvisioning and two Teamcenter
// microflows; `check` refused their own describe output with MDL063.
package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func mfCall(out, flow string) *ast.CallMicroflowStmt {
	return &ast.CallMicroflowStmt{OutputVariable: out, MicroflowName: qn("M", flow)}
}

func nfCall(out, flow string) *ast.CallNanoflowStmt {
	return &ast.CallNanoflowStmt{OutputVariable: out, NanoflowName: qn("M", flow)}
}

func flowDecls() []ast.Statement {
	str := &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeString}}
	return []ast.Statement{
		&ast.CreateMicroflowStmt{Name: qn("M", "MfVoid")},
		&ast.CreateMicroflowStmt{Name: qn("M", "MfVoidTyped"),
			ReturnType: &ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeVoid}}},
		&ast.CreateMicroflowStmt{Name: qn("M", "MfString"), ReturnType: str},
		&ast.CreateNanoflowStmt{Name: qn("M", "NfVoid")},
		&ast.CreateNanoflowStmt{Name: qn("M", "NfString"), ReturnType: str},
	}
}

func rulesHit(vs []linter.Violation, rule string) int {
	n := 0
	for _, v := range vs {
		if v.RuleID == rule {
			n++
		}
	}
	return n
}

func TestMDL063_VoidFlowCallOutputsAreNotDeclarations(t *testing.T) {
	str := ast.DataType{Kind: ast.TypeString}
	lit := &ast.LiteralExpr{Kind: ast.LiteralString, Value: "x"}
	cases := []struct {
		name string
		flow ast.Statement
		want int
	}{
		{"two void microflow calls, same name", &ast.CreateMicroflowStmt{Name: qn("M", "Caller"),
			Body: []ast.MicroflowStatement{mfCall("V", "MfVoid"), mfCall("V", "MfVoid")}}, 0},
		{"void microflow (explicit Void) twice", &ast.CreateMicroflowStmt{Name: qn("M", "Caller"),
			Body: []ast.MicroflowStatement{mfCall("V", "MfVoidTyped"), mfCall("V", "MfVoidTyped")}}, 0},
		{"void microflow call then declare", &ast.CreateMicroflowStmt{Name: qn("M", "Caller"),
			Body: []ast.MicroflowStatement{mfCall("W", "MfVoid"),
				&ast.DeclareStmt{Variable: "W", Type: str, InitialValue: lit}}}, 0},
		{"control: two String microflow calls", &ast.CreateMicroflowStmt{Name: qn("M", "Caller"),
			Body: []ast.MicroflowStatement{mfCall("Y", "MfString"), mfCall("Y", "MfString")}}, 1},
		{"two void nanoflow calls", &ast.CreateNanoflowStmt{Name: qn("M", "NCaller"),
			Body: []ast.MicroflowStatement{nfCall("N", "NfVoid"), nfCall("N", "NfVoid")}}, 0},
		{"control: two String nanoflow calls", &ast.CreateNanoflowStmt{Name: qn("M", "NCaller"),
			Body: []ast.MicroflowStatement{nfCall("N", "NfString"), nfCall("N", "NfString")}}, 1},
		{"control: unresolvable microflow still counts", &ast.CreateMicroflowStmt{Name: qn("M", "Caller"),
			Body: []ast.MicroflowStatement{mfCall("R", "Elsewhere"), mfCall("R", "Elsewhere")}}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := &ast.Program{Statements: append(flowDecls(), tc.flow)}
			if got := rulesHit(ValidateProgram(prog, ""), "MDL063"); got != tc.want {
				t.Fatalf("MDL063 count = %d, want %d", got, tc.want)
			}
		})
	}
}

// Reading a void call's output is the CE0109 the measurement found, so MDL093
// covers microflow calls as it covers Java/JavaScript ones.
func TestMDL093_ReadOfVoidMicroflowOutput(t *testing.T) {
	read := &ast.LogStmt{Level: ast.LogInfo, Node: &ast.LiteralExpr{Kind: ast.LiteralString, Value: "P"},
		Message: &ast.VariableExpr{Name: "Z"}}
	prog := &ast.Program{Statements: append(flowDecls(), &ast.CreateMicroflowStmt{Name: qn("M", "Caller"),
		Body: []ast.MicroflowStatement{mfCall("Z", "MfVoid"), read}})}
	if got := rulesHit(ValidateProgram(prog, ""), voidCallOutputRule); got != 1 {
		t.Fatalf("%s count = %d, want 1", voidCallOutputRule, got)
	}
	ctrl := &ast.Program{Statements: append(flowDecls(), &ast.CreateMicroflowStmt{Name: qn("M", "Caller"),
		Body: []ast.MicroflowStatement{mfCall("Z", "MfString"), read}})}
	if got := rulesHit(ValidateProgram(ctrl, ""), voidCallOutputRule); got != 0 {
		t.Fatalf("control: %s fired on a String microflow's output", voidCallOutputRule)
	}
}

// Stored flows are read from the project by qualified name.
func TestVoidCodeActions_ReadsStoredFlowReturnType(t *testing.T) {
	stored := map[string]microflows.DataType{
		"microflow:OIDC.ACT_ShowCusomExceptionMessage": nil,
		"microflow:M.Typed":                            &microflows.VoidType{},
		"microflow:M.Str":                              &microflows.StringType{},
		"nanoflow:M.NVoid":                             nil,
	}
	b := &mock.MockBackend{
		GetRawUnitByNameFunc: func(objectType, name string) (*types.RawUnitInfo, error) {
			if _, ok := stored[objectType+":"+name]; ok {
				return &types.RawUnitInfo{ID: objectType + ":" + name, Contents: []byte{1}}, nil
			}
			return nil, nil
		},
		ParseMicroflowBSONFunc: func(_ []byte, unitID, _ model.ID) (*microflows.Microflow, error) {
			return &microflows.Microflow{ReturnType: stored[string(unitID)]}, nil
		},
	}
	r := newVoidCodeActions(nil, func() backend.FullBackend { return b })
	check := func(s ast.MicroflowStatement, want bool, what string) {
		t.Helper()
		if got := r.callIsVoid(s); got != want {
			t.Errorf("%s: callIsVoid = %v, want %v", what, got, want)
		}
	}
	check(&ast.CallMicroflowStmt{OutputVariable: "V", MicroflowName: qn("OIDC", "ACT_ShowCusomExceptionMessage")}, true, "stored void microflow")
	check(mfCall("V", "Typed"), true, "stored VoidType microflow")
	check(mfCall("V", "Str"), false, "control: stored String microflow")
	check(mfCall("V", "Missing"), false, "control: a microflow the project does not have")
	check(nfCall("V", "NVoid"), true, "stored void nanoflow")
	check(nfCall("V", "Typed"), false, "control: a microflow name is not a nanoflow")

	// describe's duplicate-output header asks the same question of the stored action.
	act := &microflows.MicroflowCallAction{ResultVariableName: "Variable_1", UseReturnVariable: true,
		MicroflowCall: &microflows.MicroflowCall{Microflow: "OIDC.ACT_ShowCusomExceptionMessage"}}
	if !r.actionIsVoidCall(act) {
		t.Error("describe: stored call to a void microflow not recognised")
	}
}
