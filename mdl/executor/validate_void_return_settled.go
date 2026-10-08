// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// voidReturnValueMessage is MDL004's report of `return <value>` in a microflow
// that declares no return type. SettleStoredVoidReturnValues matches on it.
const voidReturnValueMessage = "return has a value but microflow does not declare a return type"

// voidReturnSource is what SettleStoredVoidReturnValues asks the project.
type voidReturnSource interface {
	// VoidReturnValues reports how many end events of the stored microflow
	// carry a return value while the microflow returns Void; ok is false when
	// the project has no such microflow.
	VoidReturnValues(qualifiedName string) (n int, ok bool)
}

// SettleStoredVoidReturnValues downgrades MDL004's "return has a value but
// microflow does not declare a return type" to a warning for a microflow the
// project already stores that way.
//
// Studio Pro keeps an end event's return value when the microflow's return
// type is changed to Nothing, and hides the field; mxbuild 10.24.15 and
// 11.13.0 build it at 0 errors, and so does a fresh one exec writes (measured).
// Evora Factory Management stores five such microflows, so `exec` refused to
// re-apply their own `describe` output — a statement that changes nothing.
//
// The value is not inert, which is why new code keeps the error: the runtime
// returns it, and a workflow branching on the call fails at instance start
// (`Trying to compare VoidConditionValue$(”) to BooleanValue('true')`, see the
// write-workflows skill). For a stored flow the warning says the same thing
// without blocking a round trip; it stays an error when there is no project, the
// flow is new, or the script adds more valued returns than are stored.
func SettleStoredVoidReturnValues(violations []linter.Violation, prog *ast.Program, stored voidReturnSource) []linter.Violation {
	if stored == nil || prog == nil {
		return violations
	}
	reported := map[string]int{}
	for _, v := range violations {
		if isVoidReturnValue(v) {
			reported[v.Location.DocumentName]++
		}
	}
	settled := map[string]bool{}
	for flow, n := range reported {
		if have, ok := stored.VoidReturnValues(flow); ok && have >= n {
			settled[flow] = true
		}
	}
	if len(settled) == 0 {
		return violations
	}
	out := make([]linter.Violation, 0, len(violations))
	for _, v := range violations {
		if isVoidReturnValue(v) && settled[v.Location.DocumentName] {
			v.Severity = linter.SeverityWarning
			v.Message = voidReturnValueMessage + " — already stored this way (Studio Pro keeps an end " +
				"event's value after the return type is set to Nothing, and hides it; mxbuild accepts it). " +
				"The runtime still returns the value, so a workflow that branches on this microflow fails at " +
				"instance start"
		}
		out = append(out, v)
	}
	return out
}

func isVoidReturnValue(v linter.Violation) bool {
	return v.RuleID == "MDL004" && v.Message == voidReturnValueMessage && v.Location.DocumentType == "microflow"
}

// StoredVoidReturns answers voidReturnSource from a project.
type StoredVoidReturns struct{ b backend.FullBackend }

// NewStoredVoidReturns reads stored microflows through b.
func NewStoredVoidReturns(b backend.FullBackend) *StoredVoidReturns {
	return &StoredVoidReturns{b: b}
}

// VoidReturnValues implements voidReturnSource.
func (s *StoredVoidReturns) VoidReturnValues(qualifiedName string) (int, bool) {
	if s == nil || s.b == nil {
		return 0, false
	}
	raw, err := s.b.GetRawUnitByName("microflow", qualifiedName)
	if err != nil || raw == nil || len(raw.Contents) == 0 {
		return 0, false
	}
	mf, err := s.b.ParseMicroflowBSON(raw.Contents, model.ID(raw.ID), "")
	if err != nil || mf == nil {
		return 0, false
	}
	if mf.ReturnType != nil && mf.ReturnType.GetTypeName() != "Void" {
		return 0, true
	}
	n := 0
	if mf.ObjectCollection != nil {
		for _, obj := range mf.ObjectCollection.Objects {
			if end, ok := obj.(*microflows.EndEvent); ok && end.ReturnValue != "" {
				n++
			}
		}
	}
	return n, true
}
