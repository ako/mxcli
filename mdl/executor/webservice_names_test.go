// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	mdltypes "github.com/mendixlabs/mxcli/mdl/types"
	"go.mongodb.org/mongo-driver/bson"
)

// importedServiceDoc builds a WebServices$ImportedServiceImpl the way the driver
// hands it back: sub-documents nested as maps once the unit is unmarshalled into
// map[string]any, and each collection prefixed with its typed-array marker.
func importedServiceDoc(t *testing.T, docName string, services ...bson.M) []byte {
	t.Helper()
	arr := bson.A{int32(2)}
	for _, s := range services {
		arr = append(arr, s)
	}
	out, err := bson.Marshal(bson.M{
		"$Type": importedServiceType,
		"Name":  docName,
		"Description": bson.M{
			"$Type":    "WebServices$WsdlDescriptionImpl",
			"Services": arr,
		},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

func serviceInfo(name string, operations ...string) bson.M {
	ops := bson.A{int32(2)}
	for _, op := range operations {
		ops = append(ops, bson.M{"$Type": "WebServices$OperationInfoImpl", "Name": op})
	}
	return bson.M{"$Type": "WebServices$ServiceInfoImpl", "Name": name, "Operations": ops}
}

func backendWithUnits(units ...[]byte) *mock.MockBackend {
	return &mock.MockBackend{
		ListRawUnitsByTypeFunc: func(typeName string) ([]*mdltypes.RawUnit, error) {
			out := make([]*mdltypes.RawUnit, 0, len(units))
			for _, u := range units {
				out = append(out, &mdltypes.RawUnit{Type: typeName, Contents: u})
			}
			return out, nil
		},
	}
}

// TestResolveWebServiceName is the regression test for CE0386.
//
// A SOAP call stores the WSDL service name, which is not the local part of the
// imported service's qualified name. Deriving it — what both engines did — made
// Mendix look for the operation inside a service that does not exist:
//
//	[CE0386] "Operation 'GetOrder' does not exist in consumed web service
//	          'Clients.OrderSoapClient'."
//
// measured on 11.14.0 against ako/TestApp, whose document is named
// OrderSoapClient and whose WSDL service is OrdersWS.
func TestResolveWebServiceName(t *testing.T) {
	b := backendWithUnits(importedServiceDoc(t, "OrderSoapClient",
		serviceInfo("OrdersWS", "GetOrder", "SaveOrder")))

	if got := resolveWebServiceName(b, "Clients.OrderSoapClient", "GetOrder"); got != "OrdersWS" {
		t.Errorf("resolveWebServiceName = %q, want OrdersWS (the WSDL service, not the document)", got)
	}
	// A single service answers even when the operation is not one of its own —
	// the operation only has to break ties.
	if got := resolveWebServiceName(b, "Clients.OrderSoapClient", "Unknown"); got != "OrdersWS" {
		t.Errorf("single-service lookup = %q, want OrdersWS", got)
	}
}

// TestResolveWebServiceName_PicksTheServiceDeclaringTheOperation — a WSDL may
// define several services, and Mendix resolves the operation within one of them.
func TestResolveWebServiceName_PicksTheServiceDeclaringTheOperation(t *testing.T) {
	b := backendWithUnits(importedServiceDoc(t, "MultiClient",
		serviceInfo("OrdersWS", "GetOrder"),
		serviceInfo("CustomersWS", "GetCustomer")))

	if got := resolveWebServiceName(b, "M.MultiClient", "GetCustomer"); got != "CustomersWS" {
		t.Errorf("= %q, want CustomersWS", got)
	}
	if got := resolveWebServiceName(b, "M.MultiClient", "GetOrder"); got != "OrdersWS" {
		t.Errorf("= %q, want OrdersWS", got)
	}
	// Two services and an operation neither declares: refuse rather than pick
	// one. Guessing here just reproduces CE0386 with a different name in it.
	if got := resolveWebServiceName(b, "M.MultiClient", "Nope"); got != "" {
		t.Errorf("= %q, want \"\" (ambiguous, must fall back)", got)
	}
}

// TestResolveWebServiceName_UnresolvableIsEmpty — every way the answer cannot be
// established returns "", so the writers fall back to the derivation that ships
// today. A wrong name is no worse than the current one; an invented one is.
func TestResolveWebServiceName_UnresolvableIsEmpty(t *testing.T) {
	doc := importedServiceDoc(t, "OrderSoapClient", serviceInfo("OrdersWS", "GetOrder"))

	for _, tc := range []struct {
		name string
		b    *mock.MockBackend
		qn   string
	}{
		{"no such document", backendWithUnits(doc), "Clients.SomethingElse"},
		{"no documents at all", backendWithUnits(), "Clients.OrderSoapClient"},
		{"empty qualified name", backendWithUnits(doc), ""},
		{"document declares no services", backendWithUnits(importedServiceDoc(t, "Bare")), "M.Bare"},
		// Two documents of the same bare name: the raw unit carries a container
		// id, not a module, so the module cannot be checked here. Refuse.
		{"ambiguous document name", backendWithUnits(doc, doc), "Clients.OrderSoapClient"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveWebServiceName(tc.b, tc.qn, "GetOrder"); got != "" {
				t.Errorf("= %q, want \"\"", got)
			}
		})
	}

	if got := resolveWebServiceName(nil, "Clients.OrderSoapClient", "GetOrder"); got != "" {
		t.Errorf("nil backend = %q, want \"\"", got)
	}
}

// TestResolveWebServiceName_ReadsMapDecodedDocuments pins the shape trap that
// cost a debugging round: a unit unmarshalled into map[string]any nests its
// sub-documents as maps, not bson.D. A lookup asserting only bson.D found
// nothing — and "found nothing" is indistinguishable here from "no such
// service", so it fell back to the wrong name instead of failing.
func TestResolveWebServiceName_ReadsMapDecodedDocuments(t *testing.T) {
	raw := importedServiceDoc(t, "OrderSoapClient", serviceInfo("OrdersWS", "GetOrder"))

	var asMap map[string]any
	if err := bson.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, isMap := asMap["Description"].(map[string]any); !isMap {
		t.Fatalf("Description decoded as %T; this test no longer pins the trap it was written for", asMap["Description"])
	}
	if got := serviceNameFromImportedService(raw, "GetOrder"); got != "OrdersWS" {
		t.Errorf("serviceNameFromImportedService = %q, want OrdersWS", got)
	}
}

// TestTypedArrayElements — a Mendix typed array leads with an int32 version
// marker, which is not an element.
func TestTypedArrayElements(t *testing.T) {
	if got := typedArrayElements(bson.A{int32(2), "a", "b"}); len(got) != 2 {
		t.Errorf("marker not dropped: %v", got)
	}
	if got := typedArrayElements(bson.A{int32(2)}); len(got) != 0 {
		t.Errorf("empty typed array = %v, want none", got)
	}
	if got := typedArrayElements([]any{int32(3), "a"}); len(got) != 1 {
		t.Errorf("[]any form not handled: %v", got)
	}
	if got := typedArrayElements("not an array"); got != nil {
		t.Errorf("non-array = %v, want nil", got)
	}
}
