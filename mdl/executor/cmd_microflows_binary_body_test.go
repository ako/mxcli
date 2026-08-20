// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// Binary POST. Mendix models it on the microflow REST CALL activity as
// Microflows$BinaryRequestHandling plus an action-level
// RequestHandlingType of "Binary" — NOT on the consumed REST client document,
// whose three body types (Rest$JsonBody, Rest$StringBody,
// Rest$ImplicitMappingBody) have no binary form.
//
// Pinned against a Studio Pro-authored microflow (ako/TestApp, Mendix 11.13.0),
// which stores the FileDocument's Contents MEMBER as the expression:
//
//	"RequestHandling":     {"$Type": "Microflows$BinaryRequestHandling",
//	                        "Expression": "$FirstFileDoc/Contents"},
//	"RequestHandlingType": "Binary"
//
// mxcli could parse that shape and could neither write nor describe it, so a
// binary body survived a read but vanished from a DESCRIBE round trip.

// The builder maps `body binary <expr>` onto BinaryRequestHandling, carrying the
// expression through as source text — quoting it would send the path as a
// string literal instead of the bytes.
func TestBuildRestCall_BinaryBody(t *testing.T) {
	fb := &flowBuilder{
		posX: 100, posY: 100, spacing: HorizontalSpacing,
		varTypes: map[string]string{}, declaredVars: map[string]string{},
		measurer: &layoutMeasurer{},
	}
	fb.addRestCallAction(&ast.RestCallStmt{
		Method: ast.HttpMethodPost,
		URL:    &ast.LiteralExpr{Kind: ast.LiteralString, Value: "https://example.invalid/post"},
		Body: &ast.RestBody{
			Type:     ast.RestBodyBinary,
			Template: &ast.AttributePathExpr{Variable: "Doc", Path: []string{"Contents"}},
		},
		Result: ast.RestResult{Type: ast.RestResultResponse},
	})

	action, ok := fb.objects[0].(*microflows.ActionActivity).Action.(*microflows.RestCallAction)
	if !ok {
		t.Fatalf("built %T, want *microflows.RestCallAction", fb.objects[0].(*microflows.ActionActivity).Action)
	}
	bin, ok := action.RequestHandling.(*microflows.BinaryRequestHandling)
	if !ok {
		t.Fatalf("RequestHandling is %T, want *microflows.BinaryRequestHandling", action.RequestHandling)
	}
	if bin.Expression != "$Doc/Contents" {
		t.Errorf("Expression = %q, want %q — Studio Pro stores the Contents member, "+
			"and the text must not be quoted or the path is sent as a literal",
			bin.Expression, "$Doc/Contents")
	}
}

// DESCRIBE must emit the body, or the round trip silently drops the payload:
// before this, a Studio Pro binary POST described as a REST call with no body
// at all, and re-executing that output produced a request that sent nothing.
func TestFormatRestCallAction_BinaryBody(t *testing.T) {
	e := newTestExecutor()
	got := e.formatRestCallAction(&microflows.RestCallAction{
		HttpConfiguration: &microflows.HttpConfiguration{
			HttpMethod:       microflows.HttpMethodPost,
			LocationTemplate: "https://api.example.com/upload",
		},
		RequestHandling: &microflows.BinaryRequestHandling{Expression: "$Doc/Contents"},
		ResultHandling:  &microflows.ResultHandlingNone{},
	})
	if !strings.Contains(got, "body binary $Doc/Contents") {
		t.Errorf("describe must emit the binary body unquoted, got:\n%s", got)
	}
}
