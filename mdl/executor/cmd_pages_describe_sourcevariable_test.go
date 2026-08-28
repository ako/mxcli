// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"
)

// upstream #977 (part A): a ContentParams entry bound to a page VARIABLE came
// back from DESCRIBE as `<unbound>`.
//
//	Variables: { $Label: String = '''hello''' }
//	dynamictext dtA (Content: 'Label is {1}', ContentParams: [{1} = $Label])
//
//	describe page …
//	  dynamictext dtA (Content: 'Label is {1}', ContentParams: [{1} = <unbound>])
//
// The report reads as a dropped write. It is not: the model is CORRECT. The
// stored parameter carries
//
//	SourceVariable: { $Type: "Forms$PageVariable", LocalVariable: "Label" }
//
// and the READER is what could not see it — it looked only at
// SourceVariable.PageParameter, found nothing, found no AttributeRef beside it,
// and fell through to "<unbound>". So nothing was lost until the describe output
// was executed again, at which point the binding really would be gone.
//
// The two describers disagreed about the same BSON: the pluggable one already
// read LocalVariable and SnippetParameter, the other did not. That is the actual
// defect — one shape, two readers — so they now share a single resolver rather
// than being fixed twice.

// clientTemplateParamValues is the exported behaviour under test in both
// describers: given one stored Forms$ClientTemplateParameter, what does MDL say?
func srcVarParam(kind, name string) map[string]any {
	sv := map[string]any{"$Type": "Forms$PageVariable"}
	switch kind {
	case "local":
		sv["LocalVariable"] = name
		sv["PageParameter"] = ""
		sv["SnippetParameter"] = ""
	case "page":
		sv["LocalVariable"] = ""
		sv["PageParameter"] = name
		sv["SnippetParameter"] = ""
	case "snippet":
		sv["LocalVariable"] = ""
		sv["PageParameter"] = ""
		sv["SnippetParameter"] = name
	}
	return map[string]any{"SourceVariable": sv}
}

func TestSourceVariableName_ReadsEveryBindingKind(t *testing.T) {
	cases := []struct {
		kind      string
		wantName  string
		wantLocal bool
	}{
		{"local", "Label", true},
		{"page", "Order", false},
		{"snippet", "Item", false},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			name, isLocal := sourceVariableBinding(srcVarParam(tc.kind, tc.wantName))
			if name != tc.wantName {
				t.Errorf("name = %q, want %q — a %s binding must not read as unbound", name, tc.wantName, tc.kind)
			}
			if isLocal != tc.wantLocal {
				t.Errorf("isLocal = %v, want %v", isLocal, tc.wantLocal)
			}
		})
	}
}

// Controls: an absent or empty SourceVariable really is no binding.
func TestSourceVariableName_NoBinding(t *testing.T) {
	for _, name := range []string{"missing", "empty", "allBlank"} {
		t.Run(name, func(t *testing.T) {
			var p map[string]any
			switch name {
			case "missing":
				p = map[string]any{}
			case "empty":
				p = map[string]any{"SourceVariable": nil}
			case "allBlank":
				p = srcVarParam("", "")
			}
			if got, _ := sourceVariableBinding(p); got != "" {
				t.Errorf("got %q, want no binding", got)
			}
		})
	}
}

// The end the report is about: a local-variable binding renders as `$Name` in
// BOTH describers. Before the fix the pluggable one said `$Label` and the other
// `<unbound>` — one stored shape, two readers, disagreeing.
func TestBothDescribers_RenderALocalVariableBinding(t *testing.T) {
	template := map[string]any{"Parameters": []any{srcVarParam("local", "Label")}}
	ctx, _ := newMockCtx(t)

	got := map[string][]string{
		"cmd_pages_describe_output":    extractClientTemplateParameters(ctx, map[string]any{"Content": template}, "Content"),
		"cmd_pages_describe_pluggable": extractTextTemplateParameters(ctx, template),
	}
	for label, values := range got {
		if len(values) != 1 {
			t.Fatalf("%s: got %d values (%v), want 1", label, len(values), values)
		}
		if values[0] != "$Label" {
			t.Errorf("%s: rendered %q, want %q", label, values[0], "$Label")
		}
		if strings.Contains(values[0], "unbound") {
			t.Errorf("%s: a stored LocalVariable binding must not read as unbound", label)
		}
	}
}

// A page-parameter binding with an AttributeRef keeps its `$Param.Attr` form —
// the case that already worked, and the one a careless merge would break.
func TestBothDescribers_KeepThePageParameterForm(t *testing.T) {
	param := srcVarParam("page", "Order")
	param["AttributeRef"] = map[string]any{"Attribute": "Sales.Order.Number"}
	template := map[string]any{"Parameters": []any{param}}
	ctx, _ := newMockCtx(t)

	for label, values := range map[string][]string{
		"cmd_pages_describe_output":    extractClientTemplateParameters(ctx, map[string]any{"Content": template}, "Content"),
		"cmd_pages_describe_pluggable": extractTextTemplateParameters(ctx, template),
	} {
		if len(values) != 1 || values[0] != "$Order.Number" {
			t.Errorf("%s: rendered %v, want [$Order.Number]", label, values)
		}
	}
}

// And a parameter with genuinely nothing on it still reads as unbound, in both.
func TestBothDescribers_StillReportATrulyUnboundParameter(t *testing.T) {
	template := map[string]any{"Parameters": []any{map[string]any{}}}
	ctx, _ := newMockCtx(t)

	for label, values := range map[string][]string{
		"cmd_pages_describe_output":    extractClientTemplateParameters(ctx, map[string]any{"Content": template}, "Content"),
		"cmd_pages_describe_pluggable": extractTextTemplateParameters(ctx, template),
	} {
		if len(values) != 1 || values[0] != "<unbound>" {
			t.Errorf("%s: rendered %v, want [<unbound>]", label, values)
		}
	}
}
