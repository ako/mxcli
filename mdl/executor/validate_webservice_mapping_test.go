// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// A mapping can be sourced from an imported web service (SOAP) — a fourth kind
// beside JSON structure, XML schema and message definition. MDL cannot spell it,
// so a CREATE OR REPLACE/MODIFY rebuilt the mapping without it and a working
// integration became an unbuildable one (ako/mxcli#365):
//
//	[error] [CE6896] "A mapping must have exactly one schema source."
//	[error] [CE0270] "No root element could be found in the schema."

func soapSource() model.WebServiceMappingSource {
	return model.WebServiceMappingSource{
		ImportedWebService: "Legacy.WS_Orders",
		ServiceName:        "OrderService",
		OperationName:      "GetOrder",
		RootElementName:    "GetOrderResponse",
	}
}

func TestWebServiceSourcedMappingIsRefused(t *testing.T) {
	err := checkNoWebServiceSource("import", "Legacy.IMM_Order", soapSource())
	if err == nil {
		t.Fatal("accepted a rewrite that would drop the web-service binding")
	}
	// The refusal must name what would have been lost, not merely that
	// something would: the source is the one thing the statement never mentions.
	for _, want := range []string{"Legacy.WS_Orders", "OrderService", "GetOrder", "CE6896"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestNonWebServiceMappingIsUntouched is the control. Every mapping mxcli can
// author has an empty source here, so a guard that fired on it would refuse
// every rewrite in the repo.
func TestNonWebServiceMappingIsUntouched(t *testing.T) {
	if err := checkNoWebServiceSource("import", "M.IMM_Json", model.WebServiceMappingSource{}); err != nil {
		t.Fatalf("refused an ordinary mapping: %v", err)
	}
}

// TestWebServiceSourceIsKeyedOnTheService pins the discriminator. ServiceName
// and OperationName only qualify WHICH part of a service the mapping covers;
// a mapping carrying them without a service is not SOAP-sourced, and refusing
// it would block rewrites on the strength of a leftover empty string.
func TestWebServiceSourceIsKeyedOnTheService(t *testing.T) {
	partial := model.WebServiceMappingSource{ServiceName: "OrderService", OperationName: "GetOrder"}
	if partial.IsSet() {
		t.Error("IsSet true without an imported web service")
	}
	if err := checkNoWebServiceSource("import", "M.IMM_X", partial); err != nil {
		t.Errorf("refused on qualifiers alone: %v", err)
	}
}

// TestWebServiceDetailDegradesGracefully pins that a partially-populated
// binding still produces a readable message rather than empty parentheses.
func TestWebServiceDetailDegradesGracefully(t *testing.T) {
	cases := []struct {
		name string
		src  model.WebServiceMappingSource
		want string
	}{
		{"both", soapSource(), " (service OrderService, operation GetOrder)"},
		{"operation only", model.WebServiceMappingSource{
			ImportedWebService: "L.WS", OperationName: "GetOrder"}, " (operation GetOrder)"},
		{"service only", model.WebServiceMappingSource{
			ImportedWebService: "L.WS", ServiceName: "OrderService"}, " (service OrderService)"},
		{"neither", model.WebServiceMappingSource{ImportedWebService: "L.WS"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := webServiceDetail(tc.src); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestExportMappingIsRefusedToo pins that both mapping kinds are covered. An
// export mapping has the same binding plus two SOAP-only properties
// (ParameterName, IsHeader), so leaving it out would drop more, not less.
func TestExportMappingIsRefusedToo(t *testing.T) {
	src := soapSource()
	src.ParameterName = "body"
	src.IsHeader = false

	err := checkNoWebServiceSource("export", "Legacy.EXM_Order", src)
	if err == nil {
		t.Fatal("accepted an export-mapping rewrite that would drop the binding")
	}
	if !strings.Contains(err.Error(), "export mapping Legacy.EXM_Order") {
		t.Errorf("error does not name the export mapping: %v", err)
	}
}
