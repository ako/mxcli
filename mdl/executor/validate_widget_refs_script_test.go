// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// ledger #138: `mxcli check` printed "References to objects created within the
// script are skipped" and then failed on exactly that.
//
//	✓ Syntax OK (4 statements)
//	(Note: References to objects created within the script are skipped)
//	Reference errors:
//	  statement 4: snippet 'Ledger.SNIPPET_ThemeBar' has reference errors:
//	  - nanoflow not found: Ledger.ACT_SkinLedgerPaper
//	  - nanoflow not found: Ledger.ACT_SkinIng
//	  - nanoflow not found: Ledger.ACT_SkinRabobank
//
// All three were created by statements 1–3 of the same file, in order. `exec`
// ran it without complaint and `mx check` reported 0 errors, so the finding was
// false in both directions that matter.
//
// It is the one omission in a list of five. validateWidgetReferences consults
// the script context for microflows, pages, snippets and entities, and for
// nanoflows alone it does not — so the exemption covered every widget reference
// except this one.

// actionWidget is a button bound to a flow, the shape the report is about.
func actionWidget(kind, ref string) []*ast.WidgetV3 {
	return []*ast.WidgetV3{{
		Name:       "ctn",
		Type:       "container",
		Properties: map[string]any{},
		Children: []*ast.WidgetV3{{
			Name: "btnA",
			Type: "actionbutton",
			Properties: map[string]any{
				"Action": &ast.ActionV3{Type: kind, Target: ref},
			},
		}},
	}}
}

// The reported case.
func TestWidgetRefs_NanoflowCreatedEarlierInTheSameScript(t *testing.T) {
	ctx, _ := newMockCtx(t)
	sc := newScriptContext()
	sc.nanoflows["Ledger.ACT_SkinIng"] = true

	errs := validateWidgetReferences(ctx, actionWidget("nanoflow", "Ledger.ACT_SkinIng"), sc)
	if len(errs) != 0 {
		t.Errorf("a nanoflow created earlier in the same script must be exempt, like every other "+
			"reference kind; got:\n  %s", strings.Join(errs, "\n  "))
	}
}

// CONTROL: a nanoflow that is in neither the project nor the script is still
// reported. Without this the fix is indistinguishable from dropping the check.
func TestWidgetRefs_UnknownNanoflowIsStillReported(t *testing.T) {
	ctx, _ := newMockCtx(t)
	sc := newScriptContext()

	errs := validateWidgetReferences(ctx, actionWidget("nanoflow", "Ledger.Nope"), sc)
	if len(errs) != 1 || !strings.Contains(errs[0], "nanoflow not found: Ledger.Nope") {
		t.Fatalf("got %v, want one 'nanoflow not found'", errs)
	}
}

// The four kinds that already worked, asserted together so the omission cannot
// come back on a different line: each is exempt when the script defines it.
func TestWidgetRefs_EveryKindHonoursTheScriptContext(t *testing.T) {
	cases := []struct {
		kind     string
		register func(sc *scriptContext, ref string)
		widgets  func(ref string) []*ast.WidgetV3
	}{
		{"microflow", func(sc *scriptContext, r string) { sc.microflows[r] = true },
			func(r string) []*ast.WidgetV3 { return actionWidget("microflow", r) }},
		{"nanoflow", func(sc *scriptContext, r string) { sc.nanoflows[r] = true },
			func(r string) []*ast.WidgetV3 { return actionWidget("nanoflow", r) }},
		{"page", func(sc *scriptContext, r string) { sc.pages[r] = true },
			func(r string) []*ast.WidgetV3 { return actionWidget("page", r) }},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			ctx, _ := newMockCtx(t)
			sc := newScriptContext()
			ref := "Ledger.Made" + tc.kind
			tc.register(sc, ref)
			if errs := validateWidgetReferences(ctx, tc.widgets(ref), sc); len(errs) != 0 {
				t.Errorf("%s: %v", tc.kind, errs)
			}
		})
	}
}
