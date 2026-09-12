// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// A SOAP call stores TWO names for the service it calls, and they are not the
// same string:
//
//	ImportedService  "Clients.OrderSoapClient"   the DOCUMENT, qualified
//	ServiceName      "OrdersWS"                  the WSDL <wsdl:service name=…>
//
// Both engines derived the second from the first by taking the part after the
// last dot, which is right only when someone happened to name the document after
// the service. Mendix resolves the operation WITHIN the named service, so when
// they differ the call does not merely look odd — it fails to validate. Measured
// on Mendix 11.14.0 against ako/TestApp, whose baseline is 0 errors:
//
//	[CE0386] "Operation 'GetOrder' does not exist in consumed web service
//	          'Clients.OrderSoapClient'."
//
// GetOrder does exist. Mendix looked for it inside a service called
// "OrderSoapClient", which the WSDL does not define.
//
// The real name is in the imported service document, in structured form — no
// WSDL parsing needed. `Description.Services[]` holds WebServices$ServiceInfoImpl
// entries, each with a Name and an Operations list of
// WebServices$OperationInfoImpl.

// importedServiceType is the stored $Type of an imported (consumed) SOAP
// service, in FULL — including the `Impl` suffix.
//
// Two things measured on ako/TestApp, both of which cost a debugging round:
//
//   - ListRawUnitsByType matches the type EXACTLY, despite its parameter being
//     called typePrefix. "WebServices$ImportedServiceImpl" returns the document;
//     "WebServices$ImportedService" and "WebServices" both return nothing.
//   - The type is `ImportedServiceImpl`, not `ImportedWebService`. The name
//     modelsdk/gen and generated/metamodel both use — WebServices$ImportedWebService
//     — is the SDK name; nothing is stored under it. A DESCRIBE-side resolver
//     asked for that one and therefore matched nothing, which went unnoticed
//     because its fallback was already the right answer (see
//     TestFormatAction_WebServiceCallRendersStoredQualifiedNames); it has since
//     been removed.
const importedServiceType = "WebServices$ImportedServiceImpl"

// resolveWebServiceName returns the WSDL service name for the imported service
// document named by qualifiedName, and the operation names it declares.
//
// It returns "" when the answer cannot be established — an unresolvable
// document, a backend that cannot list raw units, a document whose shape does
// not match, or a name that matches more than one document. The caller then
// falls back to the old derivation: a wrong ServiceName is no worse than the one
// shipping today, and an invented one would be.
//
// The match is on the document's BARE name. A raw unit carries its container id
// rather than a module name, and resolving that needs the ExecContext the flow
// builder does not hold; ambiguity is refused instead of resolved, which costs a
// fallback in the rare two-modules-same-name case and never picks the wrong
// service.
func resolveWebServiceName(b backend.FullBackend, qualifiedName, operationName string) string {
	matched := findImportedServiceDoc(b, qualifiedName)
	if matched == nil {
		return ""
	}
	return serviceNameFromImportedService(matched, operationName)
}

// findImportedServiceDoc returns the contents of the imported service document
// named by qualifiedName, or nil when it cannot be identified unambiguously.
func findImportedServiceDoc(b backend.FullBackend, qualifiedName string) []byte {
	if b == nil || qualifiedName == "" {
		return nil
	}
	units, err := b.ListRawUnitsByType(importedServiceType)
	if err != nil || len(units) == 0 {
		return nil
	}
	_, bare, ok := strings.Cut(qualifiedName, ".")
	if !ok || bare == "" {
		bare = qualifiedName
	}

	var matched []byte
	for _, unit := range units {
		if unit == nil || len(unit.Contents) == 0 {
			continue
		}
		if !strings.EqualFold(rawUnitName(unit.Contents), bare) {
			continue
		}
		if matched != nil {
			return nil // ambiguous — two documents of this name
		}
		matched = unit.Contents
	}
	return matched
}

