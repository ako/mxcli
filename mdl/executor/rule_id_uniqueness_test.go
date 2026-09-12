// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// One rule ID, one rule.
//
// A rule may be raised from several places in ITS OWN validator — MDL-WIDGET25
// fires from two branches of validate_widget_kind.go — so the invariant is not
// "one occurrence" but "one owner". Two validators sharing an ID give two
// unrelated errors the same name, which breaks the thing rule IDs exist for:
// looking one up, suppressing it, or matching it in a test.
//
// This exists because it happened. A new rule was numbered MDL-WIDGET27 while
// validate_widget_object_property.go already owned that number — the branch it
// was written on had not been updated in 18 commits, so `git grep` on the branch
// found the collision and the author did not run it.
//
// Its limit, which is the other half of that story: the branch was also behind
// MDL-WIDGET28, added upstream and not present locally at all. No test in a
// repository can see a rule that is not in it. Before claiming a NEW number,
// grep the default branch, not just the working tree.
var ruleIDPattern = regexp.MustCompile(`RuleID:\s+"([A-Z][A-Z0-9-]*)"`)

// ruleIDsSharedDeliberately are IDs raised by more than one validator today,
// each with the reason it has not been split. Renumbering a shipped rule is a
// user-visible change (it appears in output, in --format json and sarif, and in
// suppressions), so an existing overlap is recorded rather than fixed here.
var ruleIDsSharedDeliberately = map[string]string{
	"MDL-WIDGET21": "validate_widget_contentparams.go and validate_widget_editability.go " +
		"both report a property the widget does not honour; predates this test",
	"MDL059": "one rule, two sites: an annotation that parses and does nothing. " +
		"validate_flow_parameters.go covers one written on a PARAMETER, " +
		"validate_document_annotations.go one written before a CREATE. Someone " +
		"suppressing MDL059 means both, so splitting the number would be wrong",
}

func TestRuleIDHasOneOwner(t *testing.T) {
	owners := map[string]map[string]bool{}

	for _, dir := range []string{".", "../linter", "../linter/rules"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			for _, m := range ruleIDPattern.FindAllStringSubmatch(string(data), -1) {
				if owners[m[1]] == nil {
					owners[m[1]] = map[string]bool{}
				}
				owners[m[1]][filepath.Join(dir, name)] = true
			}
		}
	}
	if len(owners) == 0 {
		t.Fatal("found no rule IDs; this test no longer measures anything")
	}

	var clashes []string
	for id, files := range owners {
		if len(files) < 2 || ruleIDsSharedDeliberately[id] != "" {
			continue
		}
		var names []string
		for f := range files {
			names = append(names, f)
		}
		sort.Strings(names)
		clashes = append(clashes, id+" — "+strings.Join(names, ", "))
	}
	sort.Strings(clashes)
	if len(clashes) > 0 {
		t.Errorf("these rule IDs are raised by more than one validator:\n  %s\n"+
			"Give the newer rule the next free number — and check the DEFAULT BRANCH for it, "+
			"not just this working tree.", strings.Join(clashes, "\n  "))
	}

	// The exemption list must not outlive its reason, or it quietly re-opens the
	// hole it documents.
	var stale []string
	for id := range ruleIDsSharedDeliberately {
		if len(owners[id]) < 2 {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("ruleIDsSharedDeliberately lists %v, which no longer clash — strike them off", stale)
	}
}
