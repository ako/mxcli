// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// mendixlabs/mxcli#1140. The reported page has two parameters and a Save button
// inside a data view bound to the first:
//
//	Action: nanoflow CustomModule.ACT_BufferDefinition_SaveEdit_NF(
//	    $Dto = $Dto, $BufferDefinition = $BufferDefinition)
//
// "$BufferDefinition = $BufferDefinition → NOT wired — Studio Pro shows CE1571:
// 'No argument has been selected for parameter BufferDefinition and no default is
// available.'"
//
// Both arguments were written the same way — as a text Expression — so the
// asymmetry is not in the writing: Studio Pro supplies a default for the one that
// happens to be the data view's object and reports the other, which is exactly
// what "and no default is available" says. Neither was bound.
func builderWithParams(params ...string) *pageBuilder {
	pb := &pageBuilder{paramScope: map[string]model.ID{}, localVariables: map[string]bool{}}
	for _, p := range params {
		pb.paramScope[p] = model.ID("id-" + p)
	}
	return pb
}

func TestClassifyFlowArgValue(t *testing.T) {
	pb := builderWithParams("Dto", "BufferDefinition")
	pb.localVariables["showStock"] = true

	for _, tc := range []struct {
		name, value, wantVar, wantKind string
	}{
		{"reported page parameter", "$BufferDefinition", "$BufferDefinition", "parameter"},
		{"the data view's own parameter", "$Dto", "$Dto", "parameter"},
		{"page variable", "$showStock", "$showStock", "local"},

		// Left as expressions, each for its own reason — see classifyFlowArgValue.
		{"current object", "$currentObject", "", ""},
		{"association path", "$Dto/Module.Assoc", "", ""},
		{"attribute path", "$Dto.Name", "", ""},
		{"literal", "true", "", ""},
		{"undeclared name", "$Whatever", "", ""},
		{"bare dollar", "$", "", ""},
		{"empty", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotVar, gotKind := pb.classifyFlowArgValue(tc.value)
			if gotVar != tc.wantVar || gotKind != tc.wantKind {
				t.Errorf("classifyFlowArgValue(%q) = (%q, %q), want (%q, %q)",
					tc.value, gotVar, gotKind, tc.wantVar, tc.wantKind)
			}
		})
	}
}

// Inside a snippet the same reference fills the SnippetParameter slot of the
// Forms$PageVariable — 58 of the 95 Studio Pro bindings measured are that one, so
// getting it wrong would be the common case, not the corner.
func TestClassifyFlowArgValueInSnippet(t *testing.T) {
	pb := builderWithParams("AuditTrailViewer")
	pb.isSnippet = true

	gotVar, gotKind := pb.classifyFlowArgValue("$AuditTrailViewer")
	if gotVar != "$AuditTrailViewer" || gotKind != "snippet" {
		t.Errorf("in a snippet: got (%q, %q), want ($AuditTrailViewer, snippet)", gotVar, gotKind)
	}
}

// A primitive page parameter is not in paramScope (only entity-typed ones are),
// so it stays an expression — matching the reference, where all six
// Expression-bound mappings are Boolean literals.
func TestClassifyFlowArgValueLeavesPrimitiveParameterAlone(t *testing.T) {
	pb := builderWithParams("Order") // "Qty", a primitive, is deliberately absent
	if _, kind := pb.classifyFlowArgValue("$Qty"); kind != "" {
		t.Errorf("primitive parameter classified as %q, want an expression", kind)
	}
	if _, kind := pb.classifyFlowArgValue("$Order"); kind != "parameter" {
		t.Fatalf("control: the entity-typed parameter is not classified either (%q) — "+
			"the test above proves nothing", kind)
	}
}

// The data-source path shares the classifier, so a parameterized microflow data
// source binds its page-parameter argument the same way a button does. Before
// #1140 these were two copies of one rule in two functions.
func TestDataSourceArgsBindPageParameterThroughVariable(t *testing.T) {
	pb := builderWithParams("Order")
	got := pb.flowArgsToParameterMappings([]ast.FlowArgV3{
		{Name: "Order", Value: "$Order"},
		{Name: "Limit", Value: "10"},
	})
	if len(got) != 2 {
		t.Fatalf("got %d mappings, want 2", len(got))
	}
	if got[0].VariableKind != "parameter" || got[0].Variable != "$Order" {
		t.Errorf("page-parameter arg = %+v, want Variable=$Order kind=parameter", got[0])
	}
	if got[1].VariableKind != "" || got[1].Expression != "10" {
		t.Errorf("literal arg = %+v, want Expression=10 and no kind", got[1])
	}
}
