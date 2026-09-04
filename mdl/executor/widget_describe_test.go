// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// A widget was the only MDL extension point with no in-language DESCRIBE, which
// is why `widget init` had to generate documentation — and why that
// documentation could drift from what the parser accepts. The statement is only
// worth having if it answers without a project, which is the state an agent is
// in when it asks "what can I write here?".
func TestDescribeWidget_AnswersFromEmbeddedKnowledgeWithNoProject(t *testing.T) {
	desc, err := DescribeWidget("combobox", "")
	if err != nil {
		t.Fatalf("DescribeWidget with no project: %v", err)
	}
	if desc.WidgetID != "com.mendix.widget.web.combobox.Combobox" {
		t.Errorf("WidgetID = %q", desc.WidgetID)
	}
	if desc.Source != "embedded template" {
		t.Errorf("Source = %q, want the embedded fallback", desc.Source)
	}
	if len(desc.Properties) == 0 {
		t.Error("no properties — an empty description answers nothing")
	}
}

// The full widget id must work as well as the MDL keyword: it is what a widget
// package, a page's BSON and the generated docs all carry, and for a widget with
// no keyword it is the only name there is.
func TestDescribeWidget_AcceptsTheWidgetIdAsWellAsTheKeyword(t *testing.T) {
	byKeyword, err := DescribeWidget("combobox", "")
	if err != nil {
		t.Fatal(err)
	}
	byID, err := DescribeWidget("com.mendix.widget.web.combobox.Combobox", "")
	if err != nil {
		t.Fatal(err)
	}
	if byKeyword.WidgetID != byID.WidgetID {
		t.Errorf("keyword gave %q, id gave %q", byKeyword.WidgetID, byID.WidgetID)
	}
	if len(byKeyword.Properties) != len(byID.Properties) {
		t.Errorf("property counts differ: %d vs %d", len(byKeyword.Properties), len(byID.Properties))
	}
}

// An unknown widget must say so rather than returning an empty description that
// reads as "this widget has no properties".
func TestDescribeWidget_UnknownWidgetIsAnError(t *testing.T) {
	if _, err := DescribeWidget("notawidget", ""); err == nil {
		t.Fatal("want an error for an unknown widget, got none")
	}
}

// The project's installed .mpk is preferred over the embedded template, because
// it is version-accurate and is the only place a Marketplace widget appears.
// This is the control for the no-project test above: without it, "embedded
// template" there is equally consistent with the .mpk path never running.
func TestDescribeWidget_PrefersTheProjectPackageOverTheEmbeddedTemplate(t *testing.T) {
	desc, err := DescribeWidget("combobox", "../../testdata/expr-checker/minimal.mpr")
	if err != nil {
		t.Skipf("fixture unavailable: %v", err)
	}
	if desc.Source != "project .mpk" {
		t.Errorf("Source = %q, want the project package to win", desc.Source)
	}
}

// The rendered form is what a reader actually sees, and it is shared with
// `mxcli widget describe` — so a change that broke it would break both.
func TestPrintWidgetDescription_RendersTheHeaderAndProperties(t *testing.T) {
	desc, err := DescribeWidget("combobox", "")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	PrintWidgetDescription(&sb, *desc)
	out := sb.String()
	for _, want := range []string{"Widget:", "ID:", "Kind:", "Properties ("} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered output missing %q:\n%s", want, out)
		}
	}
}

// The containers are the half of a widget's shape that DESCRIBE WIDGET was
// missing relative to the generated .md — and the half whose MDL syntax is
// currently wrong for most widgets, so reporting them without saying which are
// reachable would repeat the .md's mistake.
//
// Gallery is used because it is an EMBEDDED definition carrying containers on
// both sides of the answer, so this needs no project and cannot skip. An
// earlier version pointed at the fixture project and skipped every run, since
// the fixture has no extracted defs — a test that only ever skips proves
// nothing (see #808 in fix-issue.md).
func TestDescribeWidget_ReportsContainersAndWhetherTheyAreAuthorable(t *testing.T) {
	desc, err := DescribeWidget("gallery", "")
	if err != nil {
		t.Fatalf("DescribeWidget(gallery): %v", err)
	}
	byKeyword := map[string]DescribedContainer{}
	for _, c := range desc.Containers {
		byKeyword[c.Keyword] = c
	}
	if len(byKeyword) == 0 {
		t.Fatal("no containers reported for a widget that has three")
	}

	// `emptyplaceholder` is not in the grammar's container vocabulary.
	if c, ok := byKeyword["emptyplaceholder"]; !ok {
		t.Errorf("emptyplaceholder missing; got %v", desc.Containers)
	} else if c.Authorable {
		t.Error("emptyplaceholder reported authorable, but it does not parse today")
	}

	// The control, in the same widget: `template` does parse. Without it,
	// "not authorable" above is equally consistent with a probe that always
	// fails — which is the risk of deriving the answer rather than listing it.
	if c, ok := byKeyword["template"]; !ok {
		t.Errorf("template missing; got %v", desc.Containers)
	} else if !c.Authorable {
		t.Error("template parses inside a widget body but was reported unauthorable")
	}
}

