// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func actionSlotValue(t *testing.T, src, key string) any {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", src, errs)
	}
	st, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("statement type = %T, want *ast.CreatePageStmtV3", prog.Statements[0])
	}
	return st.Widgets[0].Properties[key]
}

// `NOTHING` is the documented spelling for a deliberately inert widget and was
// never an actionExprV3 alternative: it reached Forms$NoAction by FAILING to
// match the action rule and falling through to `keyword COLON propertyValueV3`,
// which stores the slot as a plain string.
//
// That is why mendixlabs/mxcli#1062 could not simply be rejected — the same
// fall-through carries `Action: OPEN_LINK` and `Action: TOTALLY_MADE_UP`, so
// MDL-WIDGET28 would have flagged working, shipped syntax. Promoting NOTHING is
// what separates them.
func TestAction_NothingParsesAsAnAction(t *testing.T) {
	for _, tt := range []struct{ name, src, key string }{
		{"Action upper", "create page M.P (Title: 'x', Layout: A.L) { actionbutton b (Action: NOTHING) };", "Action"},
		{"action lower", "create page M.P (Title: 'x', Layout: A.L) { actionbutton b (action: nothing) };", "Action"},
		{"OnClick", "create page M.P (Title: 'x', Layout: A.L) { container c (OnClick: NOTHING) { dynamictext t (Content: 'x') } };", "Action"},
		{"OnChange", "create page M.P (Title: 'x', Layout: A.L) { textbox tb (Attribute: Name, OnChange: NOTHING) };", "OnChange"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := actionSlotValue(t, tt.src, tt.key)
			action, ok := raw.(*ast.ActionV3)
			if !ok {
				t.Fatalf("%s = %T (%v), want *ast.ActionV3 — a scalar here is the fall-through "+
					"MDL-WIDGET28 rejects, and would make this documented spelling an error",
					tt.key, raw, raw)
			}
			if action.Type != "none" {
				t.Errorf("Type = %q, want none", action.Type)
			}
		})
	}
}

// The fall-through itself, pinned. MDL-WIDGET28 detects the fault by finding a
// non-action in the slot, so if the grammar ever started REJECTING these at
// parse time the rule would go quiet and this test would say why.
func TestAction_UnmatchedExpressionFallsThroughToAScalar(t *testing.T) {
	for _, value := range []string{"OPEN_LINK", "SHOW_PAGE", "CREATE_OBJECT", "COMPLETE_TASK", "TOTALLY_MADE_UP"} {
		src := "create page M.P (Title: 'x', Layout: A.L) { actionbutton b (Action: " + value + ") };"
		raw := actionSlotValue(t, src, "Action")
		if _, isAction := raw.(*ast.ActionV3); isAction {
			t.Errorf("Action: %s parsed as an action — if the grammar now matches it, "+
				"MDL-WIDGET28 needs revisiting rather than this test", value)
		}
		if got, ok := raw.(string); !ok || got != value {
			t.Errorf("Action: %s stored as %T %v, want the string %q", value, raw, raw, value)
		}
	}
}
