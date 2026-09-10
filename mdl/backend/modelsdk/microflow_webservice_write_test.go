// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// encodeMicroflowAction encodes a semantic microflow action through the codec
// engine's write path, the same way a real CREATE MICROFLOW does.
func encodeMicroflowAction(t *testing.T, a microflows.MicroflowAction) bsonv1.D {
	t.Helper()
	el := microflowActionToGen(a)
	if el == nil {
		t.Fatal("actionToGen returned nil — the activity would be written with NO action (CE0008)")
	}
	out, err := (&codec.Encoder{}).Encode(el)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	var doc bsonv1.D
	if err := bsonv1.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

func fullWebServiceCall() *microflows.WebServiceCallAction {
	return &microflows.WebServiceCallAction{
		BaseElement:       model.BaseElement{ID: "ws-1"},
		ErrorHandlingType: "Rollback",
		ServiceID:         "SampleSOAP.OrderService",
		OperationName:     "FetchSampleItems",
		ReceiveMappingID:  "SampleSOAP.OrderResponse",
		OutputVariable:    "Root",
		UseReturnVariable: true,
		TimeoutExpression: "30",
	}
}

// TestWebServiceCallAction_IsWritten is the regression test for the reported
// gap, and it is the one that fails without the fix.
//
// The codec engine READ this action but had no write case, so it fell through to
// `default: return nil` and the enclosing ActionActivity was serialized with no
// action at all. Measured on 11.13.0 before the fix: `mxcli exec` of
// 06b-soap-examples.mdl reported success on all three microflows and `mx check`
// then reported CE0008 "No action defined." plus two CE0109 "Undefined variable
// 'Root'." — the knock-on from the dropped action never binding the variable.
func TestWebServiceCallAction_IsWritten(t *testing.T) {
	if el := microflowActionToGen(fullWebServiceCall()); el == nil {
		t.Fatal("microflowActionToGen(*WebServiceCallAction) = nil; the activity would carry no action (CE0008)")
	}
}

// TestWebServiceCallAction_MatchesLegacyDocument pins the whole document against
// the shape the legacy serializer writes.
//
// Legacy is the reference for this test on purpose — the change it guards is
// "stop dropping the action", so reproducing what ships is what makes it safe.
// The values below were read off a real legacy-written project (`mxcli bson
// dump`, Mendix 11.13.0), not off the serializer's source.
//
// It is NOT a fidelity test. Studio Pro-authored SOAP documents exist
// (ako/TestApp, 11.14.0) and legacy diverges from them in five places this test
// therefore also pins as-is — ServiceName, ImportMappingCall.ContentType,
// Range.SingleObject, VariableType, and the send-mapping request handling. The
// header comment in microflow_webservice_write.go lists them. When those are
// fixed, these expectations change with them, and that is the point of writing
// down which reference each one came from.
func TestWebServiceCallAction_MatchesLegacyDocument(t *testing.T) {
	doc := encodeMicroflowAction(t, fullWebServiceCall())

	for _, want := range []struct {
		key string
		val any
	}{
		{"$Type", "Microflows$CallWebServiceAction"},
		{"ErrorHandlingType", "Rollback"},
		// Qualified for ImportedService, local-only for ServiceName. Mendix
		// stores both, and they are not the same string.
		{"ImportedService", "SampleSOAP.OrderService"},
		{"ServiceName", "OrderService"},
		{"IsValidationRequired", false},
		{"OperationName", "FetchSampleItems"},
		{"RequestProxyType", "DefaultProxy"},
		{"TimeOutExpression", "30"},
		{"UseRequestTimeOut", true},
		{"ProxyConfiguration", nil},
	} {
		if got := docGet(doc, want.key); got != want.val {
			t.Errorf("%s = %#v, want %#v", want.key, got, want.val)
		}
	}

	// HttpConfiguration: a SOAP call writes the defaults with OverrideLocation
	// FALSE, unlike the REST writer's shared $Type which writes true.
	hc, ok := docGet(doc, "HttpConfiguration").(bsonv1.D)
	if !ok {
		t.Fatalf("HttpConfiguration = %#v, want a document", docGet(doc, "HttpConfiguration"))
	}
	if got := docGet(hc, "HttpMethod"); got != "Post" {
		t.Errorf("HttpConfiguration.HttpMethod = %#v, want Post", got)
	}
	if got := docGet(hc, "OverrideLocation"); got != false {
		t.Errorf("HttpConfiguration.OverrideLocation = %#v, want false", got)
	}
	// The typed-array marker is load-bearing: a wrong one is the class of defect
	// that makes a project Studio Pro cannot open. SOAP's empty HttpHeaderEntries
	// is marker 3 where REST's is 2, on the same $Type — which is why this is
	// written explicitly rather than registered globally.
	assertTypedArrayMarker(t, hc, "HttpHeaderEntries", 3)

	for _, field := range []string{"RequestBodyHandling", "RequestHeaderHandling"} {
		rh, ok := docGet(doc, field).(bsonv1.D)
		if !ok {
			t.Fatalf("%s = %#v, want a document", field, docGet(doc, field))
		}
		if got := docGet(rh, "$Type"); got != "Microflows$SimpleRequestHandling" {
			t.Errorf("%s.$Type = %#v", field, got)
		}
		if got := docGet(rh, "NullValueOption"); got != "LeaveOutElement" {
			t.Errorf("%s.NullValueOption = %#v", field, got)
		}
		assertTypedArrayMarker(t, rh, "ParameterMappings", 2)
	}
}

// TestWebServiceCallAction_ResultHandlingBindsTheReceiveMapping — the receive
// mapping travels in the ImportMappingCall under ReturnValueMapping (NOT
// "Mapping", which is what gen binds), by qualified name rather than by UUID.
// Getting the key wrong here reads back as a call with no mapping.
func TestWebServiceCallAction_ResultHandlingBindsTheReceiveMapping(t *testing.T) {
	doc := encodeMicroflowAction(t, fullWebServiceCall())

	rh, ok := docGet(doc, "NewResultHandling").(bsonv1.D)
	if !ok {
		t.Fatalf("NewResultHandling = %#v, want a document", docGet(doc, "NewResultHandling"))
	}
	if got := docGet(rh, "Bind"); got != true {
		t.Errorf("Bind = %#v, want true (the statement assigned $Root)", got)
	}
	if got := docGet(rh, "ResultVariableName"); got != "Root" {
		t.Errorf("ResultVariableName = %#v, want Root", got)
	}
	imc, ok := docGet(rh, "ImportMappingCall").(bsonv1.D)
	if !ok {
		t.Fatalf("ImportMappingCall = %#v, want a document", docGet(rh, "ImportMappingCall"))
	}
	if got := docGet(imc, "ReturnValueMapping"); got != "SampleSOAP.OrderResponse" {
		t.Errorf("ReturnValueMapping = %#v, want the qualified mapping name", got)
	}
	// Xml, not Json. A SOAP response is XML, and Studio Pro writes "Xml" in both
	// reference calls carrying an import mapping (ako/TestApp, 11.14.0). Legacy
	// hardcoded "Json"; both engines now write Xml.
	if got := docGet(imc, "ContentType"); got != "Xml" {
		t.Errorf("ContentType = %#v, want Xml", got)
	}
	rng, ok := docGet(imc, "Range").(bsonv1.D)
	if !ok {
		t.Fatalf("Range = %#v, want a document", docGet(imc, "Range"))
	}
	if got := docGet(rng, "$Type"); got != "Microflows$ConstantRange" {
		t.Errorf("Range.$Type = %#v", got)
	}
}

// TestWebServiceCallAction_ServiceNameIsTheWsdlService — ServiceName is the
// WSDL <wsdl:service name=…>, resolved by the executor off the imported service
// document, NOT the local part of the qualified document name. Writing the
// derived name made Mendix look for the operation in a service that does not
// exist: CE0386, measured on 11.14.0 against ako/TestApp.
func TestWebServiceCallAction_ServiceNameIsTheWsdlService(t *testing.T) {
	a := fullWebServiceCall()
	a.ServiceName = "OrdersWS"

	if got := docGet(encodeMicroflowAction(t, a), "ServiceName"); got != "OrdersWS" {
		t.Errorf("ServiceName = %#v, want the resolved WSDL service name", got)
	}

	// Control: unresolved, the writer falls back to the derivation that ships
	// today rather than writing nothing. A call against a service mxcli cannot
	// resolve is then no worse off than before.
	a.ServiceName = ""
	if got := docGet(encodeMicroflowAction(t, a), "ServiceName"); got != "OrderService" {
		t.Errorf("fallback ServiceName = %#v, want the derived OrderService", got)
	}
}

// TestWebServiceCallAction_NoOutputVariable — a call that binds nothing writes
// Bind false and an explicitly null ImportMappingCall, rather than omitting the
// result handling.
func TestWebServiceCallAction_NoOutputVariable(t *testing.T) {
	a := fullWebServiceCall()
	a.OutputVariable = ""
	a.UseReturnVariable = false
	a.ReceiveMappingID = ""

	rh, ok := docGet(encodeMicroflowAction(t, a), "NewResultHandling").(bsonv1.D)
	if !ok {
		t.Fatal("NewResultHandling missing")
	}
	if got := docGet(rh, "Bind"); got != false {
		t.Errorf("Bind = %#v, want false", got)
	}
	if got := docGet(rh, "ImportMappingCall"); got != nil {
		t.Errorf("ImportMappingCall = %#v, want null", got)
	}
}

// TestWebServiceCallAction_DefaultTimeout — an omitted TIMEOUT writes Mendix's
// own default rather than an empty expression, matching legacy.
func TestWebServiceCallAction_DefaultTimeout(t *testing.T) {
	a := fullWebServiceCall()
	a.TimeoutExpression = ""

	if got := docGet(encodeMicroflowAction(t, a), "TimeOutExpression"); got != "300" {
		t.Errorf("TimeOutExpression = %#v, want 300", got)
	}
}

// TestWebServiceCallAction_RawPassthrough — `call web service raw '<base64>'` is
// the escape hatch for operations neither engine can spell structurally (a SEND
// MAPPING needs Mendix$AdvancedRequestHandling, whose storage name has no
// reference here). The payload must re-emit unchanged, not be replaced by the
// structured form.
func TestWebServiceCallAction_RawPassthrough(t *testing.T) {
	raw, err := bsonv1.Marshal(bsonv1.D{
		{Key: "$Type", Value: "Microflows$CallWebServiceAction"},
		{Key: "ImportedService", Value: "Raw.Service"},
		{Key: "OperationName", Value: "RawOp"},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	a := fullWebServiceCall()
	a.RawBSON = raw

	doc := encodeMicroflowAction(t, a)
	if got := docGet(doc, "ImportedService"); got != "Raw.Service" {
		t.Errorf("ImportedService = %#v, want the RAW payload's value, not the structured one", got)
	}
	if got := docGet(doc, "OperationName"); got != "RawOp" {
		t.Errorf("OperationName = %#v, want the RAW payload's value", got)
	}
	// Control: without the raw payload the same action writes the structured
	// form, so the assertions above are not satisfied by a writer that ignores
	// both.
	a.RawBSON = nil
	if got := docGet(encodeMicroflowAction(t, a), "ImportedService"); got != "SampleSOAP.OrderService" {
		t.Errorf("structured ImportedService = %#v, want SampleSOAP.OrderService", got)
	}
}

// assertTypedArrayMarker checks an empty Mendix typed array: a one-element BSON
// array holding just the int32 version marker.
func assertTypedArrayMarker(t *testing.T, doc bsonv1.D, key string, want int32) {
	t.Helper()
	arr, ok := docGet(doc, key).(bsonv1.A)
	if !ok {
		t.Fatalf("%s = %#v, want a typed array", key, docGet(doc, key))
	}
	if len(arr) != 1 {
		t.Fatalf("%s has %d entries, want just the marker", key, len(arr))
	}
	if got, ok := arr[0].(int32); !ok || got != want {
		t.Errorf("%s marker = %#v, want int32(%d)", key, arr[0], want)
	}
}
