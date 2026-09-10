// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/modelsdk/property"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// SOAP `call web service` on the codec engine.
//
// The codec engine READ this action already (microflow_read_actions.go), but the
// write switch had no case for it, so it fell through to `default: return nil`
// and the enclosing ActionActivity was written with NO action at all. That is the
// #850 shape and it is worse than an unsupported feature: `mxcli exec` reported
// success on all three microflows and mxbuild then failed the project —
// measured on 11.13.0, `06b-soap-examples.mdl` on the DEFAULT engine gives
//
//	[CE0008] "No action defined." at Action activity 'Activity'
//	[CE0109] "Undefined variable 'Root'." at End event   (×2)
//
// the CE0109s being the knock-on from the dropped action never binding $Root.
// Legacy was the documented fallback (cmd/mxcli/engine.go), which is exactly the
// dependency that keeps the legacy engine alive.
//
// The target shape is byte-parity with the legacy serializer
// (sdk/mpr.serializeWebServiceCallAction), so that fixing the silent drop
// changes nothing else. TestWebServiceCallAction_MatchesLegacyDocument holds the
// two engines together; a discrepancy is a test failure, not a silent divergence.
//
// PARITY WITH LEGACY IS NOT FIDELITY TO STUDIO PRO, and the difference is now
// measured rather than assumed. This file first claimed no Studio Pro-authored
// SOAP document existed to pin against; one does — ako/TestApp carries three
// (Clients.GetOrders / GetCustomerOrders / SaveOrder, Mendix 11.14.0), and
// against them legacy is wrong in five places that this file faithfully
// reproduces:
//
//   - ServiceName is the WSDL SERVICE name ("OrdersWS"), not the local part of
//     the imported service's qualified name — Studio Pro writes ServiceName
//     "OrdersWS" beside ImportedService "Clients.OrderSoapClient".
//   - ImportMappingCall.ContentType is "Xml" for a SOAP import mapping, not the
//     hardcoded "Json".
//   - Range.SingleObject follows the operation's cardinality; a list result
//     writes false, not the hardcoded true.
//   - VariableType is the result's REAL type (DataTypes$ObjectType with an
//     Entity, DataTypes$BooleanType, …), not DataTypes$VoidType.
//   - A SEND MAPPING is Microflows$MappingRequestHandling {ContentType,
//     MappingId, MappingVariableName} — a type this engine ALREADY writes for
//     REST. It is not the "Mendix$AdvancedRequestHandling" the legacy comment
//     guessed at, which occurs in none of the three documents.
//
// Operation arguments are the sixth: Studio Pro carries them as
// Microflows$WebServiceOperationSimpleParameterMapping entries inside
// RequestBodyHandling.ParameterMappings, keyed by an escaped ParameterPath
// ("http%3A//www.example.com/:GetOrder|OrderId"). Both engines write that list
// empty, so a call's arguments do not reach the model.
//
// None of that is fixed here: this change is scoped to the silent drop, and
// reproducing what ships is what makes it safe to land. Closing the gaps is
// follow-up work against those reference documents.
//
// Two shapes are deliberately NOT re-derived here:
//
//   - HttpHeaderEntries is written as an empty typed array with marker 3, and
//     the two SimpleRequestHandling ParameterMappings with marker 2, EXPLICITLY
//     rather than through codec.RegisterTypeDefaults. Those registrations are
//     global and keyed by $Type, and Microflows$HttpConfiguration is shared with
//     the REST writer, which needs marker 2 for the same field — registering a
//     SOAP-shaped default would silently change REST's output. (The package
//     already carries one such collision: Microflows$HttpHeaderEntry is
//     registered as 2 in microflow_write.go and as 3 in odata_write.go, and
//     which one wins is decided by file order.)
//   - RequestBodyHandling is always SimpleRequestHandling, even when the
//     statement carries a SEND MAPPING — matching legacy, and WRONG: the real
//     type is Microflows$MappingRequestHandling (see above). Until that is
//     implemented the send mapping is silently dropped on both engines, and
//     `call web service raw` is the only way to author one.
//
// Every null the document carries is written IN KEY POSITION rather than through
// NullFields, for the same reason and with the same consequence — see addNull.

// webServiceCallActionToGen builds a Microflows$CallWebServiceAction. Mirrors
// sdk/mpr.serializeWebServiceCallAction field-for-field, in the same key order.
func webServiceCallActionToGen(a *microflows.WebServiceCallAction) element.Element {
	// The raw escape hatch: `call web service raw '<base64>'` carries an opaque
	// document that must re-emit byte-for-byte. Legacy returns the unmarshalled
	// payload verbatim; the codec's decoder preserves an unknown subtree the same
	// way, so this is passthrough on both engines.
	if len(a.RawBSON) > 0 {
		if el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(a.RawBSON); err == nil && el != nil {
			return el
		}
		// A payload that will not decode is not silently dropped — falling
		// through writes the structured form, which is wrong but visible, and
		// the executor already reports a bad base64 payload at build time.
	}

	g := newElem("Microflows$CallWebServiceAction", string(a.ID))
	addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), "Rollback"))
	addPart(g, "HttpConfiguration", webServiceHttpConfigToGen())
	// ImportedService is a BY_NAME_REFERENCE qualified-name string, not a binary
	// UUID — the same convention the legacy writer notes.
	addStr(g, "ImportedService", string(a.ServiceID))
	addBool(g, "IsValidationRequired", false)
	addPart(g, "NewResultHandling", webServiceResultHandlingToGen(a))
	addStr(g, "OperationName", a.OperationName)
	addNull(g, "ProxyConfiguration")
	addPart(g, "RequestBodyHandling", simpleRequestHandlingToGen())
	addPart(g, "RequestHeaderHandling", simpleRequestHandlingToGen())
	addStr(g, "RequestProxyType", "DefaultProxy")
	addStr(g, "ServiceName", webServiceLocalName(string(a.ServiceID)))
	addStr(g, "TimeOutExpression", orDefault(a.TimeoutExpression, "300"))
	addBool(g, "UseRequestTimeOut", true)
	return g
}

