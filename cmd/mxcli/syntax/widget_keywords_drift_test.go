// SPDX-License-Identifier: Apache-2.0

package syntax

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// mxcli-formula1 FINDINGS §69: an Admin screen was built as five separate pages
// with a hand-rolled chip row and a bespoke `.admin-nav` SCSS block, because the
// author had concluded MDL has no tab container.
//
// It has had one all along:
//
//	tabcontainer tabs {
//	  tabpage tpA (caption: 'Cycle runs') { ... }
//	  tabpage tpB (caption: 'Laps')       { ... }
//	}
//
// TABCONTAINER and TABPAGE are lexer keywords, buildWidgetV3 emits a proper
// Forms$TabControl with Forms$TabPage children, and `mx check` reports 0 errors.
//
// How the wrong conclusion was reached, in the reporter's own account: they
// searched `mxcli syntax page widgets --json` for TABCONTAINER, got nothing, and
// wrote "MDL has no tab container" into a design decision and twice into their
// findings. Two days later a twenty-second probe settled it.
//
// The lesson they drew — "absence from the documentation is not absence from the
// grammar" — is true, and is a bad thing for the documentation to require. This
// test makes it false instead: every widget keyword the grammar accepts must
// appear in a `page.*` syntax topic, so a keyword added to the parser cannot
// ship undocumented.
//
// The GRAMMAR is the authority here, deliberately, because it is what the
// reporter was told to consult instead.

// widgetTypeAlternatives reads the widgetTypeV3 rule out of the committed
// grammar. Only the generated parser is uncommitted; the .g4 sources are in the
// repository, which is what makes this checkable at test time.
func widgetTypeAlternatives(t *testing.T) []string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "mdl", "grammar", "domains", "MDLPage.g4")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	rule := regexp.MustCompile(`(?s)\nwidgetTypeV3\s*\n\s*:(.*?)\n\s*;`).FindStringSubmatch(string(b))
	if rule == nil {
		t.Fatal("widgetTypeV3 rule not found — if the rule was renamed, re-point this guard " +
			"rather than deleting it; it exists because a keyword shipped undocumented for two days")
	}
	var out []string
	for _, line := range strings.Split(rule[1], "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		for _, tok := range strings.Split(line, "|") {
			if tok = strings.TrimSpace(tok); regexp.MustCompile(`^[A-Z][A-Z0-9]*$`).MatchString(tok) {
				out = append(out, strings.ToLower(tok))
			}
		}
	}
	if len(out) < 20 {
		t.Fatalf("parsed only %d alternatives (%v) — the extraction is wrong, not the docs", len(out), out)
	}
	sort.Strings(out)
	return out
}

// documentedElsewhere are widget keywords that are not page widgets, each with
// the topic that does own them. They are exempt from page.* but NOT from being
// documented: an entry with no home is the same defect this test is about.
var documentedElsewhere = map[string]string{
	// Layout structure — a layout's vocabulary, not a page's. See the
	// write-layouts skill and `mxcli syntax layout`.
	"scrollcontainer": "layout",
	"scrollregion":    "layout",
	"navigationtree":  "layout",
	"menubar":         "layout",
	"placeholder":     "layout",
	// Object-list container keywords for pluggable widgets: each is the singular
	// form of one widget's own list property (Accordion groups → GROUP), routed
	// through that widget's def.json rather than being a widget in its own right.
	"group":             "pluggable-widget object list",
	"customitem":        "pluggable-widget object list",
	"marker":            "pluggable-widget object list",
	"dynamicmarker":     "pluggable-widget object list",
	"series":            "pluggable-widget object list",
	"line":              "pluggable-widget object list",
	"scalecolor":        "pluggable-widget object list",
	"custombutton":      "pluggable-widget object list",
	"allowedfileformat": "pluggable-widget object list",
}

// pageSyntaxCorpus is every page.* topic's text, which is where a page widget
// keyword has to be findable.
func pageSyntaxCorpus() string {
	var b strings.Builder
	for _, f := range All() {
		if f.Path == "page" || strings.HasPrefix(f.Path, "page.") {
			b.WriteString(strings.ToLower(f.Syntax))
			b.WriteString("\n")
			b.WriteString(strings.ToLower(f.Example))
			b.WriteString("\n")
			b.WriteString(strings.ToLower(strings.Join(f.Keywords, " ")))
			b.WriteString("\n")
		}
	}
	return b.String()
}

func TestEveryWidgetKeywordIsInAPageSyntaxTopic(t *testing.T) {
	corpus := pageSyntaxCorpus()
	if corpus == "" {
		t.Fatal("no page.* topics registered — the guard would pass vacuously")
	}

	var missing []string
	for _, kw := range widgetTypeAlternatives(t) {
		if _, exempt := documentedElsewhere[kw]; exempt {
			continue
		}
		if !regexp.MustCompile(`\b` + regexp.QuoteMeta(kw) + `\b`).MatchString(corpus) {
			missing = append(missing, kw)
		}
	}
	if len(missing) > 0 {
		t.Errorf("the grammar accepts %d widget keyword(s) that no page.* syntax topic mentions:\n  %s\n\n"+
			"Someone will conclude MDL cannot do these and build around them — that is exactly what "+
			"happened with `tabcontainer`, for two days. Add them to cmd/mxcli/syntax/features_page.go, "+
			"or to documentedElsewhere with the topic that owns them.",
			len(missing), strings.Join(missing, ", "))
	}
}

// The reported keyword specifically, so the regression has a name.
func TestTabContainerIsDocumented(t *testing.T) {
	corpus := pageSyntaxCorpus()
	for _, kw := range []string{"tabcontainer", "tabpage"} {
		if !strings.Contains(corpus, kw) {
			t.Errorf("%s is a lexer keyword the builder emits Forms$TabControl for, and no page.* "+
				"topic mentions it", kw)
		}
	}
}

// CONTROL: the guard must be able to fail. A keyword the grammar does not have
// is not in the corpus either — if this "finds" one, the matcher is matching
// substrings and every real check above is meaningless.
func TestWidgetKeywordGuardCanFail(t *testing.T) {
	corpus := pageSyntaxCorpus()
	if regexp.MustCompile(`\bnosuchwidgetkeyword\b`).MatchString(corpus) {
		t.Error("the corpus matched a keyword that does not exist")
	}
	// …and a keyword that IS documented must match, or the guard passes because
	// the matcher never matches anything.
	if !regexp.MustCompile(`\bdataview\b`).MatchString(corpus) {
		t.Error("dataview is documented but did not match — the matcher is broken, so the " +
			"whole guard is vacuous")
	}
}