// serviceNameFromImportedService reads Description.Services[] and returns the
// name of the service to call.
//
// A WSDL may define more than one service, so the operation decides: the service
// DECLARING it is the one Mendix resolves against. With one service the operation
// is not consulted, and with none — or an operation no service declares — the
// answer is "" rather than a guess, since guessing reproduces CE0386 with a
// different name in it.
func serviceNameFromImportedService(contents []byte, operationName string) string {
	var doc map[string]any
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return ""
	}
	services := typedArrayElements(docLookup(doc["Description"], "Services"))
	if len(services) == 0 {
		return ""
	}

	names := make([]string, 0, len(services))
	for _, svc := range services {
		name, _ := docLookup(svc, "Name").(string)
		if name == "" {
			continue
		}
		names = append(names, name)
		if operationName != "" && serviceDeclaresOperation(svc, operationName) {
			return name
		}
	}
	if len(names) == 1 {
		return names[0]
	}
	return ""
}

// resolveWebServiceOperationElement returns the operation's stored
// RequestBodyElementName — `"http://www.example.com/:GetOrder"` for
// ako/TestApp's GetOrder — which is the prefix of every argument's ParameterPath.
//
// "" when it cannot be established, and the caller then REFUSES to write the
// arguments rather than inventing a path. That is stricter than the other
// resolvers here, which fall back: falling back is safe when the alternative is
// the value that ships today, and there is no such value for a path that has
// never been written.
func resolveWebServiceOperationElement(b backend.FullBackend, qualifiedName, operationName string) string {
	if operationName == "" {
		return ""
	}
	matched := findImportedServiceDoc(b, qualifiedName)
	if matched == nil {
		return ""
	}
	var doc map[string]any
	if err := bson.Unmarshal(matched, &doc); err != nil {
		return ""
	}
	for _, svc := range typedArrayElements(docLookup(doc["Description"], "Services")) {
		for _, op := range typedArrayElements(docLookup(svc, "Operations")) {
			if name, _ := docLookup(op, "Name").(string); !strings.EqualFold(name, operationName) {
				continue
			}
			element, _ := docLookup(op, "RequestBodyElementName").(string)
			return element
		}
	}
	return ""
}

// webServiceParameterPath builds the stored ParameterPath for one argument.
//
// Measured on ako/TestApp (11.14.0): the operation element
// `http://www.example.com/:GetOrder` and the parameter `OrderId` are stored as
//
//	http%3A//www.example.com/:GetOrder|OrderId
//
// so the element's namespace and local name keep the `:` BETWEEN them and the
// `|` before the parameter, while a `:` INSIDE a segment is percent-encoded. The
// element name splits on its LAST colon, since the namespace is a URI and
// carries colons of its own.
//
// "" when the path cannot be built, which the caller turns into a refusal.
func webServiceParameterPath(operationElement, parameterName string) string {
	if operationElement == "" || parameterName == "" {
		return ""
	}
	namespace, local := "", operationElement
	if i := strings.LastIndex(operationElement, ":"); i >= 0 {
		namespace, local = operationElement[:i], operationElement[i+1:]
	}
	ns, okNS := escapeParameterPathSegment(namespace)
	lo, okLO := escapeParameterPathSegment(local)
	pn, okPN := escapeParameterPathSegment(parameterName)
	if !okNS || !okLO || !okPN {
		return ""
	}
	if namespace == "" {
		return lo + "|" + pn
	}
	return ns + ":" + lo + "|" + pn
}

// escapeParameterPathSegment percent-encodes the two characters that would
// otherwise be read as path structure.
//
// Only `:` has a reference document behind it; `|` is escaped by the same
// reasoning and has never been observed in a namespace. A segment already
// containing a `%` is REFUSED (ok=false) rather than encoded or passed through:
// whether Mendix escapes it as %25 is unmeasured, and both answers produce a
// path that silently addresses the wrong parameter.
func escapeParameterPathSegment(s string) (string, bool) {
	if strings.Contains(s, "%") {
		return "", false
	}
	s = strings.ReplaceAll(s, ":", "%3A")
	s = strings.ReplaceAll(s, "|", "%7C")
	return s, true
}

