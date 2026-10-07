// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1318: "mxcli syntax rename" said "Unknown topic: rename"
// while RENAME MICROFLOW/PAGE/ENTITY parsed, executed, and were documented by
// `mxcli help rename`. An agent consulting `syntax` concluded MDL could not
// rename a microflow and rebuilt it by hand: copy, re-point every caller,
// delete the original.
//
// As with the widget keywords (widget_keywords_drift_test.go), the grammar is
// the authority: every target the renameStatement rule accepts must be named in
// the rename topic, so a target added to the parser cannot ship undocumented.

// renameTargets reads the renameTarget rule plus the MODULE alternative of
// renameStatement out of the committed grammar, as the words a user types
// ("JAVA ACTION", not "JAVA ACTION" split in two).
func renameTargets(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "mdl", "grammar", "MDLParser.g4")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(b)
	rule := regexp.MustCompile(`(?s)\nrenameTarget\s*\n\s*:(.*?)\n\s*;`).FindStringSubmatch(src)
	if rule == nil {
		t.Fatal("renameTarget rule not found — if the rule was renamed, re-point this guard " +
			"rather than deleting it; it exists because RENAME shipped absent from `mxcli syntax` (#1318)")
	}
	var out []string
	for _, alt := range strings.Split(rule[1], "|") {
		if alt = strings.Join(strings.Fields(alt), " "); alt != "" {
			out = append(out, alt)
		}
	}
	if !regexp.MustCompile(`RENAME MODULE identifierOrKeyword TO`).MatchString(src) {
		t.Fatal("RENAME MODULE alternative not found in renameStatement — re-point this guard")
	}
	return append(out, "MODULE")
}

func TestRenameTopicIsRegistered(t *testing.T) {
	if ByPath(ResolveAlias("rename")) == nil {
		t.Fatal(`"rename" resolves to no syntax topic — ` + "`mxcli syntax rename`" + ` reports "Unknown topic: rename" (#1318)`)
	}
}

func TestRenameTopicNamesEveryGrammarTarget(t *testing.T) {
	f := ByPath(ResolveAlias("rename"))
	if f == nil {
		t.Fatal(`no "rename" topic (#1318)`)
	}
	doc := strings.ToUpper(f.Syntax + "\n" + f.Example)
	for _, target := range renameTargets(t) {
		if !strings.Contains(doc, "RENAME "+target) {
			t.Errorf("the grammar accepts RENAME %s but the rename topic never shows it", target)
		}
	}
}

// The topics a reader is on when the question arises point at it, so the
// statement is found from the document type as well as from the verb.
func TestRenameTopicIsCrossReferenced(t *testing.T) {
	for _, from := range []string{"microflow", "page", "domain-model.entity", "move"} {
		f := ByPath(from)
		if f == nil {
			t.Errorf("topic %q not registered", from)
			continue
		}
		found := false
		for _, ref := range f.SeeAlso {
			if ref == "rename" {
				found = true
			}
		}
		if !found {
			t.Errorf("topic %q has no see_also to rename", from)
		}
	}
}
