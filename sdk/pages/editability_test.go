// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance-2: `editable: Never` in CREATE PAGE was parsed,
// validated, and then dropped — the writers hardcoded "Always" for every static
// input widget, so the field stayed editable with nothing reported anywhere.
package pages

import "testing"

func TestCanonicalEditability(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		ok   bool
	}{
		{"Never", "Never", true},
		{"never", "Never", true},
		{"  Always  ", "Always", true},
		{"Conditional", "Conditional", true},

		// Mendix resolves the enum on load, so an invented member is the class of
		// value that builds clean and will not open. The caller must not pass an
		// unknown string through.
		{"ReadOnly", "", false},
		{"true", "", false},
		{"", "", false},
	} {
		got, ok := CanonicalEditability(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("CanonicalEditability(%q) = %q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestWidgetEditabilityPrecedence(t *testing.T) {
	// The conditional settings element is what makes the enum "Conditional", so it
	// has to win — writing an explicit enum beside a contradicting element is how
	// a document ends up internally inconsistent.
	cond := &BaseWidget{
		Editable:               "Never",
		ConditionalEditability: &ConditionalEditabilitySettings{Expression: "$x"},
	}
	if got := WidgetEditability(cond); got != "Conditional" {
		t.Errorf("EDITABLE IF did not win: got %q", got)
	}

	if got := WidgetEditability(&BaseWidget{Editable: "Never"}); got != "Never" {
		t.Errorf("explicit editable lost: got %q", got)
	}

	// Unset means Mendix's own default, not empty — a writer must never store "".
	if got := WidgetEditability(&BaseWidget{}); got != "Always" {
		t.Errorf("unset should default to Always, got %q", got)
	}
	if got := WidgetEditability(nil); got != "Always" {
		t.Errorf("nil should default to Always, got %q", got)
	}
}
