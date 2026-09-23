// SPDX-License-Identifier: Apache-2.0

package rules

import "testing"

// The two legacy image widgets are not supported by the React client, which
// Mendix added in 10.7 and which is the only client on 11. mxbuild reports
// CE0582 on each whenever it is enabled, so on a Mendix 11 app these are build
// ERRORS, not style points — measured on 11.12.1:
//
//	[error] [CE0582] "Widget static image is not supported in React client.
//	                  Right-click this error to convert it to an alternative
//	                  widget type."   at Static image 'imgLogo'
//
// mxcli can author both (it must: a project being converted up already contains
// them), and nothing told the author they were reaching for a widget their app
// cannot build. This rule is that telling.
func TestLegacyImageWidget_NamesBothTypesAndTheReplacement(t *testing.T) {
	cases := []struct {
		widgetType string
		wantTerm   string
		wantMDL    string
	}{
		{"Forms$StaticImageViewer", "static image", "staticimage"},
		{"Forms$ImageViewer", "dynamic image", "dynamicimage"},
	}
	for _, c := range cases {
		t.Run(c.widgetType, func(t *testing.T) {
			got, ok := LegacyImageWidget(c.widgetType)
			if !ok {
				t.Fatalf("%s not recognised as a legacy image widget", c.widgetType)
			}
			// Mendix's own term, so the lint message and the CE0582 the author
			// will see from mxbuild use the same words.
			if got.MendixTerm != c.wantTerm {
				t.Errorf("MendixTerm = %q, want %q", got.MendixTerm, c.wantTerm)
			}
			if got.MDLKeyword != c.wantMDL {
				t.Errorf("MDLKeyword = %q, want %q", got.MDLKeyword, c.wantMDL)
			}
		})
	}
}

// The CONTROL, and the reason this is a deny-list of two rather than anything
// broader: every other widget must be silent. A rule that fires on the
// PLUGGABLE image — the widget it tells people to move TO — would be worse than
// no rule at all.
func TestLegacyImageWidget_IsSilentOnEverythingElse(t *testing.T) {
	for _, widgetType := range []string{
		"CustomWidgets$CustomWidget", // the pluggable image lives here
		"Forms$DynamicText",
		"Forms$DataView",
		"DocumentTemplates$StaticImageViewer", // a document template, not a page
		"",
	} {
		if got, ok := LegacyImageWidget(widgetType); ok {
			t.Errorf("%q was reported as a legacy image widget (%+v)", widgetType, got)
		}
	}
}

// The rule's identity is part of its contract: an ID that collides with another
// rule's silently shadows it in the config, and the category decides what
// `--category` filters it into.
func TestLegacyImageWidgetRule_Identity(t *testing.T) {
	r := NewLegacyImageWidgetRule()
	if r.ID() != "MPR012" {
		t.Errorf("ID = %q, want MPR012", r.ID())
	}
	if r.Category() != "correctness" {
		t.Errorf("Category = %q — CE0582 is a build error, not a style preference", r.Category())
	}
	if r.Name() == "" || r.Description() == "" {
		t.Error("a rule with no name or description cannot be configured or explained")
	}
}
