// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// widget28 parses a script and returns only the MDL-WIDGET28 violations.
//
// It goes through visitor.Build rather than hand-built WidgetV3 literals
// BECAUSE the rule is about what the PARSER does with an unmatched action
// expression. A hand-built `Properties{"Action": "OPEN_LINK"}` would assert the
// same string comparison while proving nothing about whether the grammar still
// produces it — and the grammar is half the fix (mendixlabs/mxcli#1062).
func widget28(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing %q: %v", src, errs)
	}
	registry := LoadWidgetRegistry("")
	if registry == nil {
		t.Fatal("built-in widget registry not available")
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		for _, v := range ValidateWidgetPropertiesForStatement(stmt, registry) {
			if v.RuleID == "MDL-WIDGET28" {
				out = append(out, v)
			}
		}
	}
	return out
}

func page28(body string) string {
	return fmt.Sprintf("create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {\n  %s\n}", body)
}

// The reported case: a real action keyword short its argument. Measured on
// Mendix 11.12.0 before the fix — `mxcli check` passed, exec said "Created
// page", `mx check` reported 0 errors, and `describe page` came back with no
// action on the button at all.
func TestActionSlot_UnderSpecifiedKeywordIsReported(t *testing.T) {
	got := widget28(t, page28(`actionbutton b (Caption: 'Go', Action: OPEN_LINK)`))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET28, want 1: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — a warning would let exec write the dead button, "+
			"which is the whole complaint", got[0].Severity)
	}
	if !strings.Contains(got[0].Message, "OPEN_LINK") || !strings.Contains(got[0].Message, "`b`") {
		t.Errorf("message must name the value and the widget:\n%s", got[0].Message)
	}
	// The author was one token from working syntax; the suggestion has to say
	// which token rather than sending them to the grammar.
	if !strings.Contains(got[0].Suggestion, "https://example.com") {
		t.Errorf("suggestion must show OPEN_LINK's missing argument:\n%s", got[0].Suggestion)
	}
}

// The other half of the report: a token that was never a keyword.
func TestActionSlot_InventedKeywordIsReported(t *testing.T) {
	got := widget28(t, page28(`actionbutton b (Caption: 'Go', Action: TOTALLY_MADE_UP)`))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET28, want 1: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Suggestion, "NOTHING") {
		t.Errorf("an author who invented a keyword needs the vocabulary, including the inert "+
			"spelling:\n%s", got[0].Suggestion)
	}
}

// THE CONTROL FOR THE WHOLE RULE.
//
// `Action: NOTHING` is the documented spelling for a deliberately inert widget
// (docs-site, the quick reference, the synced alter-page skill, nine
// mdl-examples scripts) and it was NOT in actionExprV3 — it reached
// Forms$NoAction through the same fall-through the two tests above report. If
// the grammar promotion is reverted this test fails, which is what pins the two
// halves of the fix together: the scalar cannot be rejected while the working
// spelling still depends on it.
func TestActionSlot_NothingIsARealActionAndStaysClean(t *testing.T) {
	for _, body := range []string{
		`actionbutton b (Caption: 'Inert', Action: NOTHING)`,
		`actionbutton b (Caption: 'Inert', action: nothing)`,
		`container c (OnClick: NOTHING) { dynamictext t (Content: 'x') }`,
	} {
		if got := widget28(t, page28(body)); len(got) != 0 {
			t.Errorf("%s\n  reported %d violations, want 0 — this is documented, shipped syntax: %+v",
				body, len(got), got)
		}
	}
}

// Every real action form must survive the rule. Cheap to assert and it is the
// difference between "rejects scalars" and "rejects everything it does not
// recognise".
func TestActionSlot_RealActionsStayClean(t *testing.T) {
	for _, body := range []string{
		`actionbutton b (Caption: 'x', Action: SAVE_CHANGES)`,
		`actionbutton b (Caption: 'x', Action: SAVE_CHANGES CLOSE_PAGE)`,
		`actionbutton b (Caption: 'x', Action: CANCEL_CHANGES)`,
		`actionbutton b (Caption: 'x', Action: CLOSE_PAGE)`,
		`actionbutton b (Caption: 'x', Action: DELETE_OBJECT)`,
		`actionbutton b (Caption: 'x', Action: SIGN_OUT)`,
		`actionbutton b (Caption: 'x', Action: OPEN_LINK 'https://example.com')`,
		`actionbutton b (Caption: 'x', Action: COMPLETE_TASK 'Approved')`,
		`actionbutton b (Caption: 'x', Action: SHOW_PAGE M.Other)`,
		`actionbutton b (Caption: 'x', Action: MICROFLOW M.ACT_Go)`,
		`actionbutton b (Caption: 'x', Action: NANOFLOW M.NF_Go)`,
		`actionbutton b (Caption: 'x', Action: CREATE_OBJECT M.Thing THEN SHOW_PAGE M.Other)`,
	} {
		if got := widget28(t, page28(body)); len(got) != 0 {
			t.Errorf("%s\n  reported %d violations, want 0: %+v", body, len(got), got)
		}
	}
}