// Authorability is derived by parsing, never from a list. A list here would be
// the same defect the whole proposal is about, one layer up — so an invented
// keyword must come back false through the same path a real one comes back true.
func TestContainerKeywordParses_DerivesTheAnswerRatherThanListingIt(t *testing.T) {
	if !containerKeywordParses("group", false) {
		t.Error("group should parse as an object-list container")
	}
	if containerKeywordParses("definitelynotakeyword", false) {
		t.Error("an invented keyword must not report as authorable")
	}
}

// The example is the half of the generated .md that was WRONG — its version
// promised syntax that failed on its own first line. This one is built from
// probes, so the guarantee is testable: feed it back and it parses.
func TestDescribeWidget_ExampleParsesAsWritten(t *testing.T) {
	for _, w := range []string{"gallery", "combobox", "image"} {
		desc, err := DescribeWidget(w, "")
		if err != nil {
			t.Fatalf("DescribeWidget(%s): %v", w, err)
		}
		if desc.Example == "" {
			t.Errorf("%s: no example emitted", w)
			continue
		}
		if !pageBodyParses(desc.Example) {
			t.Errorf("%s: emitted example does not parse:\n%s", w, desc.Example)
		}
	}
}

// A container MDL cannot express must be left out AND named. Silently including
// it is what made the .md misleading rather than merely incomplete; silently
// dropping it would be almost as bad, since the reader would never learn the
// widget has it.
func TestDescribeWidget_ExampleOmitsUnauthorableContainersAndSaysSo(t *testing.T) {
	desc, err := DescribeWidget("gallery", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(desc.Example, "emptyplaceholder") {
		t.Errorf("example includes a container that cannot be written:\n%s", desc.Example)
	}
	var named bool
	for _, o := range desc.OmittedFromExample {
		if strings.Contains(o, "emptyplaceholder") {
			named = true
		}
	}
	if !named {
		t.Errorf("omitted container not named; got %v", desc.OmittedFromExample)
	}

	// The control: an authorable container IS included, so "omits things" is
	// not simply "omits everything".
	if !strings.Contains(desc.Example, "template") {
		t.Errorf("authorable container missing from the example:\n%s", desc.Example)
	}
}

// Two widgets sharing a name on one page is invalid, and the parser does not
// catch it — so the example has to number them itself. The .md generator had
// this same defect.
func TestDescribeWidget_ExampleNamesAreUnique(t *testing.T) {
	desc, err := DescribeWidget("gallery", "")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(desc.Example, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		name := fields[1]
		if !strings.HasPrefix(name, "slot") && !strings.HasPrefix(name, "item") {
			continue
		}
		if seen[name] {
			t.Errorf("duplicate widget name %q in example:\n%s", name, desc.Example)
		}
		seen[name] = true
	}
	if len(seen) < 2 {
		t.Skip("widget has fewer than two named containers; nothing to collide")
	}
}

// The head form is probed too: a widget whose keyword the grammar accepts uses
// it, and one whose keyword it does not falls back to the explicit-id form.
// Both halves in one test, so neither can pass vacuously.
func TestDescribeWidget_ExampleHeadFormFollowsTheGrammar(t *testing.T) {
	authorable, err := DescribeWidget("gallery", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(authorable.Example, "gallery widget1") {
		t.Errorf("gallery's keyword parses, so the example should use it:\n%s", authorable.Example)
	}

	notYet, err := DescribeWidget("image", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(notYet.Example, "pluggablewidget") == strings.HasPrefix(notYet.Example, "image widget1") {
		t.Fatalf("indeterminate head form:\n%s", notYet.Example)
	}
}

// A widget's properties are "required" only where the editor shows them. Combo
// box declares eleven bindings across mutually exclusive options-source modes,
// so listing them all overstates what a reader must supply — the same class of
// misinformation as the generated .md, quieter: not syntax that fails, but work
// that is not needed.
//
// Needs a project: the rules come from the .mpk's editorConfig, so the embedded
// template carries none. Asserted below rather than left to be discovered.
func TestDescribeWidget_BindingsHiddenUnderTheExampleAreNotAskedFor(t *testing.T) {
	const fixture = "../../testdata/expr-checker/minimal.mpr"
	desc, err := DescribeWidget("combobox", fixture)
	if err != nil {
		t.Fatalf("DescribeWidget(combobox, fixture): %v", err)
	}
	if len(desc.Rules) == 0 {
		t.Fatal("no visibility rules from the project .mpk — nothing could be pruned, so this would pass vacuously")
	}
	omitted := strings.Join(desc.OmittedFromExample, "; ")

	// attributeBoolean is hidden when optionsSourceType ≠ "boolean", and the
	// default is "association" — so the rule fires and it must not be asked for.
	if strings.Contains(omitted, "attributeBoolean") {
		t.Errorf("attributeBoolean is hidden under the example's configuration but was still asked for:\n%s", omitted)
	}
}

// The control. Pruning must not simply drop every binding: a widget whose
// datasource nothing hides still has to ask for it, or the example would look
// complete while being unusable.
func TestDescribeWidget_VisibleBindingsAreStillAskedFor(t *testing.T) {
	for _, w := range []string{"gallery", "datagrid"} {
		desc, err := DescribeWidget(w, "")
		if err != nil {
			t.Fatalf("%s: %v", w, err)
		}
		if !strings.Contains(strings.Join(desc.OmittedFromExample, "; "), "datasource") {
			t.Errorf("%s: its datasource is not hidden by any rule, so it must still be asked for; got %v",
				w, desc.OmittedFromExample)
		}
	}
}

// Conservatism, stated as a test: a rule whose condition property has no
// determinable value must NOT prune. Over-listing costs the reader a moment;
// hiding a binding they actually need sends them to a build error, which is the
// failure this area keeps producing.
func TestHiddenUnder_DoesNotPruneOnAnIndeterminableCondition(t *testing.T) {
	d := WidgetDescription{
		Properties: []DescribedProperty{
			{Key: "someBinding", Type: "datasource", Required: true},
			// `mode` has no default and no enum, so its value is unknowable.
			{Key: "mode", Type: "enumeration"},
		},
		Rules: []DescribedRule{{
			Property: "someBinding",
			Cond:     &types.WidgetVisibilityCondition{PropertyKey: "mode", Operator: "ne", Value: "x"},
		}},
	}
	if hiddenUnder(d, "someBinding") {
		t.Error("pruned on a condition whose value cannot be determined")
	}

	// The control: give `mode` a default the condition matches, and it prunes.
	d.Properties[1].Default = "y" // "y" != "x", so `ne` fires
	if !hiddenUnder(d, "someBinding") {
		t.Error("did not prune when the condition is determinable and fires")
	}
}

// A rule about an object-list ITEM's property must never prune the WIDGET's
// binding of the same name — they are different properties on different objects.
func TestHiddenUnder_IgnoresNestedItemRules(t *testing.T) {
	d := WidgetDescription{
		Properties: []DescribedProperty{
			{Key: "caption", Type: "attribute", Required: true},
			{Key: "mode", Type: "enumeration", Default: "y"},
		},
		Rules: []DescribedRule{{
			Property: "caption",
			Nested:   true,
			Cond:     &types.WidgetVisibilityCondition{PropertyKey: "mode", Operator: "ne", Value: "x"},
		}},
	}
	if hiddenUnder(d, "caption") {
		t.Error("a nested item rule pruned the widget's own binding")
	}
}

// The limitation behind the test above, stated so it is not rediscovered: with
// no project there is no .mpk, so no editorConfig, so no rules — and nothing to
// prune with. The description is still useful, it just cannot narrow the
// bindings.
func TestDescribeWidget_NoProjectMeansNoVisibilityRules(t *testing.T) {
	desc, err := DescribeWidget("combobox", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(desc.Rules) != 0 {
		t.Errorf("embedded combobox unexpectedly carries %d rules; the pruning test's project requirement may be stale", len(desc.Rules))
	}
}