// webServiceHttpConfigToGen builds the fixed HttpConfiguration a SOAP call
// carries. It is not httpConfigToGen: that one is driven by a REST action's own
// configuration and writes OverrideLocation TRUE, while a SOAP call writes the
// defaults with OverrideLocation false. Same $Type, different content.
func webServiceHttpConfigToGen() element.Element {
	hc := newElem("Microflows$HttpConfiguration", "")
	addStr(hc, "ClientCertificate", "")
	addStr(hc, "CustomLocation", "")
	addNull(hc, "CustomLocationTemplate")
	addStr(hc, "HttpAuthenticationPassword", "")
	addStr(hc, "HttpAuthenticationUserName", "")
	addEmptyTypedList(hc, "HttpHeaderEntries", 3)
	addStr(hc, "HttpMethod", "Post")
	addBool(hc, "OverrideLocation", false)
	addBool(hc, "UseHttpAuthentication", false)
	return hc
}

// webServiceResultHandlingToGen builds NewResultHandling, a Microflows$ResultHandling
// (the same type REST result handling uses). Bind is driven by whether the
// statement assigned an output variable, and the ImportMappingCall carries the
// RECEIVE mapping — by qualified name, not by UUID.
func webServiceResultHandlingToGen(a *microflows.WebServiceCallAction) element.Element {
	rh := newElem("Microflows$ResultHandling", "")
	addBool(rh, "Bind", a.OutputVariable != "")

	if a.ReceiveMappingID != "" {
		imc := newElem("Microflows$ImportMappingCall", "")
		addStr(imc, "Commit", "YesWithoutEvents")
		addStr(imc, "ContentType", "Json")
		addBool(imc, "ForceSingleOccurrence", false)
		addStr(imc, "ObjectHandlingBackup", "Create")
		addStr(imc, "ParameterVariableName", "")
		rng := newElem("Microflows$ConstantRange", "")
		addBool(rng, "SingleObject", true)
		addPart(imc, "Range", rng)
		// STORAGE NAME: ReturnValueMapping, not "Mapping" — the same key the
		// import-mapping call uses everywhere else in this engine.
		addStr(imc, "ReturnValueMapping", string(a.ReceiveMappingID))
		addPart(rh, "ImportMappingCall", imc)
	} else {
		addNull(rh, "ImportMappingCall")
	}

	addStr(rh, "ResultVariableName", a.OutputVariable)
	addPart(rh, "VariableType", newElem("DataTypes$VoidType", ""))
	return rh
}

// simpleRequestHandlingToGen builds the Microflows$SimpleRequestHandling used for
// both the body and the header handling.
func simpleRequestHandlingToGen() element.Element {
	rh := newElem("Microflows$SimpleRequestHandling", "")
	addStr(rh, "NullValueOption", "LeaveOutElement")
	addEmptyTypedList(rh, "ParameterMappings", 2)
	return rh
}

// webServiceLocalName is the service's local name — the part after the last dot
// of the qualified name. Mendix stores both: ImportedService is qualified,
// ServiceName is not.
func webServiceLocalName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// addEmptyTypedList writes an empty typed array carrying an explicit version
// marker.
//
// The marker is Mendix's array version and it is load-bearing — a wrong one is
// the class of defect that makes a project Studio Pro cannot open. It is written
// here rather than registered because the registry is keyed by $Type and these
// parent types are shared with writers that need different markers for the same
// field; see the note at the top of this file.
func addEmptyTypedList(b *element.Base, name string, marker int32) {
	p := property.NewPrimitive[bson.A](name, func(bson.Raw, string) bson.A { return nil })
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(bson.A{marker})
}

// addNull writes an explicit BSON null IN KEY POSITION for a child slot the
// document must carry but this call leaves unset.
//
// It cannot be a Part property: the encoder returns nil for a part with no
// child and the caller then skips the key entirely (encoder.go, `if val != nil`),
// so an unset part is an ABSENT key, not a null one. codec.TypeDefaults'
// NullFields does emit the key, but appends it after every property, and it is
// registered per $Type — Microflows$HttpConfiguration is shared with the REST
// writer, whose legacy counterpart writes CustomLocationTemplate only when a
// template exists (sdk/mpr writer_microflow_actions.go:688 vs :794). Registering
// it would add a null REST does not write. So the null is carried as a primitive
// value, which the driver marshals in place.
func addNull(b *element.Base, name string) {
	p := property.NewPrimitive[bson.Null](name, func(bson.Raw, string) bson.Null { return bson.Null{} })
	b.AddProperty(p, uint(len(b.Properties())))
	p.Set(bson.Null{})
}
