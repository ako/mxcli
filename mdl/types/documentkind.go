// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"
	"unicode"
)

// documentKindOverrides names the document types where mxcli's own vocabulary
// differs from Mendix's stored $Type. Everything else is derived from the
// $Type itself by DocumentKind, so a document type mxcli has never heard of
// still gets a truthful name rather than falling back to "document".
//
// Kept deliberately short: an entry here is a claim that mxcli calls the thing
// something other than what the model calls it, and each one is a place the two
// vocabularies can drift. A type absent from this map is not a gap.
var documentKindOverrides = map[string]string{
	"Menus$MenuDocument":                     "menu",
	"Rest$ConsumedRestService":               "rest client",
	"Rest$PublishedRestService":              "published rest service",
	"CustomBlobDocuments$CustomBlobDocument": "agent document",
	"CustomIcons$CustomIconCollection":       "icon collection",
	"DatabaseConnector$DatabaseConnection":   "database connection",
	// The camel-case split cannot know that "OData" and "JavaScript" are each
	// one word, so it produces "consumed o data service" and "java script
	// action". These are the derivation's blind spot, not a vocabulary
	// difference.
	"Rest$ConsumedODataService":          "odata client",
	"ODataPublish$PublishedODataService": "odata service",
	"JavaScriptActions$JavaScriptAction": "javascript action",
}

// DocumentKind renders a unit's stored $Type as the noun mxcli uses for it —
// "JsonStructures$JsonStructure" becomes "json structure".
//
// The derivation (take the part after "$", split the camel case, lowercase it)
// is right for the large majority of document types and, crucially, degrades
// honestly: an unrecognised type yields the model's own name for it instead of
// a generic placeholder. Callers use this to report what they acted on when
// they located a document without knowing its type in advance.
func DocumentKind(unitType string) string {
	if unitType == "" {
		return "document"
	}
	if kind, ok := documentKindOverrides[unitType]; ok {
		return kind
	}
	name := unitType
	if i := strings.LastIndex(unitType, "$"); i >= 0 {
		name = unitType[i+1:]
	}
	if name == "" {
		return unitType
	}
	return strings.ToLower(splitCamelWords(name))
}

// splitCamelWords inserts a space before each interior capital that starts a
// new word, so "JsonStructure" reads as "Json Structure". A run of capitals is
// treated as one word ("XPathQuery" → "XPath Query") so acronyms survive.
func splitCamelWords(s string) string {
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || unicode.IsDigit(prev) || (unicode.IsUpper(prev) && nextIsLower) {
				b.WriteRune(' ')
			}
		}
		b.WriteRune(r)
	}
	return b.String()
}
