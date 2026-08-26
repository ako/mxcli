// SPDX-License-Identifier: Apache-2.0

// mendixlabs/mxcli#970, stage 2. The executor fixed the sites that CREATE a
// model.Text; these are the writer leaves that build a Texts$Text straight from
// a bare Go string, where the model never carried a language at all. A widget
// label is the reachable case: pages.TextBox.Label is a string, so
// serializeLabelTemplate is the only thing that can choose its LanguageCode.
package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// languageCodesIn walks serialized BSON and collects every LanguageCode written.
func languageCodesIn(v any) []string {
	var out []string
	switch t := v.(type) {
	case bson.D:
		for _, e := range t {
			if e.Key == "LanguageCode" {
				if s, ok := e.Value.(string); ok {
					out = append(out, s)
				}
				continue
			}
			out = append(out, languageCodesIn(e.Value)...)
		}
	case bson.A:
		for _, e := range t {
			out = append(out, languageCodesIn(e)...)
		}
	case []any:
		for _, e := range t {
			out = append(out, languageCodesIn(e)...)
		}
	}
	return out
}

func TestLabelTemplateUsesAuthoringLanguage(t *testing.T) {
	orig := model.AuthoringLanguage()
	t.Cleanup(func() { model.SetAuthoringLanguage(orig) })

	for _, tc := range []struct{ set, want string }{
		{"nl_NL", "nl_NL"},
		{"en_US", "en_US"}, // the common case must not regress
		{"", "en_US"},      // unset falls back to the pre-fix behaviour
	} {
		t.Run(tc.want+"/"+tc.set, func(t *testing.T) {
			model.SetAuthoringLanguage(tc.set)
			got := languageCodesIn(serializeLabelTemplate("Opslaan"))
			if len(got) == 0 {
				t.Fatal("no LanguageCode written for a label")
			}
			for _, code := range got {
				if code != tc.want {
					t.Errorf("label LanguageCode = %q, want %q (mendixlabs/mxcli#970)", code, tc.want)
				}
			}
		})
	}
}
