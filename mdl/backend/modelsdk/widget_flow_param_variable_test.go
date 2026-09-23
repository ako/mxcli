// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	bsonv1 "go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/sdk/pages"
)

// mendixlabs/mxcli#1140 — a page parameter passed as an argument to a nanoflow or
// microflow button action was written as a text Expression ("$BufferDefinition").
// Studio Pro binds such an argument through Variable → Forms$PageVariable and
// reports CE1571 "No argument has been selected for parameter 'X' and no default
// is available" for the Expression form. mxbuild accepts it at 0 errors, so the
// build is not a safety net here — the error appears only on opening the page.
//
// Measured on Workflow Commons 4.11.0 (Studio Pro-authored, 42 pages + 84
// snippets): of 101 Forms$MicroflowParameterMapping / Forms$NanoflowParameterMapping
// elements, 95 bind through Variable → Forms$PageVariable and 6 through Expression
// — and all 6 of those are Boolean literals ("true\n", "false\n"). A $-prefixed
// Expression, the only form mxcli emitted, occurs zero times.
//
// The PageVariable slot follows what the name refers to: PageParameter (20),
// SnippetParameter (58) and Widget (17, a grid's selection — no MDL syntax).

// paramVariable returns the Variable sub-document of a parameter mapping, or nil.
func paramVariable(pm bsonv1.D) bsonv1.D {
	v, ok := docGet(pm, "Variable").(bsonv1.D)
	if !ok {
		return nil
	}
	return v
}

func TestNanoflowActionBindsPageParameterThroughVariable(t *testing.T) {
	a := &pages.NanoflowClientAction{
		NanoflowName: "CustomModule.ACT_BufferDefinition_SaveEdit_NF",
		ParameterMappings: []*pages.NanoflowParameterMapping{
			{ParameterName: "BufferDefinition", Variable: "$BufferDefinition", VariableKind: "parameter"},
		},
	}
	pm := firstParamMapping(t, encodeAction(t, a))

	if got := docGet(pm, "Parameter"); got != "CustomModule.ACT_BufferDefinition_SaveEdit_NF.BufferDefinition" {
		t.Errorf("Parameter = %v", got)
	}
	v := paramVariable(pm)
	if v == nil {
		t.Fatalf("mapping has no Variable — the argument is unbound and Studio Pro reports CE1571.\n"+
			"Expression = %v", docGet(pm, "Expression"))
	}
	if got := docGet(v, "$Type"); got != "Forms$PageVariable" {
		t.Errorf("Variable $Type = %v, want Forms$PageVariable", got)
	}
	if got := docGet(v, "PageParameter"); got != "BufferDefinition" {
		t.Errorf("Variable.PageParameter = %v, want BufferDefinition", got)
	}
	if got := docGet(pm, "Expression"); got == "$BufferDefinition" {
		t.Errorf("Expression = %q — the $-reference must not be written as an expression", got)
	}
}

func TestMicroflowActionBindsPageParameterThroughVariable(t *testing.T) {
	a := &pages.MicroflowClientAction{
		MicroflowName: "CustomModule.ACT_Save",
		ParameterMappings: []*pages.MicroflowParameterMapping{
			{ParameterName: "BufferDefinition", Variable: "$BufferDefinition", VariableKind: "parameter"},
		},
	}
	settings, ok := docGet(encodeAction(t, a), "MicroflowSettings").(bsonv1.D)
	if !ok {
		t.Fatalf("MicroflowSettings missing")
	}
	pm := firstParamMapping(t, settings)

	v := paramVariable(pm)
	if v == nil {
		t.Fatalf("mapping has no Variable — CE1571. Expression = %v", docGet(pm, "Expression"))
	}
	if got := docGet(v, "PageParameter"); got != "BufferDefinition" {
		t.Errorf("Variable.PageParameter = %v, want BufferDefinition", got)
	}
}

// A snippet parameter fills a different slot of the same Forms$PageVariable —
// 58 of the 95 Studio Pro bindings are this one.
func TestFlowActionBindsSnippetParameterThroughVariable(t *testing.T) {
	a := &pages.NanoflowClientAction{
		NanoflowName: "WorkflowCommons.ACT_AuditTrailViewer_Minimal",
		ParameterMappings: []*pages.NanoflowParameterMapping{
			{ParameterName: "AuditTrailViewer", Variable: "$AuditTrailViewer", VariableKind: "snippet"},
		},
	}
	v := paramVariable(firstParamMapping(t, encodeAction(t, a)))
	if v == nil {
		t.Fatal("mapping has no Variable")
	}
	if got := docGet(v, "SnippetParameter"); got != "AuditTrailViewer" {
		t.Errorf("Variable.SnippetParameter = %v, want AuditTrailViewer", got)
	}
	if got := docGet(v, "PageParameter"); got == "AuditTrailViewer" {
		t.Errorf("a snippet parameter was written into the PageParameter slot")
	}
}

// CONTROL — an argument that is NOT a page-variable reference must still be
// written as an Expression, with no Variable. All six Expression-bound mappings
// in the reference are literals of this shape, and a fix that routed everything
// through Variable would pass the tests above and break them.
func TestFlowActionKeepsLiteralArgumentAsExpression(t *testing.T) {
	a := &pages.MicroflowClientAction{
		MicroflowName: "WorkflowCommons.ACT_TaskAssignmentHelper_Reassign",
		ParameterMappings: []*pages.MicroflowParameterMapping{
			{ParameterName: "KeepAsTargetUser", Expression: "true"},
		},
	}
	settings, _ := docGet(encodeAction(t, a), "MicroflowSettings").(bsonv1.D)
	pm := firstParamMapping(t, settings)

	if got := docGet(pm, "Expression"); got != "true" {
		t.Errorf("Expression = %v, want true", got)
	}
	if v := paramVariable(pm); v != nil {
		t.Errorf("a literal argument was given a Variable: %v", v)
	}
}

// CONTROL — $currentObject is deliberately left as an Expression. No Studio Pro
// reference for the bare form was measured, and the enclosing-context binding is
// what mxcli already relies on elsewhere; changing it without evidence would put
// the working case at risk (#1140 is about a NAMED page parameter).
func TestFlowActionLeavesCurrentObjectAsExpression(t *testing.T) {
	a := &pages.NanoflowClientAction{
		NanoflowName: "M.NF",
		ParameterMappings: []*pages.NanoflowParameterMapping{
			{ParameterName: "Obj", Variable: "$currentObject"},
		},
	}
	pm := firstParamMapping(t, encodeAction(t, a))
	if got := docGet(pm, "Expression"); got != "$currentObject" {
		t.Errorf("Expression = %v, want $currentObject", got)
	}
	if v := paramVariable(pm); v != nil {
		t.Errorf("$currentObject was bound through Variable: %v", v)
	}
}
