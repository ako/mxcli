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

// importMappingDoc builds an ImportMappings$ImportMapping whose root object
// mapping element names an entity.
func importMappingDoc(t *testing.T, name, rootEntity string) []byte {
	t.Helper()
	elements := bson.A{int32(2)}
	if rootEntity != "" {
		elements = append(elements, bson.M{
			"$Type":  "ImportMappings$ObjectMappingElement",
			"Entity": rootEntity,
			// A value child, to prove the root is what is read rather than the
			// first element carrying any key at all.
			"Children": bson.A{int32(2), bson.M{
				"$Type":     "ImportMappings$ValueMappingElement",
				"Attribute": rootEntity + ".SomeAttr",
			}},
		})
	}
	out, err := bson.Marshal(bson.M{
		"$Type": importMappingType, "Name": name, "Elements": elements,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

// TestResolveImportMappingEntity is the regression test for CE0243/CE0366.
//
// The call's result VariableType is the entity the receive mapping produces.
// Both engines wrote DataTypes$VoidType — "returns nothing" — which contradicts
// the mapping and makes assigning the result an error of its own. Measured on
// 11.14.0 against ako/TestApp: writing Void gave
//
//	[CE0243] "The mapping used to return a value of type 'Nothing', but now
//	          returns a value of type 'Clients.Order'."
//	[CE0366] "Cannot store in variable when there is no return value."
//
// and both cleared once the entity was read off the mapping.
func TestResolveImportMappingEntity(t *testing.T) {
	b := backendWithUnits(importMappingDoc(t, "SoapOrdersImportMapping", "Clients.Order"))

	if got := resolveImportMappingEntity(b, "Clients.SoapOrdersImportMapping"); got != "Clients.Order" {
		t.Errorf("resolveImportMappingEntity = %q, want Clients.Order", got)
	}
}

// TestResolveImportMappingEntity_UnresolvableIsEmpty — every way the entity
// cannot be established returns "", and the writers then keep VoidType. That is
// still wrong, but it is what ships today: an unresolvable mapping must not be
// made worse, and must not be guessed at.
func TestResolveImportMappingEntity_UnresolvableIsEmpty(t *testing.T) {
	doc := importMappingDoc(t, "SoapOrdersImportMapping", "Clients.Order")

	for _, tc := range []struct {
		name string
		b    *mock.MockBackend
		qn   string
	}{
		{"no such mapping", backendWithUnits(doc), "Clients.Missing"},
		{"no mappings at all", backendWithUnits(), "Clients.SoapOrdersImportMapping"},
		{"empty name", backendWithUnits(doc), ""},
		{"mapping has no root entity", backendWithUnits(importMappingDoc(t, "Bare", "")), "M.Bare"},
		{"ambiguous name", backendWithUnits(doc, doc), "Clients.SoapOrdersImportMapping"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveImportMappingEntity(tc.b, tc.qn); got != "" {
				t.Errorf("= %q, want \"\"", got)
			}
		})
	}
	if got := resolveImportMappingEntity(nil, "Clients.SoapOrdersImportMapping"); got != "" {
		t.Errorf("nil backend = %q, want \"\"", got)
	}
}

// operationDoc builds an imported service whose single service declares
// operations with their RequestBodyElementName — the shape the ParameterPath
// derivation reads.
func operationDoc(t *testing.T, docName, serviceName string, ops map[string]string) []byte {
	t.Helper()
	opArr := bson.A{int32(2)}
	for name, element := range ops {
		opArr = append(opArr, bson.M{
			"$Type": "WebServices$OperationInfoImpl",
			"Name":  name, "RequestBodyElementName": element,
		})
	}
	out, err := bson.Marshal(bson.M{
		"$Type": importedServiceType, "Name": docName,
		"Description": bson.M{"Services": bson.A{int32(2), bson.M{
			"$Type": "WebServices$ServiceInfoImpl", "Name": serviceName, "Operations": opArr,
		}}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

// TestWebServiceParameterPath pins the escaping character for character.
//
// Measured on ako/TestApp (11.14.0): operation element
// "http://www.example.com/:GetOrder" and parameter "OrderId" are stored as
// "http%3A//www.example.com/:GetOrder|OrderId". Note what is NOT escaped — the
// slashes, and the colon BETWEEN namespace and local name — because a plausible
// wrong escaping is exactly what mxbuild accepts and Studio Pro does not.
func TestWebServiceParameterPath(t *testing.T) {
	got := webServiceParameterPath("http://www.example.com/:GetOrder", "OrderId")
	if want := "http%3A//www.example.com/:GetOrder|OrderId"; got != want {
		t.Errorf("webServiceParameterPath = %q, want %q", got, want)
	}
	// An element with no namespace at all keeps the bare local name.
	if got := webServiceParameterPath("GetOrder", "OrderId"); got != "GetOrder|OrderId" {
		t.Errorf("unqualified element = %q, want GetOrder|OrderId", got)
	}
	// A "|" inside a segment would otherwise be read as the separator.
	if got := webServiceParameterPath("urn:a|b:Op", "P"); got != "urn%3Aa%7Cb:Op|P" {
		t.Errorf("pipe not escaped: %q", got)
	}
}

// TestWebServiceParameterPath_RefusesUnverifiableEscaping — a segment already
// containing "%" is refused rather than encoded or passed through. Whether
// Mendix writes %25 there is unmeasured, and both answers produce a path that
// silently addresses the wrong parameter.
func TestWebServiceParameterPath_RefusesUnverifiableEscaping(t *testing.T) {
	for _, tc := range []struct{ element, name string }{
		{"http://x/%y:Op", "P"},
		{"http://x/:Op", "P%1"},
		{"", "P"},
		{"http://x/:Op", ""},
	} {
		if got := webServiceParameterPath(tc.element, tc.name); got != "" {
			t.Errorf("webServiceParameterPath(%q, %q) = %q, want \"\"", tc.element, tc.name, got)
		}
	}
}

// TestResolveWebServiceOperationElement reads the prefix every argument's path
// is built from, off the operation rather than out of the WSDL text.
func TestResolveWebServiceOperationElement(t *testing.T) {
	b := backendWithUnits(operationDoc(t, "OrderSoapClient", "OrdersWS", map[string]string{
		"GetOrder":  "http://www.example.com/:GetOrder",
		"SaveOrder": "http://www.example.com/:SaveOrder",
	}))

	if got := resolveWebServiceOperationElement(b, "Clients.OrderSoapClient", "GetOrder"); got != "http://www.example.com/:GetOrder" {
		t.Errorf("= %q", got)
	}
	// Unresolvable every way returns "", and the caller REFUSES rather than
	// falling back — there is no shipping value for a path never written.
	for _, tc := range []struct{ name, qn, op string }{
		{"no such operation", "Clients.OrderSoapClient", "Nope"},
		{"no such document", "Clients.Missing", "GetOrder"},
		{"no operation named", "Clients.OrderSoapClient", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveWebServiceOperationElement(b, tc.qn, tc.op); got != "" {
				t.Errorf("= %q, want \"\"", got)
			}
		})
	}
	if got := resolveWebServiceOperationElement(nil, "Clients.OrderSoapClient", "GetOrder"); got != "" {
		t.Errorf("nil backend = %q", got)
	}
}
