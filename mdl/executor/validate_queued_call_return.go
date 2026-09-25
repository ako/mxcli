// SPDX-License-Identifier: Apache-2.0

// CE7033: "A microflow used for background execution must have a Microflow
// return type of 'Nothing'."
//
// `CALL MICROFLOW M.F(…) IN QUEUE M.Q` runs F in the background, and Mendix
// refuses the call unless F returns nothing. Both halves are in the model — the
// binding mxcli writes and the signature of the flow it names — and nothing
// compared them, so `mxcli check --references` said "Check passed!" and the build
// failed (mendixlabs/mxcli#1064). It is the queued-call sibling of MDL073's
// after-startup check (CE0142): the reference RESOLVES, and the constraint is on
// the thing it names.
//
// Only CALL MICROFLOW is checked. A queued CALL JAVA ACTION has its own rule
// (CE7038, documented in the queues skill) and is not part of this report.
package executor

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// queuedMicroflowCall is one `CALL MICROFLOW … IN QUEUE …` found in a body.
type queuedMicroflowCall struct {
	target string // qualified name of the called microflow
	queue  string // qualified name of the queue
}

// queuedMicroflowCalls returns every queued microflow call under root, found by
// walking the whole statement tree — calls nest inside IF / LOOP / WHILE / error
// handlers, and a hand-written switch over statement kinds silently misses
// whichever nesting was added last (the same reason authoredQueueTargets walks
// by reflection).
func queuedMicroflowCalls(root any) []queuedMicroflowCall {
	var out []queuedMicroflowCall
	var walk func(v reflect.Value)
	seen := map[uintptr]bool{}
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Ptr, reflect.Interface:
			if v.IsNil() {
				return
			}
			if v.Kind() == reflect.Ptr {
				if seen[v.Pointer()] {
					return
				}
				seen[v.Pointer()] = true
				if s, ok := v.Interface().(*ast.CallMicroflowStmt); ok && s.Queue != nil && s.MicroflowName.Module != "" {
					out = append(out, queuedMicroflowCall{
						target: s.MicroflowName.String(),
						queue:  s.Queue.Module + "." + s.Queue.Name,
					})
				}
			}
			walk(v.Elem())
		case reflect.Slice, reflect.Array:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).PkgPath != "" {
					continue // unexported
				}
				walk(v.Field(i))
			}
		}
	}
	walk(reflect.ValueOf(root))
	return out
}

// checkQueuedMicroflowReturnsNothing reports a queued call whose target returns
// a value. returnType is the target's return type name as the backend or the
// script declares it; "" and "Void" both mean Nothing.
func checkQueuedMicroflowReturnsNothing(call queuedMicroflowCall, returnType string) error {
	if returnType == "" || strings.EqualFold(returnType, "Void") {
		return nil
	}
	return mdlerrors.NewValidationf(
		"microflow %s is called in queue %s but returns %s — a microflow run in the background must "+
			"return nothing, and the build reports CE7033 \"A microflow used for background execution "+
			"must have a Microflow return type of 'Nothing'.\"",
		call.target, call.queue, returnType)
}

// validateQueuedMicroflowTargets is the --references half: queued calls whose
// target is already stored in the project, typed by returnTypes (from
// buildMicroflowReturnTypes).
//
// A target the script itself (re)creates is skipped — its declared signature
// is what will be written, and ValidateQueuedCallReturnType reports it without
// a project; checking it here too would print the same fault twice. A target
// whose return type is unavailable is left alone: nothing is KNOWN to be wrong,
// and a refusal would block a script that builds.
func validateQueuedMicroflowTargets(body []ast.MicroflowStatement, returnTypes map[string]string, sc *scriptContext) []string {
	var errs []string
	for _, call := range queuedMicroflowCalls(body) {
		if sc != nil && sc.microflows[call.target] {
			continue
		}
		ret, ok := returnTypes[call.target]
		if !ok {
			continue
		}
		if err := checkQueuedMicroflowReturnsNothing(call, ret); err != nil {
			errs = append(errs, err.Error())
		}
	}
	return errs
}

// ValidateQueuedCallReturnType (MDL088) is the project-less half: a queued call
// whose target the same script creates. That is the common shape — write the
// worker microflow, then enqueue it — so the answer is in the script and
// `mxcli check` with no project can give it.
func ValidateQueuedCallReturnType(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}

	returnTypes := map[string]string{}
	for _, stmt := range prog.Statements {
		mf, ok := stmt.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		if mf.ReturnType == nil || mf.ReturnType.Type.Kind == ast.TypeVoid {
			returnTypes[mf.Name.String()] = ""
			continue
		}
		if kind := mf.ReturnType.Type.Kind.String(); kind != "Unknown" {
			returnTypes[mf.Name.String()] = kind
		}
	}
	if len(returnTypes) == 0 {
		return nil
	}

	var out []linter.Violation
	for _, stmt := range prog.Statements {
		mf, ok := stmt.(*ast.CreateMicroflowStmt)
		if !ok {
			continue
		}
		for _, call := range queuedMicroflowCalls(mf.Body) {
			// Only a microflow this script defines: anything else needs the
			// project, and reporting it from here would guess.
			ret, defined := returnTypes[call.target]
			if !defined {
				continue
			}
			if err := checkQueuedMicroflowReturnsNothing(call, ret); err != nil {
				out = append(out, linter.Violation{
					RuleID:   "MDL088",
					Severity: linter.SeverityError,
					Message:  fmt.Sprintf("microflow %s: %s", mf.Name.String(), err.Error()),
					Suggestion: "Drop the `returns` clause from " + call.target +
						" (and its `return` value), or call it without `in queue`",
				})
			}
		}
	}
	return out
}
