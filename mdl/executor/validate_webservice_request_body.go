// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for a SOAP call's REQUEST BODY.
//
// `Microflows$CallWebServiceAction.RequestBodyHandling` is a polymorphic child
// and a call stores exactly ONE of them. The two a SOAP call uses are
// alternatives, not a pair:
//
//	operation X (a = …)        Microflows$SimpleRequestHandling
//	send mapping M from $v     Microflows$MappingRequestHandling
//
// The grammar admits both so this can name them. What it decides is decidable
// from the statement alone, so `mxcli check` reports it without a project —
// which is the point, because the alternative is what shipped before: both
// clauses accepted, neither written, and mxbuild reporting CE0369 several
// minutes later on a statement that mentions no "simple request body" at all.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// checkWebServiceRequestBodyStmt reports what is wrong with a SOAP call's
// request-body clauses, or nil.
//
// This is the single decision: the executor calls it before building the
// action, and the check pass calls it through the microflow validator, so a
// script cannot pass one and fail the other.
func checkWebServiceRequestBodyStmt(s *ast.CallWebServiceStmt) error {
	if s == nil || s.RawBSONBase64 != "" {
		// A raw payload re-emits verbatim; its request body is whatever the
		// bytes say, and no clause was parsed to contradict it.
		return nil
	}

	if len(s.Arguments) > 0 && s.SendMappingID != "" {
		return fmt.Errorf(
			"call web service %s: a call sends EITHER the operation's arguments OR an "+
				"export mapping, never both — Mendix stores one request body "+
				"(RequestBodyHandling), so one of the two would be silently dropped.\n"+
				"  Keep `operation %s (…)` for a simple body, or `send mapping %s from $var` "+
				"for a mapped one",
			s.ServiceID, s.OperationName, s.SendMappingID)
	}

	if len(s.Arguments) > 0 && s.OperationName == "" {
		return fmt.Errorf(
			"call web service %s: arguments were given without an operation — the "+
				"parameter path is built from the operation's request body element, so "+
				"there is nothing to bind them to.\n"+
				"  Add `operation <Name>` before the argument list",
			s.ServiceID)
	}

	if s.SendMappingID != "" && s.SendMappingVariable == "" {
		return fmt.Errorf(
			"call web service %s: `send mapping %s` has no source variable — an export "+
				"mapping maps an OBJECT, and Mendix stores which one "+
				"(MappingRequestHandling.MappingVariableName), so mxcli cannot write the "+
				"mapping without it.\n"+
				"  Write `send mapping %s from $YourVariable`",
			s.ServiceID, s.SendMappingID, s.SendMappingID)
	}

	for _, arg := range s.Arguments {
		if arg.Name == "" {
			return fmt.Errorf(
				"call web service %s: an argument has no parameter name — each one binds a "+
					"named parameter of operation %s, e.g. `(OrderId = $Id)`",
				s.ServiceID, s.OperationName)
		}
	}
	return nil
}

// checkWebServiceRequestBody surfaces checkWebServiceRequestBodyStmt as MDL-SOAP01.
func (v *microflowValidator) checkWebServiceRequestBody(stmt *ast.CallWebServiceStmt) {
	err := checkWebServiceRequestBodyStmt(stmt)
	if err == nil {
		return
	}
	v.addViolation("MDL-SOAP01", linter.SeverityError, err.Error(),
		"See `mxcli syntax soap` for the two request-body forms")
}
