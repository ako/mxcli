// SPDX-License-Identifier: Apache-2.0

// Every object-list container keyword the .def.json generator derives must be a
// keyword the MDL grammar can actually lex.
//
// The generator builds `mdlContainer` from the widget's property key
// (customButtons → CUSTOMBUTTON), while the grammar's widgetTypeV3 carries a
// hand-maintained list. Nothing checked that the two agree, so a widget whose
// object list had no keyword described back as MDL that does not parse:
//
//	line 22:12 mismatched input 'custombutton' expecting '}'
//
// Measured on File Uploader 2.5.0: `describe page` emitted `custombutton
// custombutton1 (...)` and `exec` of that description was a parse error, so the
// page could not round-trip and the item's action slots could not be authored at
// all (#956).
package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// containerKeywordsInGrammar reads the keywords widgetTypeV3 accepts straight
// from the grammar, so the test cannot drift from it.
func containerKeywordsInGrammar(t *testing.T) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "grammar", "domains", "MDLPage.g4"))
	if err != nil {
		t.Skipf("grammar not readable: %v", err)
	}
	out := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*\|\s*([A-Z][A-Z0-9]*)\s*$`).FindAllStringSubmatch(string(src), -1) {
		out[m[1]] = true
	}
	if len(out) < 10 {
		t.Fatalf("only %d keywords parsed from MDLPage.g4 — the regex has drifted from the grammar", len(out))
	}
	return out
}

// knownUnsupportedContainers are derived keywords that collide with an existing
// token (ATTRIBUTE, ATTR and EVENT are already lexed for other purposes), so they
// cannot simply be added to widgetTypeV3. Their widgets' object lists are not
// authorable until the collision is resolved — tracked, not silently tolerated.
var knownUnsupportedContainers = map[string]string{
	"ATTRIBUTE": "collides with the Attribute: property keyword (htmlelement attributes)",
	"ATTR":      "collides with the ATTR token (accessibilityhelper)",
	"EVENT":     "collides with the EVENT token (htmlelement events)",
}

func TestDerivedContainerKeywordsExistInTheGrammar(t *testing.T) {
	known := containerKeywordsInGrammar(t)

	// Derive the keyword the generator would produce for each object-list
	// property name seen on real widgets, using the generator's own function.
	for _, propertyKey := range []string{
		"columns", "groups", "series", "lines", "markers", "dynamicMarkers",
		"scaleColors", "customItems", "basicItems", "customButtons",
		"allowedFileFormats",
	} {
		kw := strings.ToUpper(deriveObjectListKeyword(propertyKey))
		if kw == "" {
			continue
		}
		if known[kw] {
			continue
		}
		if why, ok := knownUnsupportedContainers[kw]; ok {
			t.Logf("%s (from %q) is knowingly unsupported: %s", kw, propertyKey, why)
			continue
		}
		t.Errorf("object-list property %q derives container keyword %q, which widgetTypeV3 does not accept — "+
			"DESCRIBE will emit MDL that does not parse; add it to MDLLexer.g4 + MDLPage.g4, or record it in "+
			"knownUnsupportedContainers with the reason", propertyKey, kw)
	}
}
