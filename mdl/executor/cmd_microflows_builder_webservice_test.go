// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"encoding/base64"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdltypes "github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func TestBuildFlowGraph_WebServiceCallCreatesRealAction(t *testing.T) {
	moduleID := mkID("soap-module")
	serviceID := mkID("soap-service")
	sendMappingID := mkID("soap-send")
	receiveMappingID := mkID("soap-receive")
	serviceContents, err := bson.Marshal(bson.M{"Name": "OrderService"})
	if err != nil {
		t.Fatal(err)
	}
	backend := &mock.MockBackend{
		ListRawUnitsByTypeFunc: func(typePrefix string) ([]*mdltypes.RawUnit, error) {
			return []*mdltypes.RawUnit{{
				ID:          serviceID,
				ContainerID: moduleID,
				Type:        typePrefix,
				Contents:    serviceContents,
			}}, nil
		},
		GetExportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ExportMapping, error) {
			return &model.ExportMapping{
				BaseElement: model.BaseElement{ID: sendMappingID},
				ContainerID: moduleID,
				Name:        name,
			}, nil
		},
		GetImportMappingByQualifiedNameFunc: func(moduleName, name string) (*model.ImportMapping, error) {
			return &model.ImportMapping{
				BaseElement: model.BaseElement{ID: receiveMappingID},
				ContainerID: moduleID,
				Name:        name,
			}, nil
		},
	}
	h := mkHierarchy(&model.Module{BaseElement: model.BaseElement{ID: moduleID}, Name: "SampleSOAP"})
	fb := &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		declaredVars: map[string]string{},
		backend:      backend,
		hierarchy:    h,
	}

	oc := fb.buildFlowGraph([]ast.MicroflowStatement{&ast.CallWebServiceStmt{
		OutputVariable:   "Root",
		ServiceID:        "SampleSOAP.OrderService",
		OperationName:    "FetchOrders",
		SendMappingID:    "SampleSOAP.OrderRequest",
		ReceiveMappingID: "SampleSOAP.OrderResponse",
		Timeout:          &ast.LiteralExpr{Kind: ast.LiteralInteger, Value: 30},
	}}, nil)

	action := firstWebServiceCallAction(t, oc)
	if action.ServiceID != "SampleSOAP.OrderService" {
		t.Errorf("ServiceID = %q, want SampleSOAP.OrderService", action.ServiceID)
	}
	// The QUALIFIED NAMES, not the mappings' unit ids.
	//
	// This test used to assert the ids, locking in a defect: the stored
	// ImportMappingCall's ReturnValueMapping is an ImportMappingIdentifier, and a
	// UUID there makes the project impossible to LOAD — `mx check` stops with a
	// StorageLoadException before validation. Measured on 11.14.0 against
	// ako/TestApp. The mock resolving these names is what made the old
	// expectation reachable at all; the only SOAP fixture names mappings that do
	// not exist, so exec never took the resolving branch and nothing else noticed.
	if string(action.SendMappingID) != "SampleSOAP.OrderRequest" {
		t.Errorf("SendMappingID = %q, want the qualified name", action.SendMappingID)
	}
	if string(action.ReceiveMappingID) != "SampleSOAP.OrderResponse" {
		t.Errorf("ReceiveMappingID = %q, want the qualified name", action.ReceiveMappingID)
	}
	// Unused now that the ids must not reach the action, but kept wired into the
	// mock above: a future change that starts resolving them again fails here.
	_, _ = sendMappingID, receiveMappingID
	if action.OutputVariable != "Root" || !action.UseReturnVariable {
		t.Errorf("output = %q/%v", action.OutputVariable, action.UseReturnVariable)
	}
	if action.TimeoutExpression != "30" {
		t.Errorf("TimeoutExpression = %q, want 30", action.TimeoutExpression)
	}
}

func TestBuildFlowGraph_WebServiceCallPreservesRawBSON(t *testing.T) {
	raw, err := bson.Marshal(bson.D{
		{Key: "$ID", Value: "soap-action"},
		{Key: "$Type", Value: "Microflows$CallWebServiceAction"},
		{Key: "OperationName", Value: "FetchOrders"},
	})
	if err != nil {
		t.Fatal(err)
	}
	fb := &flowBuilder{posX: 100, posY: 100, spacing: HorizontalSpacing}
	oc := fb.buildFlowGraph([]ast.MicroflowStatement{&ast.CallWebServiceStmt{
		OutputVariable: "Root",
		RawBSONBase64:  base64.StdEncoding.EncodeToString(raw),
	}}, nil)

	action := firstWebServiceCallAction(t, oc)
	if string(action.RawBSON) != string(raw) {
		t.Fatalf("RawBSON was not preserved")
	}
}

func firstWebServiceCallAction(t *testing.T, oc *microflows.MicroflowObjectCollection) *microflows.WebServiceCallAction {
	t.Helper()
	for _, obj := range oc.Objects {
		activity, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		action, ok := activity.Action.(*microflows.WebServiceCallAction)
		if ok {
			return action
		}
	}
	t.Fatal("WebServiceCallAction not found")
	return nil
}
