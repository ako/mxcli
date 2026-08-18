// SPDX-License-Identifier: Apache-2.0

package executor

import "testing"

// TestMisusedBuiltinPropertyNamesTheWorkingSpelling is the regression test for
// mxcli-owid #38: `combobox cb (Association: …, Caption: Name)` passed check,
// executed, lost the caption, and failed the build with
// CE0642 "Property 'Caption' is required." The working spelling is
// CaptionAttribute, so the author was one property name away — and concluded
// the feature was unusable and redesigned around it.
func TestMisusedBuiltinPropertyNamesTheWorkingSpelling(t *testing.T) {
	const comboBox = "com.mendix.widget.web.combobox.Combobox"

	right, wrong := misusedBuiltinProperty(comboBox, "Caption")
	if !wrong {
		t.Fatal("Caption on a combobox was not flagged")
	}
	if right != "CaptionAttribute" {
		t.Errorf("suggested %q, want CaptionAttribute", right)
	}
}

// TestMisusedBuiltinPropertyIsCaseInsensitive — property lookup elsewhere folds
// case, so a rule that did not would miss `caption:`.
func TestMisusedBuiltinPropertyIsCaseInsensitive(t *testing.T) {
	const comboBox = "com.mendix.widget.web.combobox.Combobox"
	for _, spelling := range []string{"Caption", "caption", "CAPTION"} {
		if _, wrong := misusedBuiltinProperty(comboBox, spelling); !wrong {
			t.Errorf("%q was not flagged", spelling)
		}
	}
}

// TestMisusedBuiltinPropertyLeavesEverythingElseAlone. The rule is a short list
// of measured cases; it must not touch the working spelling, other properties on
// the same widget, or the same property on a widget that does route it.
func TestMisusedBuiltinPropertyLeavesEverythingElseAlone(t *testing.T) {
	const comboBox = "com.mendix.widget.web.combobox.Combobox"
	cases := []struct{ widgetID, key string }{
		{comboBox, "CaptionAttribute"}, // the fix must not be flagged
		{comboBox, "Label"},
		{comboBox, "Association"},
		{comboBox, "Class"},
		// An action button's Caption is routed and entirely correct.
		{"com.mendix.widget.web.actionbutton.ActionButton", "Caption"},
		{"com.mendix.widget.custom.unknown.Unknown", "Caption"},
	}
	for _, c := range cases {
		if right, wrong := misusedBuiltinProperty(c.widgetID, c.key); wrong {
			t.Errorf("%s on %s was flagged (suggested %q)", c.key, c.widgetID, right)
		}
	}
}
