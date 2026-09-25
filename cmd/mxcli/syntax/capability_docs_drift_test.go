// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docsRoot resolves docs/01-project/ from this package's directory.
func docsRoot(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..", "..", "docs", "01-project")
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("docs/01-project not found from %s: %v", mustWD(t), err)
	}
	return root
}

func mustWD(t *testing.T) string {
	t.Helper()
	wd, _ := os.Getwd()
	return wd
}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(docsRoot(t), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// TestCapabilityDocsDoNotClaimShippedFeaturesAreMissing is the drift guard for
// FINDINGS #20.
//
// The failure it catches is not a typo — it is a document that keeps asserting a
// gap after the gap was filled. That is worse than no document: a reader greps
// it, believes the feature is unavailable, and designs an elaborate workaround
// around a blocker that no longer exists. The report that prompted this named
// the External Database Connector, which had been listed as unsupported long
// after CREATE DATABASE CONNECTION shipped.
//
// The check is deliberately indirect: rather than re-encoding a list of features
// (which would itself drift), it asserts that every capability with a registered
// `mxcli syntax` topic is NOT sitting in the docs' "no MDL surface at all"
// table. The syntax registry is populated from the code, so it cannot claim a
// topic for something that does not exist.
// shippedCapabilities pairs a capability with the syntax topic that PROVES it
// ships. A topic is registered from Go code, so the pairing cannot go stale in
// the direction that matters: delete the feature and the topic goes with it.
//
// The last three were added after an audit found the matrix claiming all three
// were unavailable: "Microflow rules" and "Message definitions" sat under "Not
// Yet Implemented" (i.e. "no MDL surface at all") while both were authorable,
// and Layouts were described as "Read-only, no syntax topic" in three separate
// gap lists. None of it was caught, because this table did not name them — a
// hand-maintained guard only guards what someone remembered to add.
var shippedCapabilities = []struct{ topic, claim string }{
	{"database-connection", "Ext. DB connector"},
	{"queue", "Task queue"},
	{"scheduled-event", "Scheduled events"},
	{"regular-expression", "Regular expressions"},
	{"image-collection", "Image collection"},
	{"navigation.menu-document", "Menus"},
	{"workflow", "Workflows"},
	{"layout", "Layouts"},
	{"microflow.rule", "Microflow rules"},
	{"message-definition", "Message definitions"},
}

func TestCapabilityDocsDoNotClaimShippedFeaturesAreMissing(t *testing.T) {
	matrix := readDoc(t, "MDL_FEATURE_MATRIX.md")

	start := strings.Index(matrix, "### Not Yet Implemented")
	if start < 0 {
		t.Fatal(`MDL_FEATURE_MATRIX.md has no "### Not Yet Implemented" section — ` +
			`if it was renamed, update this guard rather than deleting it`)
	}
	section := matrix[start:]
	if end := strings.Index(section, "\n## "); end > 0 {
		section = section[:end]
	}

	// Each capability that must not appear as unimplemented, keyed by the syntax
	// topic that proves it ships. A topic is registered from Go code, so this
	// pairing cannot go stale in the direction that matters.
	for _, c := range shippedCapabilities {
		if ByPath(c.topic) == nil {
			t.Errorf("no `mxcli syntax %s` topic — either the feature was removed "+
				"(then drop this row) or the topic is missing (then add it)", c.topic)
			continue
		}
		if strings.Contains(section, c.claim) {
			t.Errorf("MDL_FEATURE_MATRIX.md still lists %q under \"Not Yet Implemented\", "+
				"but `mxcli syntax %s` exists — the doc sends readers to Studio Pro "+
				"for work mxcli can already do", c.claim, c.topic)
		}
	}
}

// TestMissingCapabilitiesIsMarkedAsDated pins the header that stops the older
// survey being read as current status. Its per-type counts are still worth
// keeping; its conclusions are from Mendix 11.6.3 and mostly overtaken.
func TestMissingCapabilitiesIsMarkedAsDated(t *testing.T) {
	doc := readDoc(t, "MISSING_CAPABILITIES.md")

	// The warning must come before any per-type analysis, or a reader who lands
	// mid-document via grep (which is how #20 happened) never sees it.
	warn := strings.Index(doc, "Dated survey")
	if warn < 0 {
		t.Fatal("MISSING_CAPABILITIES.md no longer warns that it is a dated survey — " +
			"without that, grepping it for a document type reads as current status")
	}
	if first := strings.Index(doc, "## Summary"); first >= 0 && warn > first {
		t.Error("the dated-survey warning must precede the summary table")
	}
	if !strings.Contains(doc, "MDL_FEATURE_MATRIX.md") {
		t.Error("MISSING_CAPABILITIES.md must point at the canonical current status")
	}

	// Every row of the summary table needs a status cell, so no type can be read
	// as unsupported by omission.
	row := regexp.MustCompile(`(?m)^\| ` + "`" + `[A-Za-z]+\$?[A-Za-z]*` + "`" + ` \|.*$`)
	rows := row.FindAllString(doc, -1)
	if len(rows) < 10 {
		t.Fatalf("only matched %d summary rows; the guard is not looking at the table", len(rows))
	}
	for _, r := range rows {
		if !strings.Contains(r, "Supported") && !strings.Contains(r, "Still missing") &&
			!strings.Contains(r, "Read-only") {
			t.Errorf("summary row has no status cell:\n%s", r)
		}
	}
}

// TestMissingSyntaxTopicsAreActuallyMissing closes the hole that let Layouts be
// listed as "Read-only, no syntax topic" while `mxcli syntax layout` answered.
//
// This is a direct contradiction rather than a judgement call, which is why it
// can be asserted mechanically: the section states a topic does not exist, and
// the registry says it does. Deliberately narrower than the Skills/Examples gap
// lists, where an entry can be true at the same time as a syntax topic exists —
// Regular Expressions has a topic AND genuinely has no skill.
func TestMissingSyntaxTopicsAreActuallyMissing(t *testing.T) {
	matrix := readDoc(t, "MDL_FEATURE_MATRIX.md")

	start := strings.Index(matrix, "### Missing Syntax Topics")
	if start < 0 {
		t.Fatal(`MDL_FEATURE_MATRIX.md has no "### Missing Syntax Topics" section — ` +
			`if it was renamed, update this guard rather than deleting it`)
	}
	section := matrix[start:]
	if end := strings.Index(section, "\n### "); end > 0 {
		section = section[:end]
	}

	for _, c := range shippedCapabilities {
		if ByPath(c.topic) == nil {
			continue // covered by the sibling test, which reports it there
		}
		if strings.Contains(section, c.claim) {
			t.Errorf("MDL_FEATURE_MATRIX.md lists %q under \"Missing Syntax Topics\", "+
				"but `mxcli syntax %s` resolves — the doc tells a reader to look for "+
				"syntax that is already published", c.claim, c.topic)
		}
	}
}
