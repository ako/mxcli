// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"
	bsonv2 "go.mongodb.org/mongo-driver/v2/bson"

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
// It is NOT a fidelity test, and the difference matters for reading the
// expectations below. Studio Pro-authored SOAP documents exist (ako/TestApp,
// 11.14.0); measured against them, four of legacy's six divergences have since
// been fixed in BOTH engines, so most of this test now agrees with Studio Pro
// too. The exceptions are deliberate:
//
//   - ServiceName "OrderService" here is the FALLBACK, exercised because this
//     action carries no resolved ServiceName. The resolved path has its own
//     test (TestWebServiceCallAction_ServiceNameIsTheWsdlService).
//   - Range.SingleObject and the send-mapping request handling are the two
//     divergences still open; the header comment in
//     microflow_webservice_write.go says what each one costs and why it has not
//     been changed on a guess.
//
// When those are fixed these expectations change with them, which is the point
// of recording which reference each one came from.
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

// TestWebServiceCallAction_VariableTypeIsTheMappingsEntity — the result's type
// is the entity the receive mapping produces. VoidType says the call returns
// nothing: CE0243 and CE0366, measured on 11.14.0 against ako/TestApp.
func TestWebServiceCallAction_VariableTypeIsTheMappingsEntity(t *testing.T) {
	a := fullWebServiceCall()
	a.ResultEntity = "Clients.Order"

	rh, ok := docGet(encodeMicroflowAction(t, a), "NewResultHandling").(bsonv1.D)
	if !ok {
		t.Fatal("NewResultHandling missing")
	}
	vt, ok := docGet(rh, "VariableType").(bsonv1.D)
	if !ok {
		t.Fatalf("VariableType = %#v, want a document", docGet(rh, "VariableType"))
	}
	if got := docGet(vt, "$Type"); got != "DataTypes$ObjectType" {
		t.Errorf("VariableType.$Type = %#v, want DataTypes$ObjectType", got)
	}
	if got := docGet(vt, "Entity"); got != "Clients.Order" {
		t.Errorf("VariableType.Entity = %#v, want Clients.Order", got)
	}

	// Control: unresolved, it stays VoidType — wrong, but what ships, so an
	// unresolvable mapping is no worse off than before.
	a.ResultEntity = ""
	rh2, _ := docGet(encodeMicroflowAction(t, a), "NewResultHandling").(bsonv1.D)
	vt2, _ := docGet(rh2, "VariableType").(bsonv1.D)
	if got := docGet(vt2, "$Type"); got != "DataTypes$VoidType" {
		t.Errorf("fallback VariableType.$Type = %#v, want DataTypes$VoidType", got)
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

// TestWebServiceCallAction_ArgumentsAreSimpleParameterMappings is the regression
// test for CE0178.
//
// An operation taking parameters needs them bound, and an empty
// SimpleRequestHandling.ParameterMappings is mxbuild's "Body parameter mapping
// needs to be refreshed." Measured on 11.14.0 against ako/TestApp: the same
// script goes 1 error -> 0 once the list is written.
//
// The ParameterPath is asserted character for character against what Studio Pro
// stored, because a plausible wrong escaping is exactly what mxbuild accepts and
// Studio Pro does not.
func TestWebServiceCallAction_ArgumentsAreSimpleParameterMappings(t *testing.T) {
	a := fullWebServiceCall()
	a.Arguments = []microflows.WebServiceArgument{{
		Name:       "OrderId",
		Path:       "http%3A//www.example.com/:GetOrder|OrderId",
		Expression: "$Customer/OrderId",
		Checked:    true,
	}}

	body, ok := docGet(encodeMicroflowAction(t, a), "RequestBodyHandling").(bsonv1.D)
	if !ok {
		t.Fatal("RequestBodyHandling is not a document")
	}
	if got := docGet(body, "$Type"); got != "Microflows$SimpleRequestHandling" {
		t.Fatalf("$Type = %#v, want Microflows$SimpleRequestHandling", got)
	}

	arr, ok := docGet(body, "ParameterMappings").(bsonv1.A)
	if !ok || len(arr) != 2 {
		t.Fatalf("ParameterMappings = %#v, want the marker plus one mapping", docGet(body, "ParameterMappings"))
	}
	// Marker 2 — measured on all three reference calls. The codec's DEFAULT is
	// 3, so without the RegisterListMarker in the writer this is the assertion
	// that fails, and a wrong marker is the class of defect that makes a project
	// Studio Pro cannot open while mxbuild stays silent.
	if got, isInt := arr[0].(int32); !isInt || got != 2 {
		t.Errorf("ParameterMappings marker = %#v, want int32(2)", arr[0])
	}

	pm, ok := arr[1].(bsonv1.D)
	if !ok {
		t.Fatalf("mapping = %#v, want a document", arr[1])
	}
	for _, want := range []struct {
		key string
		val any
	}{
		{"$Type", "Microflows$WebServiceOperationSimpleParameterMapping"},
		{"Argument", "$Customer/OrderId"},
		{"IsChecked", true},
		// "" in both reference mappings; what fills it is unmeasured.
		{"ParameterName", ""},
		{"ParameterPath", "http%3A//www.example.com/:GetOrder|OrderId"},
	} {
		if got := docGet(pm, want.key); got != want.val {
			t.Errorf("%s = %#v, want %#v", want.key, got, want.val)
		}
	}

	// The HEADER handling stays the bare empty form — it is not where arguments go.
	hdr, ok := docGet(encodeMicroflowAction(t, a), "RequestHeaderHandling").(bsonv1.D)
	if !ok {
		t.Fatal("RequestHeaderHandling is not a document")
	}
	assertTypedArrayMarker(t, hdr, "ParameterMappings", 2)
}

// TestWebServiceCallAction_SendMappingIsAMappingRequestHandling is the
// regression test for CE0369.
//
// `send mapping` parsed, was accepted and was DISCARDED: the writer emitted an
// empty SimpleRequestHandling regardless, and mxbuild reported "Cannot use
// simple request body, as the operation's body is complex". The mapping name
// appeared zero times in the written document, on either engine.
//
// The two name keys are the ones modelsdk/gen gets wrong (its key audit lists
// Mapping -> MappingId and MappingArgumentVariableName -> MappingVariableName),
// so this test is what stops a future rewrite through the gen accessors: the
// wrong spellings build clean and give a document Studio Pro cannot open.
func TestWebServiceCallAction_SendMappingIsAMappingRequestHandling(t *testing.T) {
	a := fullWebServiceCall()
	a.ReceiveMappingID = ""
	a.OutputVariable = ""
	a.UseReturnVariable = false
	a.SendMappingID = "Clients.SoapOrderExportMapping"
	a.SendMappingVariable = "NewSaveOrder"

	body, ok := docGet(encodeMicroflowAction(t, a), "RequestBodyHandling").(bsonv1.D)
	if !ok {
		t.Fatal("RequestBodyHandling is not a document")
	}
	for _, want := range []struct {
		key string
		val any
	}{
		{"$Type", "Microflows$MappingRequestHandling"},
		// "Json" on an XML protocol is what Studio Pro wrote on the one
		// reference document — written as observed, not as it would seem.
		{"ContentType", "Json"},
		{"MappingId", "Clients.SoapOrderExportMapping"},
		{"MappingVariableName", "NewSaveOrder"},
	} {
		if got := docGet(body, want.key); got != want.val {
			t.Errorf("%s = %#v, want %#v", want.key, got, want.val)
		}
	}
	// A marker variant carries no Value/ParameterMappings of the other form.
	if got := docGet(body, "ParameterMappings"); got != nil {
		t.Errorf("ParameterMappings = %#v on a mapping body, want absent", got)
	}
}

// TestWebServiceCallAction_SendMappingContentTypeIsCarried — a stored
// ContentType survives a rewrite rather than being normalised to the default.
// One reference document is not enough to call "Json" the rule.
func TestWebServiceCallAction_SendMappingContentTypeIsCarried(t *testing.T) {
	a := fullWebServiceCall()
	a.SendMappingID = "M.Export"
	a.SendMappingVariable = "Order"
	a.SendMappingContentType = "Xml"

	body, _ := docGet(encodeMicroflowAction(t, a), "RequestBodyHandling").(bsonv1.D)
	if got := docGet(body, "ContentType"); got != "Xml" {
		t.Errorf("ContentType = %#v, want the stored Xml", got)
	}
}

// referenceSoapActionMap is the fifteen-key shape every ako/TestApp SOAP call
// carries, with mxcli's own values for the six boilerplate keys.
//
// It is built as a map and mutated BEFORE marshalling on purpose. Round-tripping
// through bson.Unmarshal to poke at a nested field does not work here: driver v2
// decodes nested documents into bson.D even when the top level is a bson.M, the
// mirror image of the map-vs-D trap already recorded for driver v1.
func referenceSoapActionMap() bsonv2.M {
	return bsonv2.M{
		"$Type":             "Microflows$CallWebServiceAction",
		"ErrorHandlingType": "Rollback",
		"HttpConfiguration": bsonv2.M{
			"$Type":                      "Microflows$HttpConfiguration",
			"ClientCertificate":          "",
			"CustomLocation":             "",
			"CustomLocationTemplate":     nil,
			"HttpAuthenticationPassword": "",
			"HttpAuthenticationUserName": "",
			"HttpHeaderEntries":          bsonv2.A{int32(3)},
			"HttpMethod":                 "Post",
			"OverrideLocation":           false,
			"UseHttpAuthentication":      false,
		},
		"ImportedService":      "Clients.OrderSoapClient",
		"IsValidationRequired": false,
		"NewResultHandling": bsonv2.M{
			"$Type": "Microflows$ResultHandling", "Bind": true,
			"ImportMappingCall": bsonv2.M{
				"$Type": "Microflows$ImportMappingCall", "Commit": "YesWithoutEvents",
				"ContentType": "Xml", "ForceSingleOccurrence": false,
				"ObjectHandlingBackup": "Create", "ParameterVariableName": "",
				"Range":              bsonv2.M{"$Type": "Microflows$ConstantRange", "SingleObject": true},
				"ReturnValueMapping": "Clients.SoapOrdersImportMapping",
			},
			"ResultVariableName": "Orders",
			"VariableType":       bsonv2.M{"$Type": "DataTypes$ObjectType", "Entity": "Clients.Order"},
		},
		"OperationName": "GetOrder", "ProxyConfiguration": nil,
		"RequestBodyHandling": bsonv2.M{
			"$Type": "Microflows$SimpleRequestHandling", "NullValueOption": "LeaveOutElement",
			"ParameterMappings": bsonv2.A{int32(2), bsonv2.M{
				"$Type":    "Microflows$WebServiceOperationSimpleParameterMapping",
				"Argument": "2", "IsChecked": true, "ParameterName": "",
				"ParameterPath": "http%3A//www.example.com/:GetOrder|OrderId",
			}},
		},
		"RequestHeaderHandling": bsonv2.M{
			"$Type": "Microflows$SimpleRequestHandling", "NullValueOption": "LeaveOutElement",
			"ParameterMappings": bsonv2.A{int32(2)},
		},
		"RequestProxyType": "DefaultProxy", "ServiceName": "OrdersWS",
		"TimeOutExpression": "300", "UseRequestTimeOut": true,
	}
}

func marshalAction(t *testing.T, m bsonv2.M) bsonv2.Raw {
	t.Helper()
	out, err := bsonv2.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}

// TestWebServiceActionRequiresRawBSON_AgreesWithLegacy mirrors the sdk/mpr twin
// case for case. The two engines implement this decision SEPARATELY — one over
// bson.Raw, one over map[string]any — so nothing but a pair of tests keeps them
// from drifting, and a drift here means the same project describes differently
// depending on which engine read it.
func TestWebServiceActionRequiresRawBSON_AgreesWithLegacy(t *testing.T) {
	if webServiceActionRequiresRawBSON(marshalAction(t, referenceSoapActionMap())) {
		t.Error("an action mxcli would write itself still falls back to raw")
	}

	for _, tc := range []struct {
		name  string
		mutit func(bsonv2.M)
	}{
		// Measured: Clients.GetOrders stores SingleObject FALSE where mxcli
		// writes true. No error comes of it, which is exactly why writing it
		// back must not happen silently.
		{"Range.SingleObject differs", func(m bsonv2.M) {
			m["NewResultHandling"].(bsonv2.M)["ImportMappingCall"].(bsonv2.M)["Range"] =
				bsonv2.M{"$Type": "Microflows$ConstantRange", "SingleObject": false}
		}},
		// Measured: Clients.SaveOrder binds $IsSaved with NO import mapping and
		// a DataTypes$BooleanType — the OPERATION's return type, which lives in
		// the WSDL. Written back as VoidType it is CE0366 + CE6011.
		{"result type comes from the WSDL, not a mapping", func(m bsonv2.M) {
			m["NewResultHandling"] = bsonv2.M{
				"$Type": "Microflows$ResultHandling", "Bind": true,
				"ImportMappingCall":  nil,
				"ResultVariableName": "IsSaved",
				"VariableType":       bsonv2.M{"$Type": "DataTypes$BooleanType"},
			}
		}},
		{"HTTP authentication configured", func(m bsonv2.M) {
			m["HttpConfiguration"].(bsonv2.M)["UseHttpAuthentication"] = true
		}},
		{"custom location", func(m bsonv2.M) {
			m["HttpConfiguration"].(bsonv2.M)["CustomLocation"] = "https://elsewhere/"
		}},
		{"a SOAP header is configured", func(m bsonv2.M) {
			m["RequestHeaderHandling"].(bsonv2.M)["ParameterMappings"] = bsonv2.A{int32(2),
				bsonv2.M{"$Type": "Microflows$WebServiceOperationSimpleParameterMapping"}}
		}},
		{"validation required", func(m bsonv2.M) { m["IsValidationRequired"] = true }},
		{"non-default proxy", func(m bsonv2.M) { m["RequestProxyType"] = "NoProxy" }},
		{"timeout disabled", func(m bsonv2.M) { m["UseRequestTimeOut"] = false }},
		{"advanced parameter mapping", func(m bsonv2.M) {
			m["RequestBodyHandling"].(bsonv2.M)["ParameterMappings"] = bsonv2.A{int32(2),
				bsonv2.M{"$Type": "Microflows$WebServiceOperationAdvancedParameterMapping"}}
		}},
		{"parameter path with no name segment", func(m bsonv2.M) {
			pms := m["RequestBodyHandling"].(bsonv2.M)["ParameterMappings"].(bsonv2.A)
			pms[1].(bsonv2.M)["ParameterPath"] = "http%3A//www.example.com/:GetOrder"
		}},
		{"unknown key entirely", func(m bsonv2.M) { m["SomethingNew"] = int32(1) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := referenceSoapActionMap()
			tc.mutit(m)
			if !webServiceActionRequiresRawBSON(marshalAction(t, m)) {
				t.Error("describes structurally, so a round trip would silently rewrite it")
			}
		})
	}
}
