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
// against them legacy was wrong in six places. FOUR are now fixed in both
// engines, each verified by writing a call against TestApp's real service and
// running mx check — the errors came one at a time, each hidden by the one
// before it (StorageLoadException -> CE0386 -> CE0243+CE0366 -> CE0178):
//
//   - ServiceName is the WSDL SERVICE name ("OrdersWS"), not the local part of
//     the imported service's qualified name ("OrderSoapClient"). Deriving it
//     gave CE0386 "Operation 'GetOrder' does not exist in consumed web service".
//     Now resolved from the document by the executor (resolveWebServiceName),
//     with the old derivation kept as a fallback.
//   - ImportMappingCall.ContentType is "Xml", not the hardcoded "Json".
//   - ReturnValueMapping is the mapping's QUALIFIED NAME. The executor used to
//     resolve it to the mapping's unit `$ID`, which made the project impossible
//     to LOAD — mx check stopped at a StorageLoadException before validation.
//     That one only fired when the reference RESOLVED, so the only SOAP fixture
//     (whose mappings deliberately do not exist) never triggered it.
//   - VariableType is the result's REAL type, read off the receive mapping's
//     root ObjectMappingElement (resolveImportMappingEntity). DataTypes$VoidType
//     said the call returns nothing, which contradicted the mapping (CE0243) and
//     made assigning the result its own error (CE0366). VoidType stays the
//     fallback for a mapping that cannot be resolved.
//
// Two more have since been fixed, and they turned out to be one thing:
// RequestBodyHandling is a polymorphic child holding EITHER the operation's
// arguments or an export mapping, so what looked like two gaps was the two
// branches of one property.
//
//   - Operation ARGUMENTS are Microflows$WebServiceOperationSimpleParameterMapping
//     entries inside a SimpleRequestHandling, keyed by an escaped ParameterPath
//     ("http%3A//www.example.com/:GetOrder|OrderId" — the operation's
//     RequestBodyElementName, escaped, plus "|" plus the parameter). Writing that
//     list empty gave CE0178 "Body parameter mapping needs to be refreshed".
//   - A SEND MAPPING is a Microflows$MappingRequestHandling, NOT the
//     Mendix$AdvancedRequestHandling legacy's comment named (that type appears in
//     none of the three reference documents). Writing SimpleRequestHandling
//     regardless — which both engines did — dropped the mapping silently and gave
//     CE0369 "Cannot use simple request body, as the operation's body is complex".
//
// One remains:
//
//   - Range.SingleObject follows the operation's cardinality; both reference
//     calls write false where both engines write true. No error has been
//     measured from it, so it is left until one is — the reference roots carry
//     MaxOccurs 1 while the calls carry SingleObject false, so it is NOT simply
//     the mapping's cardinality and would be a guess today.
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
//   - MappingRequestHandling's two keys are written as RAW strings rather than
//     through the gen accessors. gen binds them as `Mapping` and
//     `MappingArgumentVariableName`; Studio Pro stores `MappingId` and
//     `MappingVariableName` (modelsdk/gen/keyaudit_test.go). A document written
//     through gen's names is one mxbuild tolerates and Studio Pro cannot open.
//
// Every null the document carries is written IN KEY POSITION rather than through
// NullFields, for the same reason and with the same consequence — see addNull.

func init() {
	// A populated ParameterMappings list leads with marker 2 (measured on all
	// three ako/TestApp calls). The codec's default is 3, so WITHOUT this the
	// arguments would serialize under the wrong array version — the class of
	// defect that makes a project Studio Pro cannot open, and one mxbuild does
	// not catch.
	//
	// Unlike the markers the note above keeps out of the registry, this child
	// type is SOAP-only: nothing else writes a
	// WebServiceOperationSimpleParameterMapping, so there is no writer for a
	// global registration to disturb.
	codec.RegisterListMarker("Microflows$WebServiceOperationSimpleParameterMapping", 2)
}

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
	addPart(g, "RequestBodyHandling", webServiceRequestBodyToGen(a))
	// The HEADER handling is always Simple and always empty: MDL cannot author
	// SOAP headers, and all three reference calls carry the bare form.
	addPart(g, "RequestHeaderHandling", simpleRequestHandlingToGen(nil))
	addStr(g, "RequestProxyType", "DefaultProxy")
	addStr(g, "ServiceName", webServiceName(a))
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
		// Xml, not Json: a SOAP response IS XML, and Studio Pro writes "Xml" in
		// both reference calls that carry an import mapping (ako/TestApp,
		// Clients.GetOrders and GetCustomerOrders, 11.14.0).
		addStr(imc, "ContentType", "Xml")
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
	addPart(rh, "VariableType", webServiceVariableType(a))
	return rh
}

