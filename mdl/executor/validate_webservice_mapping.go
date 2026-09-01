// SPDX-License-Identifier: Apache-2.0

// Guard-don't-drop for a mapping sourced from an imported web service (SOAP).
//
// A mapping can be sourced four ways — a JSON structure, an XML schema, a
// message definition, or an IMPORTED WEB SERVICE. MDL can spell the first
// three. The fourth is a WSDL binding (which service, which operation, which
// root element), and a CREATE OR REPLACE/MODIFY rebuilds the mapping from the
// statement, so anything the script does not restate is gone.
//
// Measured on mxbuild 11.13.0 against a mapping carrying the four properties a
// WSDL import writes. describe → exec — which is how a document is copied —
// left ServiceName and OperationName blank and removed ImportedWebService and
// RootElementName outright:
//
//	[error] [CE6896] "A mapping must have exactly one schema source."
//	[error] [CE0270] "No root element could be found in the schema."
//
// So a working SOAP integration became an unbuildable one, and the diff blames
// the statement the user ran rather than the source they never mentioned. That
// is the same class as the queued-call refusal (ADR-0005) and worse than
// ako/mxcli#259's dangling reference, which at least wrote a bad name through
// rather than deleting a good one.
//
// This REFUSES rather than preserving. Carrying the binding through a rebuild
// would mean claiming the rest of the document survives it too, and mxcli
// cannot check that: a SOAP mapping's elements resolve against the WSDL's
// inline schema entries, which mxcli does not read. Refusing is the honest
// answer until `with web service …` exists.
package executor

import (
	"fmt"

	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// checkNoWebServiceSource refuses a rewrite of a mapping whose stored source is
// an imported web service. kind is "import" or "export".
func checkNoWebServiceSource(kind, qualifiedName string, src model.WebServiceMappingSource) error {
	if !src.IsSet() {
		return nil
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"%s mapping %s is sourced from the imported web service %s%s, which MDL cannot "+
			"express — rewriting it would drop the binding and leave the mapping with no "+
			"schema source at all (CE6896). Edit it in Studio Pro, or drop and re-import "+
			"the web service",
		kind, qualifiedName, src.ImportedWebService, webServiceDetail(src)))
}

// webServiceDetail names the part of the service the mapping covers, so the
// refusal says what would have been lost rather than only that something would.
func webServiceDetail(src model.WebServiceMappingSource) string {
	switch {
	case src.ServiceName != "" && src.OperationName != "":
		return fmt.Sprintf(" (service %s, operation %s)", src.ServiceName, src.OperationName)
	case src.OperationName != "":
		return fmt.Sprintf(" (operation %s)", src.OperationName)
	case src.ServiceName != "":
		return fmt.Sprintf(" (service %s)", src.ServiceName)
	}
	return ""
}
