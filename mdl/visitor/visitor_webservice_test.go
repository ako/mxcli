// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCallWebServiceStatement(t *testing.T) {
	stmt := firstStatement(t, `$Root = call web service SampleSOAP.OrderService
operation FetchSampleItems
send mapping SampleSOAP.OrderRequest
receive mapping SampleSOAP.OrderResponse
timeout 30
on error rollback;`)

	call, ok := stmt.(*ast.CallWebServiceStmt)
	if !ok {
		t.Fatalf("expected CallWebServiceStmt, got %T", stmt)
	}
	if call.OutputVariable != "Root" {
		t.Errorf("OutputVariable = %q, want Root", call.OutputVariable)
	}
	if call.ServiceID != "SampleSOAP.OrderService" {
		t.Errorf("ServiceID = %q", call.ServiceID)
	}
	if call.OperationName != "FetchSampleItems" {
		t.Errorf("OperationName = %q", call.OperationName)
	}
	if call.SendMappingID != "SampleSOAP.OrderRequest" {
		t.Errorf("SendMappingID = %q", call.SendMappingID)
	}
	if call.ReceiveMappingID != "SampleSOAP.OrderResponse" {
		t.Errorf("ReceiveMappingID = %q", call.ReceiveMappingID)
	}
	if call.Timeout == nil {
		t.Fatal("expected Timeout expression")
	}
	if call.ErrorHandling == nil || call.ErrorHandling.Type != ast.ErrorHandlingRollback {
		t.Fatalf("ErrorHandling = %#v, want rollback", call.ErrorHandling)
	}
}

func TestCallWebServiceStatementQuotedFallbackRefs(t *testing.T) {
	stmt := firstStatement(t, `$Root = call web service 'sample-service-id'
operation 'FetchSampleItems'
send mapping 'sample-send-mapping-id'
receive mapping 'sample-receive-mapping-id';`)

	call, ok := stmt.(*ast.CallWebServiceStmt)
	if !ok {
		t.Fatalf("expected CallWebServiceStmt, got %T", stmt)
	}
	if call.ServiceID != "sample-service-id" {
		t.Errorf("ServiceID = %q", call.ServiceID)
	}
	if call.OperationName != "FetchSampleItems" {
		t.Errorf("OperationName = %q", call.OperationName)
	}
	if call.SendMappingID != "sample-send-mapping-id" {
		t.Errorf("SendMappingID = %q", call.SendMappingID)
	}
	if call.ReceiveMappingID != "sample-receive-mapping-id" {
		t.Errorf("ReceiveMappingID = %q", call.ReceiveMappingID)
	}
}

func TestCallWebServiceRawStatement(t *testing.T) {
	stmt := firstStatement(t, `$Root = call web service raw 'AQID';`)

	call, ok := stmt.(*ast.CallWebServiceStmt)
	if !ok {
		t.Fatalf("expected CallWebServiceStmt, got %T", stmt)
	}
	if call.OutputVariable != "Root" {
		t.Errorf("OutputVariable = %q, want Root", call.OutputVariable)
	}
	if call.RawBSONBase64 != "AQID" {
		t.Errorf("RawBSONBase64 = %q, want AQID", call.RawBSONBase64)
	}
	if call.ServiceID != "" || call.OperationName != "" {
		t.Errorf("raw statement should not set structured refs: %#v", call)
	}
}

// TestCallWebServiceArguments — the operation's arguments use callArgumentList,
// the same `(Name = value)` form every other call statement in MDL uses, so the
// visitor reuses the same builder.
func TestCallWebServiceArguments(t *testing.T) {
	stmt := firstStatement(t, `$Order = call web service Clients.OrderSoapClient
operation GetOrder (OrderId = $Customer/OrderId, Verbose = true)
receive mapping Clients.SoapOrdersImportMapping;`)

	call := stmt.(*ast.CallWebServiceStmt)
	if call.OperationName != "GetOrder" {
		t.Fatalf("OperationName = %q", call.OperationName)
	}
	if len(call.Arguments) != 2 {
		t.Fatalf("read %d arguments, want 2: %#v", len(call.Arguments), call.Arguments)
	}
	if call.Arguments[0].Name != "OrderId" || call.Arguments[1].Name != "Verbose" {
		t.Errorf("argument names = %q, %q", call.Arguments[0].Name, call.Arguments[1].Name)
	}
	if call.Arguments[0].Value == nil {
		t.Error("argument expression not built")
	}
}

// TestCallWebServiceSendMappingVariable — `from $var` names the object the
// export mapping maps, which Mendix stores as MappingVariableName.
//
// The output variable and this one are both VARIABLE tokens in the same rule,
// so the visitor walks them positionally against EQUALS and FROM. The second
// case below is the one that breaks a naive "first VARIABLE is the output"
// reading: there is no output variable, so the FIRST variable in the statement
// is the send mapping's.
func TestCallWebServiceSendMappingVariable(t *testing.T) {
	withOutput := firstStatement(t, `$Ok = call web service Clients.OrderSoapClient
operation SaveOrder
send mapping Clients.SoapOrderExportMapping from $NewSaveOrder;`).(*ast.CallWebServiceStmt)
	if withOutput.OutputVariable != "Ok" || withOutput.SendMappingVariable != "NewSaveOrder" {
		t.Errorf("output = %q, send variable = %q", withOutput.OutputVariable, withOutput.SendMappingVariable)
	}

	noOutput := firstStatement(t, `call web service Clients.OrderSoapClient
operation SaveOrder
send mapping Clients.SoapOrderExportMapping from $NewSaveOrder;`).(*ast.CallWebServiceStmt)
	if noOutput.OutputVariable != "" {
		t.Errorf("OutputVariable = %q, want empty", noOutput.OutputVariable)
	}
	if noOutput.SendMappingVariable != "NewSaveOrder" {
		t.Errorf("SendMappingVariable = %q, want NewSaveOrder", noOutput.SendMappingVariable)
	}
}

// TestCallWebServiceWithoutNewClauses — every statement that parsed before still
// parses, with both new fields empty.
func TestCallWebServiceWithoutNewClauses(t *testing.T) {
	call := firstStatement(t, `$Root = call web service SampleSOAP.OrderService
operation FetchSampleItems
receive mapping SampleSOAP.OrderResponse;`).(*ast.CallWebServiceStmt)
	if len(call.Arguments) != 0 || call.SendMappingVariable != "" {
		t.Errorf("new fields set on an old-shape statement: %#v", call)
	}
}