// webServiceVariableType is the type of the value the call returns: the entity
// the receive mapping produces. VoidType — what both engines wrote
// unconditionally — says the call returns NOTHING, which contradicts the mapping
// (CE0243 "the mapping used to return a value of type 'Nothing'") and makes
// assigning the result an error in its own right (CE0366 "cannot store in
// variable when there is no return value"). Kept as the fallback for a mapping
// that could not be resolved.
func webServiceVariableType(a *microflows.WebServiceCallAction) element.Element {
	if a.ResultEntity == "" {
		return newElem("DataTypes$VoidType", "")
	}
	vt := newElem("DataTypes$ObjectType", "")
	addStr(vt, "Entity", a.ResultEntity)
	return vt
}

// webServiceRequestBodyToGen builds RequestBodyHandling — the polymorphic child
// that carries EITHER the operation's arguments or an export mapping.
//
// The executor refuses a statement asking for both (MDL-SOAP01), so the branch
// here is a plain else: a send mapping wins only because it cannot coexist with
// arguments, not by precedence.
func webServiceRequestBodyToGen(a *microflows.WebServiceCallAction) element.Element {
	if a.SendMappingID != "" {
		return mappingRequestHandlingToGen(a)
	}
	return simpleRequestHandlingToGen(a.Arguments)
}

// mappingRequestHandlingToGen builds Microflows$MappingRequestHandling — a SOAP
// request body produced by an export mapping.
//
// Both name keys are written RAW. gen binds them as `Mapping` and
// `MappingArgumentVariableName`, which are the SDK names; Studio Pro stores
// `MappingId` and `MappingVariableName`, and both mismatches are listed in
// modelsdk/gen/keyaudit_test.go. mxbuild tolerates the wrong spellings, so the
// symptom of getting this wrong is not a build error — it is a document Studio
// Pro cannot open.
func mappingRequestHandlingToGen(a *microflows.WebServiceCallAction) element.Element {
	rh := newElem("Microflows$MappingRequestHandling", "")
	// "Json" is what Studio Pro wrote on the one reference document
	// (ako/TestApp Clients.SaveOrder) — surprising on an XML protocol, which is
	// why a stored value is carried through rather than normalised, and why the
	// default is the observed one rather than the plausible one.
	addStr(rh, "ContentType", orDefault(a.SendMappingContentType, "Json"))
	addStr(rh, "MappingId", string(a.SendMappingID))
	addStr(rh, "MappingVariableName", a.SendMappingVariable)
	return rh
}

// simpleRequestHandlingToGen builds the Microflows$SimpleRequestHandling that
// carries the operation's arguments. Passing nil gives the empty form, which is
// what the header handling and an argument-less call use.
func simpleRequestHandlingToGen(args []microflows.WebServiceArgument) element.Element {
	rh := newElem("Microflows$SimpleRequestHandling", "")
	addStr(rh, "NullValueOption", "LeaveOutElement")
	if len(args) == 0 {
		addEmptyTypedList(rh, "ParameterMappings", 2)
		return rh
	}
	children := make([]element.Element, 0, len(args))
	for _, arg := range args {
		pm := newElem("Microflows$WebServiceOperationSimpleParameterMapping", "")
		addStr(pm, "Argument", arg.Expression)
		addBool(pm, "IsChecked", arg.Checked)
		// ParameterName is "" in both reference mappings. What fills it is
		// unmeasured — plausibly an RPC-style binding, which neither reference
		// operation uses — so it is written empty rather than guessed at from
		// the argument's own name.
		addStr(pm, "ParameterName", "")
		addStr(pm, "ParameterPath", arg.Path)
		children = append(children, pm)
	}
	addPartList(rh, "ParameterMappings", children)
	return rh
}

// webServiceName is the WSDL <wsdl:service name=…> Mendix resolves the operation
// within. The executor reads it off the imported service document; the fallback
// below is what BOTH engines used to do unconditionally, and it is right only
// when the document happens to be named after the service — otherwise Mendix
// reports CE0386 "Operation … does not exist in consumed web service …". Kept as
// a fallback rather than an error because it is what ships today, and a call
// against an unresolvable service is no worse than before.
func webServiceName(a *microflows.WebServiceCallAction) string {
	if a.ServiceName != "" {
		return a.ServiceName
	}
	return webServiceLocalName(string(a.ServiceID))
}

// webServiceLocalName is the part after the last dot of a qualified name.
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
