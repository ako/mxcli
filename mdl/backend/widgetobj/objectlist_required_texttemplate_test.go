// SPDX-License-Identifier: Apache-2.0

// Issue #891: authoring an Accordion raised CE0463 "the definition of this
// widget has changed".
//
// An object-list item's REQUIRED TextTemplate that the author leaves unset was
// serialized as null. emptyClientTemplateRules covers only DataGrid columns, so
// every other object-list widget fell through it — in a stock blank app the
// Accordion's group (required `headerText`) failed, and thirteen shipped widgets
// declare object lists with texttemplate items.
//
// Both weaker forms were measured and rejected:
//
//	null           -> CE0463 "the definition of this widget has changed"
//	empty template -> CE4899 "Property 'Groups/1/Text' is required"
//
// Only the widget's own shipped translations satisfy both, which is what
// `mx update-widgets` writes.
package widgetobj

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"go.mongodb.org/mongo-driver/bson"
)

// accordionGroupEntry mirrors the Accordion's `groups` object list: a required
// `headerText` TextTemplate shipping 'Header'/'Koptekst', plus an optional one.
func accordionGroupEntry() pages.PropertyTypeIDEntry {
	return pages.PropertyTypeIDEntry{
		PropertyTypeID: "00000000000000000000000000000001",
		NestedKeyOrder: []string{"headerText", "optionalText"},
		NestedPropertyIDs: map[string]pages.PropertyTypeIDEntry{
			"headerText": {
				PropertyTypeID: "00000000000000000000000000000002",
				ValueType:      "TextTemplate",
				Required:       true,
				DefaultTranslations: []pages.PropertyTranslation{
					{LanguageCode: "en_US", Text: "Header"},
					{LanguageCode: "nl_NL", Text: "Koptekst"},
				},
			},
			"optionalText": {
				PropertyTypeID: "00000000000000000000000000000003",
				ValueType:      "TextTemplate",
				Required:       false,
				DefaultTranslations: []pages.PropertyTranslation{
					{LanguageCode: "en_US", Text: "Tooltip"},
				},
			},
		},
	}
}

// collectTranslations gathers every Texts$Translation under a node.
func collectTranslations(v any, out *[][2]string) {
	switch n := v.(type) {
	case bson.D:
		isTranslation := false
		var lang, text string
		for _, e := range n {
			if e.Key == "$Type" && e.Value == "Texts$Translation" {
				isTranslation = true
			}
			if e.Key == "LanguageCode" {
				lang, _ = e.Value.(string)
			}
			if e.Key == "Text" {
				text, _ = e.Value.(string)
			}
		}
		if isTranslation {
			*out = append(*out, [2]string{lang, text})
		}
		for _, e := range n {
			collectTranslations(e.Value, out)
		}
	case bson.A:
		for _, e := range n {
			collectTranslations(e, out)
		}
	}
}

// Goes through buildObjectListItemBSON, not the helper, so removing the call
// site fails this test — a helper-level assertion would prove the helper works
// and nothing about the wiring.
func TestObjectListItem_RequiredTextTemplateGetsShippedDefault(t *testing.T) {
	got := buildObjectListItemBSON(
		"com.mendix.widget.web.accordion.Accordion", "groups",
		accordionGroupEntry(),
		backend.ObjectListItemSpec{}, // author set nothing
	)

	var found [][2]string
	collectTranslations(got, &found)

	want := map[string]string{"en_US": "Header", "nl_NL": "Koptekst"}
	seen := map[string]string{}
	for _, p := range found {
		seen[p[0]] = p[1]
	}
	for lang, text := range want {
		if seen[lang] != text {
			t.Errorf("required headerText missing its shipped %s default: got %q, want %q (null here is CE0463, empty is CE4899)",
				lang, seen[lang], text)
		}
	}

	// The OPTIONAL TextTemplate must NOT be filled — Studio Pro leaves an unset
	// optional one null, and filling every template took CE0463 from 33 to 127
	// in a previous attempt at this class of fix.
	if seen["en_US"] == "Tooltip" || len(found) > len(want) {
		t.Errorf("an optional TextTemplate was populated too; translations found: %v", found)
	}
}

func TestIsUnsetRequiredTextTemplate(t *testing.T) {
	cases := []struct {
		name string
		in   pages.PropertyTypeIDEntry
		want bool
	}{
		{"required texttemplate", pages.PropertyTypeIDEntry{ValueType: "TextTemplate", Required: true}, true},
		{"optional texttemplate", pages.PropertyTypeIDEntry{ValueType: "TextTemplate", Required: false}, false},
		{"required string", pages.PropertyTypeIDEntry{ValueType: "String", Required: true}, false},
		{"required attribute", pages.PropertyTypeIDEntry{ValueType: "Attribute", Required: true}, false},
	}
	for _, tc := range cases {
		if got := isUnsetRequiredTextTemplate(tc.in); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}
