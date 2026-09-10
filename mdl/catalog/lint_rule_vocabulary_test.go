// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The bundled Starlark rules match on strings this package produces: action
// labels from getMicroflowActionType and reference kinds from the RefKind
// constants. Nothing connected the two, so a rule could name something the
// catalog never emits and simply match nothing — which is not a visible failure,
// it is a rule that silently flags everything or nothing.
//
// CONV010 shipped that way. Its ALLOWED_ACTIONS held the Mendix *storage* names
// (ShowFormAction, CloseFormAction) while the catalog reports the *SDK* names
// (ShowPageAction, ClosePageAction) — see the storage-name table in CLAUDE.md.
// The allowlist matched nothing, so every ACT_ microflow that showed a page,
// closed one, or called a sub-microflow was flagged: 11 false positives out of 13
// findings in the banking-app report, which buried the 2 real ones.
//
// QUAL004 had the sibling bug in the other vocabulary: it counted only the "call"
// and "schedule" reference kinds, so a microflow reached through a page data
// source, a widget action or a calculated attribute was reported as
// "not called from anywhere".

func lintRulesDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", ".claude", "lint-rules")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("bundled lint rules not present: %v", err)
	}
	return dir
}

func readRule(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(lintRulesDir(t), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// TestCONV010AllowsWhatTheCatalogCallsUIActions pins the rule's allowlist to the
// labels this package actually produces, by asking the labeller rather than
// hardcoding a second copy of the names.
func TestCONV010AllowsWhatTheCatalogCallsUIActions(t *testing.T) {
	src := readRule(t, "conv010_act_microflow_content.star")

	// The activities CONV010 documents as permitted in an ACT_ microflow.
	permitted := []microflows.MicroflowAction{
		&microflows.ShowPageAction{},
		&microflows.ClosePageAction{},
		&microflows.MicroflowCallAction{},
	}

	for _, action := range permitted {
		label := getMicroflowActionType(action)
		t.Run(label, func(t *testing.T) {
			if !strings.Contains(src, `"`+label+`"`) {
				t.Errorf("CONV010 does not allow %q, which is what the catalog labels this action.\n"+
					"Every ACT_ microflow using it will be flagged.", label)
			}
		})
	}
}

// starListItems returns the quoted strings of a top-level `NAME = [...]` or
// `NAME = (...)` literal in a Starlark source file.
func starListItems(src, name string) []string {
	re := regexp.MustCompile(`(?s)\b` + regexp.QuoteMeta(name) + `\s*=\s*[\[(](.*?)[\])]`)
	m := re.FindStringSubmatch(src)
	if m == nil {
		return nil
	}
	var out []string
	for _, q := range regexp.MustCompile(`"([^"]*)"`).FindAllStringSubmatch(m[1], -1) {
		out = append(out, q[1])
	}
	return out
}

// TestQUAL004EntryKindsAreRealRefKinds keeps the orphan rule's two kind lists
// spelled the way the reference builder emits them. A typo or an invented kind
// silently narrows the rule into false positives.
func TestQUAL004EntryKindsAreRealRefKinds(t *testing.T) {
	known := map[string]bool{}
	for _, k := range []string{
		RefKindCall, RefKindCreate, RefKindRetrieve, RefKindShowPage,
		RefKindGeneralize, RefKindAssociate, RefKindLayout, RefKindDatasource,
		RefKindParameter, RefKindAction, RefKindHomePage, RefKindLoginPage,
		RefKindMenuItem, RefKindChange, RefKindDelete, RefKindCalculate,
		RefKindReturn, RefKindSchedule, RefKindValidate, RefKindSettings,
	} {
		known[k] = true
	}

	src := readRule(t, "orphaned_elements.star")
	for _, listName := range []string{"MICROFLOW_ENTRY_KINDS", "PAGE_ENTRY_KINDS"} {
		items := starListItems(src, listName)
		if len(items) == 0 {
			t.Fatalf("%s not found in orphaned_elements.star, or is empty", listName)
		}
		for _, kind := range items {
			if !known[kind] {
				t.Errorf("%s lists %q, which the reference builder never emits", listName, kind)
			}
		}
	}
}

// The kinds that mean "this runs" / "this opens" must actually be listed. Losing
// one is how the rule regresses into reporting live documents as dead.
func TestQUAL004CountsEveryEntryPointKind(t *testing.T) {
	src := readRule(t, "orphaned_elements.star")

	for _, want := range []string{
		RefKindCall, RefKindSchedule, RefKindDatasource, RefKindAction, RefKindCalculate,
		RefKindSettings,
	} {
		if !contains(starListItems(src, "MICROFLOW_ENTRY_KINDS"), want) {
			t.Errorf("MICROFLOW_ENTRY_KINDS is missing %q — a microflow reached only that way "+
				"is reported as 'not called from anywhere'", want)
		}
	}
	for _, want := range []string{
		RefKindShowPage, RefKindHomePage, RefKindLoginPage, RefKindMenuItem,
	} {
		if !contains(starListItems(src, "PAGE_ENTRY_KINDS"), want) {
			t.Errorf("PAGE_ENTRY_KINDS is missing %q — a page reached only that way "+
				"is reported as orphaned", want)
		}
	}
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// An `if` in a microflow produces BOTH an ExclusiveSplit and the ExclusiveMerge
// that closes it. CONV010's ALLOWED_ACTIVITY_TYPES held the split and not the
// merge, so an ACT_ microflow that guards anything — "do not open a page with an
// empty parameter", the most ordinary thing an action microflow does — was
// flagged for its own closing brace while the branch it closes was permitted.
//
// Measured on a microflow whose only violation was the merge, and 122 times over
// on one real project (ako/CapTrackV4 R11).
//
// This package had already settled the question in the other direction:
// countMicroflowActivities excludes ExclusiveMerge as structural, with a comment
// saying so. CONV010 was the only place that treated it as business logic.
func TestCONV010AllowsBothHalvesOfAnIf(t *testing.T) {
	src := readRule(t, "conv010_act_microflow_content.star")
	allowed := starListItems(src, "ALLOWED_ACTIVITY_TYPES")

	split := getMicroflowObjectType(&microflows.ExclusiveSplit{})
	merge := getMicroflowObjectType(&microflows.ExclusiveMerge{})

	if !contains(allowed, split) {
		t.Fatalf("CONV010 does not allow %q — the control for the assertion below", split)
	}
	if !contains(allowed, merge) {
		t.Errorf("CONV010 allows %q but not %q, and an `if` emits both. Every guard in "+
			"an ACT_ microflow is reported for the join it cannot avoid creating.", split, merge)
	}
}

// CONTROL: allowing the merge must not quietly allow the rest of the structural
// vocabulary. A LOOP in an ACT_ microflow is business logic and CONV010 is right
// to flag it, so a fix that widened the list to "anything not an ActionActivity"
// would pass the test above and gut the rule.
func TestCONV010StillFlagsALoop(t *testing.T) {
	src := readRule(t, "conv010_act_microflow_content.star")
	allowed := starListItems(src, "ALLOWED_ACTIVITY_TYPES")

	for _, obj := range []microflows.MicroflowObject{
		&microflows.LoopedActivity{},
		&microflows.InheritanceSplit{},
	} {
		label := getMicroflowObjectType(obj)
		if contains(allowed, label) {
			t.Errorf("CONV010 now allows %q in an ACT_ microflow — that is business logic, "+
				"and the rule exists to move it to a SUB_ microflow", label)
		}
	}
}
