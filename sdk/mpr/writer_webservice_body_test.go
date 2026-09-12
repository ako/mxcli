// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
	"go.mongodb.org/mongo-driver/bson"
)

func bodyGet(doc bson.D, key string) any {
	for _, e := range doc {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// TestWebServiceRequestBody_Arguments is the legacy half of the CE0178 fix, and
// exists to keep the two engines from drifting: the modelsdk twin
// (TestWebServiceCallAction_ArgumentsAreSimpleParameterMappings) asserts the
// same keys, values and marker.
func TestWebServiceRequestBody_Arguments(t *testing.T) {
	doc := webServiceRequestBody(&microflows.WebServiceCallAction{
		Arguments: []microflows.WebServiceArgument{{
			Name:       "OrderId",
			Path:       "http%3A//www.example.com/:GetOrder|OrderId",
			Expression: "$Customer/OrderId",
			Checked:    true,
		}},
	})

	if got := bodyGet(doc, "$Type"); got != "Microflows$SimpleRequestHandling" {
		t.Fatalf("$Type = %#v", got)
	}
	arr, ok := bodyGet(doc, "ParameterMappings").(bson.A)
	if !ok || len(arr) != 2 {
		t.Fatalf("ParameterMappings = %#v, want the marker plus one mapping", bodyGet(doc, "ParameterMappings"))
	}
	if got, isInt := arr[0].(int32); !isInt || got != 2 {
		t.Errorf("marker = %#v, want int32(2)", arr[0])
	}
	pm, ok := arr[1].(bson.D)
	if !ok {
		t.Fatalf("mapping = %#v", arr[1])
	}
	for _, want := range []struct {
		key string
		val any
	}{
		{"$Type", "Microflows$WebServiceOperationSimpleParameterMapping"},
		{"Argument", "$Customer/OrderId"},
		{"IsChecked", true},
		{"ParameterName", ""},
		{"ParameterPath", "http%3A//www.example.com/:GetOrder|OrderId"},
	} {
		if got := bodyGet(pm, want.key); got != want.val {
			t.Errorf("%s = %#v, want %#v", want.key, got, want.val)
		}
	}
}

// TestWebServiceRequestBody_SendMapping is the legacy half of the CE0369 fix.
//
// MappingId / MappingVariableName are the STORAGE names; modelsdk/gen binds the
// same two properties as Mapping / MappingArgumentVariableName, which mxbuild
// tolerates and Studio Pro cannot open.
func TestWebServiceRequestBody_SendMapping(t *testing.T) {
	doc := webServiceRequestBody(&microflows.WebServiceCallAction{
		SendMappingID:       "Clients.SoapOrderExportMapping",
		SendMappingVariable: "NewSaveOrder",
	})
	for _, want := range []struct {
		key string
		val any
	}{
		{"$Type", "Microflows$MappingRequestHandling"},
		{"ContentType", "Json"},
		{"MappingId", "Clients.SoapOrderExportMapping"},
		{"MappingVariableName", "NewSaveOrder"},
	} {
		if got := bodyGet(doc, want.key); got != want.val {
			t.Errorf("%s = %#v, want %#v", want.key, got, want.val)
		}
	}
}

// TestWebServiceRequestBody_EmptyIsTheBareSimpleForm — a call with neither
// clause still writes the empty Simple body every reference call carries, so
// this change does not alter what already shipped.
func TestWebServiceRequestBody_EmptyIsTheBareSimpleForm(t *testing.T) {
	doc := webServiceRequestBody(&microflows.WebServiceCallAction{})
	if got := bodyGet(doc, "$Type"); got != "Microflows$SimpleRequestHandling" {
		t.Fatalf("$Type = %#v", got)
	}
	arr, ok := bodyGet(doc, "ParameterMappings").(bson.A)
	if !ok || len(arr) != 1 {
		t.Fatalf("ParameterMappings = %#v, want just the marker", bodyGet(doc, "ParameterMappings"))
	}
}

// TestParseWebServiceRequestBody round-trips both variants back into the model,
// and pins that the parameter NAME comes from the path's last segment — the only
// part MDL spells.
func TestParseWebServiceRequestBody(t *testing.T) {
	action := &microflows.WebServiceCallAction{}
	parseWebServiceRequestBody(map[string]any{
		"RequestBodyHandling": map[string]any{
			"$Type":           "Microflows$SimpleRequestHandling",
			"NullValueOption": "LeaveOutElement",
			"ParameterMappings": []any{int32(2), map[string]any{
				"$Type":         "Microflows$WebServiceOperationSimpleParameterMapping",
				"Argument":      "2",
				"IsChecked":     true,
				"ParameterName": "",
				"ParameterPath": "http%3A//www.example.com/:GetOrder|OrderId",
			}},
		},
	}, action)
	if len(action.Arguments) != 1 {
		t.Fatalf("read %d arguments, want 1", len(action.Arguments))
	}
	got := action.Arguments[0]
	if got.Name != "OrderId" || got.Expression != "2" || !got.Checked ||
		got.Path != "http%3A//www.example.com/:GetOrder|OrderId" {
		t.Errorf("argument = %+v", got)
	}

	mapped := &microflows.WebServiceCallAction{}
	parseWebServiceRequestBody(map[string]any{
		"RequestBodyHandling": map[string]any{
			"$Type":               "Microflows$MappingRequestHandling",
			"ContentType":         "Json",
			"MappingId":           "Clients.SoapOrderExportMapping",
			"MappingVariableName": "NewSaveOrder",
		},
	}, mapped)
	if string(mapped.SendMappingID) != "Clients.SoapOrderExportMapping" ||
		mapped.SendMappingVariable != "NewSaveOrder" ||
		mapped.SendMappingContentType != "Json" {
		t.Errorf("send mapping = %+v", mapped)
	}
	// The two variants differ in ARITY, so dispatching on $Type rather than on
	// which fields are present is what stops one being read as the other.
	if len(mapped.Arguments) != 0 {
		t.Errorf("a mapping body produced %d arguments", len(mapped.Arguments))
	}
}

// referenceSoapAction builds the fifteen-key action shape every ako/TestApp SOAP
// call carries, with mxcli's own values for the six boilerplate keys.
func referenceSoapAction() map[string]any {
	return map[string]any{
		"$ID": "a", "$Type": "Microflows$CallWebServiceAction",
		"ErrorHandlingType": "Rollback",
		"HttpConfiguration": map[string]any{
			"$Type":             "Microflows$HttpConfiguration",
			"ClientCertificate": "", "CustomLocation": "",
			"CustomLocationTemplate":     nil,
			"HttpAuthenticationPassword": "", "HttpAuthenticationUserName": "",
			"HttpHeaderEntries": []any{int32(3)},
			"HttpMethod":        "Post",
			"OverrideLocation":  false, "UseHttpAuthentication": false,
		},
		"ImportedService": "Clients.OrderSoapClient", "IsValidationRequired": false,
		"NewResultHandling": map[string]any{
			"$Type": "Microflows$ResultHandling", "Bind": true,
			"ImportMappingCall": map[string]any{
				"$Type": "Microflows$ImportMappingCall", "Commit": "YesWithoutEvents",
				"ContentType": "Xml", "ForceSingleOccurrence": false,
				"ObjectHandlingBackup": "Create", "ParameterVariableName": "",
				"Range":              map[string]any{"$Type": "Microflows$ConstantRange", "SingleObject": true},
				"ReturnValueMapping": "Clients.SoapOrdersImportMapping",
			},
			"ResultVariableName": "Orders",
			"VariableType":       map[string]any{"$Type": "DataTypes$ObjectType", "Entity": "Clients.Order"},
		},
		"OperationName": "GetOrder", "ProxyConfiguration": nil,
		"RequestBodyHandling": map[string]any{
			"$Type": "Microflows$SimpleRequestHandling", "NullValueOption": "LeaveOutElement",
			"ParameterMappings": []any{int32(2), map[string]any{
				"$Type":    "Microflows$WebServiceOperationSimpleParameterMapping",
				"Argument": "2", "IsChecked": true, "ParameterName": "",
				"ParameterPath": "http%3A//www.example.com/:GetOrder|OrderId",
			}},
		},
		"RequestHeaderHandling": map[string]any{
			"$Type": "Microflows$SimpleRequestHandling", "NullValueOption": "LeaveOutElement",
			"ParameterMappings": []any{int32(2)},
		},
		"RequestProxyType": "DefaultProxy", "ServiceName": "OrdersWS",
		"TimeOutExpression": "300", "UseRequestTimeOut": true,
	}
}

// TestWebServiceActionRequiresRawBSON_StructuredWhenReproducible — an action
// mxcli itself would write describes structurally rather than as base64.
//
// Before the request body was authorable this could never happen: a real call
// carries fifteen keys and only nine were admitted, so EVERY SOAP call in every
// project — Studio Pro's and mxcli's — rendered as `call web service raw '<…>'`.
func TestWebServiceActionRequiresRawBSON_StructuredWhenReproducible(t *testing.T) {
	if webServiceActionRequiresRawBSON(referenceSoapAction()) {
		t.Error("an action mxcli would write itself still falls back to raw")
	}
}

// TestWebServiceActionRequiresRawBSON_ValueSensitive is the regression test for
// what a describe -> exec round trip over ako/TestApp actually caught.
//
// Admitting the six boilerplate keys BY NAME would have silently normalised a
// call the moment anyone round-tripped it. Each case below is a document mxcli
// would write differently, so each must keep the byte-exact raw fallback.
func TestWebServiceActionRequiresRawBSON_ValueSensitive(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mutit func(map[string]any)
	}{
		// Measured: Clients.GetOrders stores SingleObject FALSE where mxcli
		// writes true. No error comes of it, which is exactly why writing it
		// back must not happen silently — the round trip would change the
		// user's document with nothing to show for it.
		{"Range.SingleObject differs", func(m map[string]any) {
			rh := m["NewResultHandling"].(map[string]any)
			imc := rh["ImportMappingCall"].(map[string]any)
			imc["Range"] = map[string]any{"$Type": "Microflows$ConstantRange", "SingleObject": false}
		}},
		// Measured: Clients.SaveOrder binds $IsSaved with NO import mapping and
		// a DataTypes$BooleanType — the OPERATION's return type, which lives in
		// the WSDL. Written back as VoidType it is CE0366 + CE6011.
		{"result type comes from the WSDL, not a mapping", func(m map[string]any) {
			m["NewResultHandling"] = map[string]any{
				"$Type": "Microflows$ResultHandling", "Bind": true,
				"ImportMappingCall":  nil,
				"ResultVariableName": "IsSaved",
				"VariableType":       map[string]any{"$Type": "DataTypes$BooleanType"},
			}
		}},
		{"HTTP authentication configured", func(m map[string]any) {
			m["HttpConfiguration"].(map[string]any)["UseHttpAuthentication"] = true
		}},
		{"custom location", func(m map[string]any) {
			m["HttpConfiguration"].(map[string]any)["CustomLocation"] = "https://elsewhere/"
		}},
		{"a SOAP header is configured", func(m map[string]any) {
			m["RequestHeaderHandling"].(map[string]any)["ParameterMappings"] = []any{int32(2),
				map[string]any{"$Type": "Microflows$WebServiceOperationSimpleParameterMapping"}}
		}},
		{"validation required", func(m map[string]any) { m["IsValidationRequired"] = true }},
		{"non-default proxy", func(m map[string]any) { m["RequestProxyType"] = "NoProxy" }},
		{"timeout disabled", func(m map[string]any) { m["UseRequestTimeOut"] = false }},
		// A per-parameter export mapping has no MDL spelling at all.
		{"advanced parameter mapping", func(m map[string]any) {
			m["RequestBodyHandling"].(map[string]any)["ParameterMappings"] = []any{int32(2),
				map[string]any{"$Type": "Microflows$WebServiceOperationAdvancedParameterMapping"}}
		}},
		// Without a "|" the parameter name MDL spells cannot be recovered, so
		// the write path could not rebuild the same path.
		{"parameter path with no name segment", func(m map[string]any) {
			pms := m["RequestBodyHandling"].(map[string]any)["ParameterMappings"].([]any)
			pms[1].(map[string]any)["ParameterPath"] = "http%3A//www.example.com/:GetOrder"
		}},
		{"unknown key entirely", func(m map[string]any) { m["SomethingNew"] = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := referenceSoapAction()
			tc.mutit(m)
			if !webServiceActionRequiresRawBSON(m) {
				t.Error("describes structurally, so a round trip would silently rewrite it")
			}
		})
	}
}
