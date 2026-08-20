// SPDX-License-Identifier: Apache-2.0

package types

import "testing"

// TestDocumentKindDerivesTheCommonCase pins that the derivation, not the
// override table, carries most document types — the table stays short only if
// this keeps being true.
func TestDocumentKindDerivesTheCommonCase(t *testing.T) {
	cases := map[string]string{
		"JsonStructures$JsonStructure":         "json structure",
		"ImportMappings$ImportMapping":         "import mapping",
		"ExportMappings$ExportMapping":         "export mapping",
		"Microflows$Microflow":                 "microflow",
		"Microflows$Nanoflow":                  "nanoflow",
		"Forms$Page":                           "page",
		"Forms$Snippet":                        "snippet",
		"Forms$Layout":                         "layout",
		"Forms$BuildingBlock":                  "building block",
		"Enumerations$Enumeration":             "enumeration",
		"Constants$Constant":                   "constant",
		"Queues$Queue":                         "queue",
		"ScheduledEvents$ScheduledEvent":       "scheduled event",
		"RegularExpressions$RegularExpression": "regular expression",
		"Workflows$Workflow":                   "workflow",
		"JavaActions$JavaAction":               "java action",
		"Images$ImageCollection":               "image collection",
		"DataTransformers$DataTransformer":     "data transformer",
		"BusinessEvents$BusinessEventService":  "business event service",
	}
	for unitType, want := range cases {
		if got := DocumentKind(unitType); got != want {
			t.Errorf("DocumentKind(%q) = %q, want %q", unitType, got, want)
		}
	}
}

// TestDocumentKindOverridesTheDerivationsBlindSpots covers the two reasons an
// override exists: mxcli calling something by a different name than the model
// does, and the camel-case split not knowing that "OData" and "JavaScript" are
// each a single word.
func TestDocumentKindOverridesTheDerivationsBlindSpots(t *testing.T) {
	cases := map[string]string{
		"Menus$MenuDocument":                 "menu",
		"Rest$ConsumedRestService":           "rest client",
		"Rest$PublishedRestService":          "published rest service",
		"CustomIcons$CustomIconCollection":   "icon collection",
		"Rest$ConsumedODataService":          "odata client",
		"ODataPublish$PublishedODataService": "odata service",
		"JavaScriptActions$JavaScriptAction": "javascript action",
	}
	for unitType, want := range cases {
		if got := DocumentKind(unitType); got != want {
			t.Errorf("DocumentKind(%q) = %q, want %q", unitType, got, want)
		}
	}
}

// TestDocumentKindDegradesToTheModelsOwnName is the property that lets callers
// report a document type nobody has taught mxcli about. An unknown type must
// yield the model's word for it, never a generic placeholder that would read as
// though mxcli recognised it.
func TestDocumentKindDegradesToTheModelsOwnName(t *testing.T) {
	if got := DocumentKind("SomeFutureDomain$WidgetGallery"); got != "widget gallery" {
		t.Errorf("DocumentKind of an unknown type = %q, want it derived from the type", got)
	}
	if got := DocumentKind(""); got != "document" {
		t.Errorf("DocumentKind(\"\") = %q, want the generic fallback", got)
	}
}