// Each under-specified keyword gets its own missing token named. Written as a
// table because the map behind it is the kind of list that rots silently.
func TestActionSlot_EachKeywordNamesWhatItIsMissing(t *testing.T) {
	for _, tt := range []struct{ value, want string }{
		{"OPEN_LINK", "https://example.com"},
		{"COMPLETE_TASK", "COMPLETE_TASK 'Approved'"},
		{"SHOW_PAGE", "SHOW_PAGE Module.Page"},
		{"CREATE_OBJECT", "CREATE_OBJECT Module.Entity"},
		{"microflow", "MICROFLOW Module.Flow"},
		{"nanoflow", "NANOFLOW Module.Flow"},
	} {
		got := widget28(t, page28(fmt.Sprintf(`actionbutton b (Caption: 'x', Action: %s)`, tt.value)))
		if len(got) != 1 {
			t.Errorf("Action: %s — got %d violations, want 1: %+v", tt.value, len(got), got)
			continue
		}
		if !strings.Contains(got[0].Suggestion, tt.want) {
			t.Errorf("Action: %s — suggestion must contain %q:\n%s", tt.value, tt.want, got[0].Suggestion)
		}
	}
}

// `OnClick:` and `OnChange:` have their own dedicated grammar branches and so
// degrade the same way. `OnClick:` is an ALIAS that lands on Properties["Action"]
// once parsed (#603) but keeps its own key when it does NOT parse — so a rule
// that only looked at "Action" would miss exactly the broken spelling.
func TestActionSlot_EveryDedicatedSlotIsChecked(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"Action", `actionbutton b (Caption: 'x', Action: OPEN_LINK)`},
		{"OnClick", `container c (OnClick: TOTALLY_MADE_UP) { dynamictext t (Content: 'x') }`},
		{"OnChange", `textbox tb (Attribute: Name, OnChange: TOTALLY_MADE_UP)`},
	} {
		got := widget28(t, page28(tt.body))
		if len(got) != 1 {
			t.Errorf("%s: got %d violations, want 1: %+v", tt.name, len(got), got)
			continue
		}
		if !strings.Contains(got[0].Message, tt.name+":") {
			t.Errorf("%s: message must name the slot as written:\n%s", tt.name, got[0].Message)
		}
	}
}

// A widget nested below the top level is reached by the same walk.
func TestActionSlot_NestedWidgetIsReported(t *testing.T) {
	got := widget28(t, page28(
		`container outer { container inner { actionbutton b (Caption: 'x', Action: SHOW_PAGE) } }`))
	if len(got) != 1 {
		t.Fatalf("got %d violations, want 1: %+v", len(got), got)
	}
}

// Two faulty slots on one widget must come out in the same order every run.
// w.Properties is a map, so iterating it directly would shuffle (CLAUDE.md).
func TestActionSlot_TwoFaultySlotsReportDeterministically(t *testing.T) {
	w := &ast.WidgetV3{Name: "tb", Type: "textbox", Properties: map[string]any{
		"Action":   "OPEN_LINK",
		"OnChange": "TOTALLY_MADE_UP",
	}}
	initial := validateWidgetActionSlot(w, "page M.P")
	if len(initial) != 2 {
		t.Fatalf("got %d violations, want 2: %+v", len(initial), initial)
	}
	first := violationOrder(initial)
	for i := 0; i < 20; i++ {
		if got := violationOrder(validateWidgetActionSlot(w, "page M.P")); got != first {
			t.Fatalf("order changed between runs: %q then %q", first, got)
		}
	}
}

func violationOrder(vs []linter.Violation) string {
	var parts []string
	for _, v := range vs {
		parts = append(parts, v.Message)
	}
	return strings.Join(parts, "|")
}

// A non-string scalar reaches the slot too (propertyValueV3 admits numbers and
// booleans), and must not panic or be silently accepted.
func TestActionSlot_NonStringScalarIsReported(t *testing.T) {
	for _, raw := range []any{42, true, []any{"a"}} {
		w := &ast.WidgetV3{Name: "b", Type: "actionbutton", Properties: map[string]any{"Action": raw}}
		got := validateWidgetActionSlot(w, "page M.P")
		if len(got) != 1 {
			t.Errorf("Action: %v (%T) — got %d violations, want 1", raw, raw, len(got))
			continue
		}
		if !strings.Contains(got[0].Suggestion, "action expression") {
			t.Errorf("Action: %v — a value that is not a keyword needs the vocabulary:\n%s",
				raw, got[0].Suggestion)
		}
	}
}

// A widget with no action slot at all is the commonest widget there is.
func TestActionSlot_NoActionPropertyIsSilent(t *testing.T) {
	w := &ast.WidgetV3{Name: "t", Type: "dynamictext", Properties: map[string]any{"Content": "x"}}
	if got := validateWidgetActionSlot(w, "page M.P"); len(got) != 0 {
		t.Errorf("got %d violations on a widget with no action slot: %+v", len(got), got)
	}
}
