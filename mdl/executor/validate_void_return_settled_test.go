// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

type fakeVoidReturns map[string]int

func (f fakeVoidReturns) VoidReturnValues(qn string) (int, bool) {
	n, ok := f[qn]
	return n, ok
}

const voidValueFlow = `create or modify microflow OIDC.SUB_HandlePrivateKey ($C: Boolean)
begin
  if $C then
    return OIDC.ENU_ClientAssertion.PRIVATE_KEY;
  else
    return;
  end if;
end;`

func voidValueViolations(t *testing.T) (*ast.Program, []linter.Violation) {
	t.Helper()
	prog, errs := visitor.Build(voidValueFlow)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	return prog, ValidateProgram(prog, "")
}

func severityOfVoidValue(vs []linter.Violation) (linter.Severity, bool) {
	for _, v := range vs {
		if v.RuleID == "MDL004" && strings.HasPrefix(v.Message, voidReturnValueMessage) {
			return v.Severity, true
		}
	}
	return 0, false
}

// A void microflow whose end event carries a value is what Evora Factory
// Management stores in OIDC.SUB_HandlePrivateKey and four other microflows —
// a leftover of an earlier return type, hidden by Studio Pro and built at 0
// errors by mxbuild 10.24.15 and 11.13.0 (measured, also for a fresh one written
// by exec). Re-applying the microflow's own describe output changes nothing,
// so for a flow already stored that way the refusal is a warning: the value is
// still reported, but exec no longer refuses an untouched flow.
func TestSettleStoredVoidReturnValues_StoredFlowIsAWarning(t *testing.T) {
	prog, vs := voidValueViolations(t)
	if sev, ok := severityOfVoidValue(vs); !ok || sev != linter.SeverityError {
		t.Fatalf("precondition: MDL004 void-value error expected, got %v (found=%v)", sev, ok)
	}
	got := SettleStoredVoidReturnValues(vs, prog, fakeVoidReturns{"OIDC.SUB_HandlePrivateKey": 1})
	if sev, ok := severityOfVoidValue(got); !ok || sev != linter.SeverityWarning {
		t.Fatalf("stored void flow with a return value: want a warning, got %v (found=%v)", sev, ok)
	}
}

// Controls: new code keeps the error — the runtime does return the value, and
// a workflow that branches on the call fails at instance start (write-workflows
// skill), so authoring it is still refused.
func TestSettleStoredVoidReturnValues_NewCodeStaysAnError(t *testing.T) {
	prog, vs := voidValueViolations(t)
	cases := map[string]voidReturnSource{
		"no project":                    nil,
		"flow not in the project":       fakeVoidReturns{},
		"stored flow has no such value": fakeVoidReturns{"OIDC.SUB_HandlePrivateKey": 0},
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			got := SettleStoredVoidReturnValues(vs, prog, src)
			if sev, ok := severityOfVoidValue(got); !ok || sev != linter.SeverityError {
				t.Fatalf("want the error kept, got %v (found=%v)", sev, ok)
			}
		})
	}
}

// The project reader counts valued end events of a stored void microflow and
// answers 0 for one that declares a return type.
func TestStoredVoidReturns_CountsValuedEndEvents(t *testing.T) {
	end := func(v string) *microflows.EndEvent { return &microflows.EndEvent{ReturnValue: v} }
	stored := map[string]*microflows.Microflow{
		"M.Void": {ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{end("M.E.A\n"), end(""), end("false")}}},
		"M.Typed": {ReturnType: &microflows.BooleanType{}, ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{end("true")}}},
	}
	b := &mock.MockBackend{
		GetRawUnitByNameFunc: func(_, name string) (*types.RawUnitInfo, error) {
			if _, ok := stored[name]; ok {
				return &types.RawUnitInfo{ID: name, Contents: []byte{1}}, nil
			}
			return nil, nil
		},
		ParseMicroflowBSONFunc: func(_ []byte, id, _ model.ID) (*microflows.Microflow, error) {
			return stored[string(id)], nil
		},
	}
	s := NewStoredVoidReturns(b)
	if n, ok := s.VoidReturnValues("M.Void"); !ok || n != 2 {
		t.Errorf("M.Void: got %d,%v want 2,true", n, ok)
	}
	if n, ok := s.VoidReturnValues("M.Typed"); !ok || n != 0 {
		t.Errorf("M.Typed: got %d,%v want 0,true", n, ok)
	}
	if _, ok := s.VoidReturnValues("M.Missing"); ok {
		t.Error("M.Missing reported as stored")
	}
}
