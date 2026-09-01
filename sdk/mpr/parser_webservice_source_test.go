// SPDX-License-Identifier: Apache-2.0

package mpr

import "testing"

// A mapping's SOAP binding must survive the read, because that is the only way
// a rewrite can be refused rather than silently dropping it (ako/mxcli#365).
// Before this, model.ImportMapping carried three source fields and a comment
// saying "at most one is set" — the fourth was not in the type at all.

func TestParseWebServiceSourceReadsTheBinding(t *testing.T) {
	got := parseWebServiceSource(map[string]any{
		"ImportedWebService": "Legacy.WS_Orders",
		"ServiceName":        "OrderService",
		"OperationName":      "GetOrder",
		"RootElementName":    "GetOrderResponse",
		"ParameterName":      "body",
		"IsHeader":           true,
	})

	if !got.IsSet() {
		t.Fatal("IsSet false for a mapping with an imported web service")
	}
	if got.ImportedWebService != "Legacy.WS_Orders" {
		t.Errorf("ImportedWebService = %q", got.ImportedWebService)
	}
	if got.ServiceName != "OrderService" || got.OperationName != "GetOrder" {
		t.Errorf("service/operation = %q/%q", got.ServiceName, got.OperationName)
	}
	// RootElementName is stored under that key; the SDK calls it
	// xsdRootElementName, which is what makes it easy to bind wrongly.
	if got.RootElementName != "GetOrderResponse" {
		t.Errorf("RootElementName = %q", got.RootElementName)
	}
	// Export-only, and carried for the same reason as the rest.
	if got.ParameterName != "body" || !got.IsHeader {
		t.Errorf("ParameterName/IsHeader = %q/%v", got.ParameterName, got.IsHeader)
	}
}

// TestParseWebServiceSourceIsEmptyForAnOrdinaryMapping is the control: every
// mapping mxcli can author reaches this with none of the keys present, and must
// come back not-set or the guard would refuse every rewrite.
func TestParseWebServiceSourceIsEmptyForAnOrdinaryMapping(t *testing.T) {
	got := parseWebServiceSource(map[string]any{
		"Name":          "IMM_Order",
		"JsonStructure": "Shop.JSON_Order",
	})
	if got.IsSet() {
		t.Errorf("IsSet true for a JSON-sourced mapping: %+v", got)
	}
}
