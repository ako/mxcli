// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// TestCheckWebServiceRequestBody_RefusesBothForms is the regression test for the
// silent drop this rule replaces.
//
// A call stores ONE RequestBodyHandling. Before this, both clauses parsed, the
// writer ignored the send mapping, and mxbuild reported CE0369 "Cannot use
// simple request body, as the operation's body is complex" — an error naming a
// simple request body on a statement that asked for a mapping.
func TestCheckWebServiceRequestBody_RefusesBothForms(t *testing.T) {
	err := checkWebServiceRequestBodyStmt(&ast.CallWebServiceStmt{
		ServiceID:           "Clients.OrderSoapClient",
		OperationName:       "SaveOrder",
		Arguments:           []ast.CallArgument{{Name: "OrderId", Value: nil}},
		SendMappingID:       "Clients.SoapOrderExportMapping",
		SendMappingVariable: "Order",
	})
	if err == nil {
		t.Fatal("a call asking for both arguments and a send mapping was accepted")
	}
	for _, want := range []string{"EITHER", "operation SaveOrder", "send mapping"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message does not name %q: %v", want, err)
		}
	}
}

// TestCheckWebServiceRequestBody_SendMappingNeedsAVariable — an export mapping
// maps an OBJECT, and Mendix stores which one. mxcli cannot write the mapping
// without it, so the clause is refused rather than written incomplete.
func TestCheckWebServiceRequestBody_SendMappingNeedsAVariable(t *testing.T) {
	err := checkWebServiceRequestBodyStmt(&ast.CallWebServiceStmt{
		ServiceID:     "Clients.OrderSoapClient",
		OperationName: "SaveOrder",
		SendMappingID: "Clients.SoapOrderExportMapping",
	})
	if err == nil {
		t.Fatal("`send mapping X` with no source variable was accepted")
	}
	if !strings.Contains(err.Error(), "from $") {
		t.Errorf("message does not show the fix: %v", err)
	}
}

// TestCheckWebServiceRequestBody_ArgumentsNeedAnOperation — the parameter path
// is built from the operation's request body element, so arguments without an
// operation have nothing to bind to.
func TestCheckWebServiceRequestBody_ArgumentsNeedAnOperation(t *testing.T) {
	if err := checkWebServiceRequestBodyStmt(&ast.CallWebServiceStmt{
		ServiceID: "Clients.OrderSoapClient",
		Arguments: []ast.CallArgument{{Name: "OrderId"}},
	}); err == nil {
		t.Fatal("arguments without an operation were accepted")
	}
}

// TestCheckWebServiceRequestBody_Accepts covers every shape that is valid,
// including the ones that were valid before this rule existed — a rule that
// refuses working scripts is worse than the drop it replaces.
func TestCheckWebServiceRequestBody_Accepts(t *testing.T) {
	for _, tc := range []struct {
		name string
		stmt *ast.CallWebServiceStmt
	}{
		{"arguments only", &ast.CallWebServiceStmt{
			ServiceID: "M.S", OperationName: "Op",
			Arguments: []ast.CallArgument{{Name: "OrderId"}},
		}},
		{"send mapping with its variable", &ast.CallWebServiceStmt{
			ServiceID: "M.S", OperationName: "Op",
			SendMappingID: "M.Export", SendMappingVariable: "Order",
		}},
		{"neither — today's argument-less call", &ast.CallWebServiceStmt{
			ServiceID: "M.S", OperationName: "Op", ReceiveMappingID: "M.Import",
		}},
		// A raw payload re-emits verbatim, so its request body is whatever the
		// bytes say and no clause was parsed to contradict it.
		{"raw payload with clauses that would otherwise clash", &ast.CallWebServiceStmt{
			RawBSONBase64: "AQID",
			Arguments:     []ast.CallArgument{{Name: "OrderId"}},
			SendMappingID: "M.Export",
		}},
		{"nil statement", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := checkWebServiceRequestBodyStmt(tc.stmt); err != nil {
				t.Errorf("refused a valid call: %v", err)
			}
		})
	}
}
