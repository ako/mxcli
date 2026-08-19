// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `Body: file from $Doc` was written as a Rest$StringBody whose value is the
// EXPRESSION TEXT, so the request sent the four bytes `$Doc` instead of the
// document. Measured against httpbingo: `Content-Length: 4` for an 8090-byte
// PNG, HTTP 200, `mxcli check` clean and `mx check` 0 errors — the worst shape
// a defect can take.
//
// There is no better type to write: Mendix's 11.13 metamodel has exactly three
// request-body types (Rest$JsonBody, Rest$StringBody, Rest$ImplicitMappingBody)
// and none of them is binary. So the fix is to REFUSE the clause, the way
// MDL-REST01 refuses a mapping document in an inline mapping, rather than
// degrade it to something that looks like it works — and point at the
// microflow REST CALL activity, which does carry a binary body.
func TestBuildRestClientOperation_RefusesFileRequestBody(t *testing.T) {
	_, err := buildRestClientOperation(&ast.RestOperationDef{
		Name:         "Upload",
		BodyType:     "file",
		BodyVariable: "$Doc",
	})
	if err == nil {
		t.Fatal("a file request body must be refused: writing it sends the expression text, not the file")
	}
	// The message must point at the route that WORKS. Binary POST is expressible
	// on the microflow REST CALL activity (Microflows$BinaryRequestHandling);
	// only the consumed client document has nowhere to put it.
	for _, want := range []string{"binary", "$Doc", "rest call post", "body binary"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got:\n%v", want, err)
		}
	}
}

// The check-time pass must catch it too, so `mxcli check` refuses the script
// before anything is written — same function behind both, as with MDL-REST01.
func TestValidateRestClientMappings_ReportsFileRequestBody(t *testing.T) {
	prog := &ast.Program{Statements: []ast.Statement{
		&ast.CreateRestClientStmt{
			Name: ast.QualifiedName{Module: "M", Name: "RC"},
			Operations: []*ast.RestOperationDef{
				{Name: "Upload", BodyType: "file", BodyVariable: "$Doc"},
			},
		},
	}}
	violations := ValidateRestClientMappings(prog)
	if len(violations) == 0 {
		t.Fatal("check must report a file request body")
	}
	v := violations[0]
	if v.RuleID != "MDL-REST02" {
		t.Errorf("RuleID = %q, want MDL-REST02", v.RuleID)
	}
	if !strings.Contains(v.Message, "Upload") {
		t.Errorf("message should name the operation, got %q", v.Message)
	}
}

// The controls. Refusing too much would break every working script, and the
// RESPONSE side is a separate matter — downloading a file works, and only the
// CHANGE on the result is broken (a different defect, not this one).
func TestBuildRestClientOperation_FileBodyRefusalIsNarrow(t *testing.T) {
	for _, tc := range []struct {
		name string
		def  *ast.RestOperationDef
	}{
		{"json body", &ast.RestOperationDef{Name: "A", BodyType: "json", BodyVariable: "$x"}},
		{"template body", &ast.RestOperationDef{Name: "B", BodyType: "template", BodyVariable: "'hi'"}},
		{"file RESPONSE is not a request body", &ast.RestOperationDef{
			Name: "C", ResponseType: "file", ResponseVariable: "$Doc",
		}},
		{"no body at all", &ast.RestOperationDef{Name: "D"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := buildRestClientOperation(tc.def); err != nil {
				t.Errorf("must be accepted, got: %v", err)
			}
		})
	}
}
