// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// systemFileDocument is the base type a REST file-document result must specialize.
const systemFileDocument = "System.FileDocument"

// checkRestFileDocumentResult validates `rest call … returns Module.Entity` — MDL064.
//
// Two things are checked, both measured against mxbuild 11.6.6:
//
//  1. The base type is rejected by Mendix itself: storing a response into
//     System.FileDocument builds as CE0362 "System entity 'System.FileDocument'
//     is not allowed as a return type." A specialization is mandatory, and one
//     is always available because CE1540 lists FileDocument among the four
//     System entities that MAY be specialized (User, FileDocument, Image,
//     Paging) — which is also why there is no equivalent form for an
//     HttpResponse result: HttpResponse cannot be specialized at all, so
//     `returns response` already names the only type it can have.
//
//  2. The name must be qualified. `returns MyFile` is indistinguishable from a
//     typo, and an unqualified entity reference is MDL008 everywhere else.
//
// Whether the named entity really specializes FileDocument needs the project,
// so it is left to `check --references`; this rule is the part that works
// without one.
func (v *microflowValidator) checkRestFileDocumentResult(stmt *ast.RestCallStmt) {
	if stmt.Result.Type != ast.RestResultFileDocument {
		return
	}
	entity := stmt.Result.ResultEntity

	if entity.Module == "" {
		v.addViolation("MDL064", linter.SeverityError,
			fmt.Sprintf("rest call 'returns %s': the file document type needs a module prefix",
				entity.Name),
			fmt.Sprintf("Write the qualified name, e.g. 'MyModule.%s'", entity.Name))
		return
	}

	if entity.String() == systemFileDocument {
		v.addViolation("MDL064", linter.SeverityError,
			"rest call 'returns System.FileDocument': Mendix does not allow the base "+
				"System.FileDocument as a REST result type — mxbuild rejects it with CE0362 "+
				"\"System entity 'System.FileDocument' is not allowed as a return type.\"",
			"Create an entity that specializes it (create persistent entity MyModule.MyFile "+
				"extends System.FileDocument) and return that instead")
	}
}