// serviceDeclaresOperation reports whether a WebServices$ServiceInfoImpl lists an
// operation of this name.
func serviceDeclaresOperation(svc any, operationName string) bool {
	for _, op := range typedArrayElements(docLookup(svc, "Operations")) {
		if name, _ := docLookup(op, "Name").(string); strings.EqualFold(name, operationName) {
			return true
		}
	}
	return false
}

// typedArrayElements drops the leading version marker from a Mendix typed array
// and returns the elements. A value that is not an array yields none.
func typedArrayElements(v any) []any {
	var arr []any
	switch a := v.(type) {
	case bson.A:
		arr = a
	case []any:
		arr = a
	default:
		return nil
	}
	if len(arr) == 0 {
		return nil
	}
	switch arr[0].(type) {
	case int32, int64, int:
		return arr[1:]
	}
	return arr
}

// docLookup reads a key from a BSON sub-document whichever way the driver
// decoded it.
//
// This is not defensive dressing: a unit unmarshalled into map[string]any nests
// its sub-documents as map[string]any, while the same bytes decoded into a
// bson.D nest as bson.D. Asserting only the second silently found nothing, and
// "found nothing" here is indistinguishable from "no such service" — it fell
// back to the wrong name instead of failing.
func docLookup(v any, key string) any {
	switch d := v.(type) {
	case map[string]any:
		return d[key]
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
	}
	return nil
}

// importMappingType is the stored $Type of an import mapping document. As with
// importedServiceType, this is matched EXACTLY — and note it is
// `ImportMappings$ImportMapping`, not the `Mappings$…` prefix its child elements
// use (Mappings$ObjectMappingElement also appears in these documents).
const importMappingType = "ImportMappings$ImportMapping"

// resolveImportMappingEntity returns the qualified entity an import mapping
// produces — the Entity of its root ObjectMappingElement.
//
// It is what Mendix stores as the SOAP call's result VariableType. Writing
// DataTypes$VoidType instead (which both engines did) tells Mendix the call
// returns nothing, and assigning that to a variable is two errors at once,
// measured on 11.14.0 against ako/TestApp:
//
//	[CE0243] "The mapping used to return a value of type 'Nothing', but now
//	          returns a value of type 'Clients.Order'."
//	[CE0366] "Cannot store in variable when there is no return value."
//
// "" when it cannot be established, and the writers then keep VoidType — which
// is wrong, but is what ships today, so an unresolvable mapping is no worse off.
func resolveImportMappingEntity(b backend.FullBackend, qualifiedName string) string {
	if b == nil || qualifiedName == "" {
		return ""
	}
	units, err := b.ListRawUnitsByType(importMappingType)
	if err != nil || len(units) == 0 {
		return ""
	}
	_, bare, ok := strings.Cut(qualifiedName, ".")
	if !ok || bare == "" {
		bare = qualifiedName
	}

	var matched []byte
	for _, unit := range units {
		if unit == nil || len(unit.Contents) == 0 {
			continue
		}
		if !strings.EqualFold(rawUnitName(unit.Contents), bare) {
			continue
		}
		if matched != nil {
			return "" // ambiguous — refuse, as with the service lookup
		}
		matched = unit.Contents
	}
	if matched == nil {
		return ""
	}
	return rootMappingEntity(matched)
}

// rootMappingEntity reads Elements[] and returns the first object mapping
// element's Entity.
//
// The root is the only element whose entity the CALL is typed on; the children
// are value mappings and nested objects, which belong to the mapping's own
// structure rather than to the result.
func rootMappingEntity(contents []byte) string {
	var doc map[string]any
	if err := bson.Unmarshal(contents, &doc); err != nil {
		return ""
	}
	for _, el := range typedArrayElements(doc["Elements"]) {
		entity, _ := docLookup(el, "Entity").(string)
		if entity != "" {
			return entity
		}
	}
	return ""
}
