// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func finding(typ, name string, v Verdict, reason string) Finding {
	return Finding{Key: ElementKey{Type: typ, Name: name}, Verdict: v, Reason: reason}
}

func render(t *testing.T, d *DiffResult) string {
	t.Helper()
	var buf bytes.Buffer
	if err := d.WriteText(&buf); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	return buf.String()
}

// TestDiffResult_CleanModuleReadsAsVerified is the baseline: an untouched module
// must say so plainly, and must state how much was actually checked.
func TestDiffResult_CleanModuleReadsAsVerified(t *testing.T) {
	rep := &Report{Module: "Administration", Findings: []Finding{
		finding("ENTITY", "Account", Unchanged, ""),
		finding("PAGE", "Account_Overview", Unchanged, ""),
	}}
	d := NewDiffResult("Administration", "4.3.2", "11.12.1", rep)

	if d.LocallyModified {
		t.Error("an unchanged module must not be reported as locally modified")
	}
	if !d.Verified {
		t.Error("every element compared, so the result is verified")
	}
	if d.UnchangedCount != 2 {
		t.Errorf("UnchangedCount = %d, want 2", d.UnchangedCount)
	}

	out := render(t, d)
	for _, want := range []string{"Administration", "4.3.2", "11.12.1", "2 of 2 elements verified unchanged"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should mention %q:\n%s", want, out)
		}
	}
}

// TestDiffResult_UnknownIsNeverACleanBillOfHealth is the honesty rule, and the
// single most important assertion in this file.
//
// An element nobody could read is not an element that did not change. If the
// report renders "no modifications" without also saying the check was
// incomplete, a user upgrades on the strength of it and Studio Pro discards the
// edit that was hiding in the element we could not describe.
func TestDiffResult_UnknownIsNeverACleanBillOfHealth(t *testing.T) {
	rep := &Report{Module: "Administration", Findings: []Finding{
		finding("ENTITY", "Account", Unchanged, ""),
		finding("PAGE", "Account_Edit", Unknown, "not describable in the project: unsupported widget"),
	}}
	d := NewDiffResult("Administration", "4.3.2", "11.12.1", rep)

	if d.Verified {
		t.Fatal("Verified must be false when an element could not be read")
	}
	if len(d.Unknown) != 1 || d.Unknown[0].Element != "PAGE Account_Edit" {
		t.Fatalf("the unreadable element must be reported: %+v", d.Unknown)
	}
	if d.Unknown[0].Reason == "" {
		t.Error("an unknown element without a reason gives the user nothing to act on")
	}

	out := render(t, d)
	if strings.Contains(out, "verified unchanged") {
		t.Errorf("an incomplete check must not render as a clean verification:\n%s", out)
	}
	for _, want := range []string{"could not be read", "not a clean bill of health", "PAGE Account_Edit"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should mention %q:\n%s", want, out)
		}
	}
}

// TestDiffResult_ReportsEditsAndDeletionsSeparately checks the three ways a
// module can differ are told apart. "I changed it", "I added it" and "I deleted
// it" call for different decisions before an upgrade.
func TestDiffResult_ReportsEditsAndDeletionsSeparately(t *testing.T) {
	rep := &Report{Module: "Administration", Findings: []Finding{
		finding("ENTITY", "Account", Modified, ""),
		finding("MICROFLOW", "MyHelper", OnlyInstalled, ""),
		finding("PAGE", "Account_Overview", OnlyPackage, ""),
		finding("ENUMERATION", "Status", Unchanged, ""),
	}}
	d := NewDiffResult("Administration", "4.3.2", "11.12.1", rep)

	if !d.LocallyModified {
		t.Error("LocallyModified must be true when an element was changed")
	}
	for _, b := range []struct {
		bucket string
		got    []string
		want   string
	}{
		{"modified", d.Modified, "ENTITY Account"},
		{"onlyInstalled", d.OnlyInstalled, "MICROFLOW MyHelper"},
		{"onlyPackage", d.OnlyPackage, "PAGE Account_Overview"},
	} {
		if len(b.got) != 1 || b.got[0] != b.want {
			t.Errorf("%s = %v, want [%s]", b.bucket, b.got, b.want)
		}
	}

	out := render(t, d)
	for _, want := range []string{"changed   ENTITY Account", "added     MICROFLOW MyHelper", "removed   PAGE Account_Overview"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should mention %q:\n%s", want, out)
		}
	}
}

// TestDiffResult_JSONCarriesTheHeadlineFields guards the CI contract: a build
// gate reads locallyModified and verified, so both must always be present —
// including when false, which `omitempty` would silently drop.
func TestDiffResult_JSONCarriesTheHeadlineFields(t *testing.T) {
	d := NewDiffResult("Administration", "4.3.2", "11.12.1", &Report{
		Findings: []Finding{finding("ENTITY", "Account", Unchanged, "")},
	})

	var buf bytes.Buffer
	if err := d.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"module", "installedVersion", "mendixVersion", "locallyModified", "verified", "unchangedCount"} {
		if _, ok := got[key]; !ok {
			t.Errorf("JSON is missing %q, which a build gate needs:\n%s", key, buf.String())
		}
	}
	if got["locallyModified"] != false {
		t.Errorf("locallyModified should be false here, got %v", got["locallyModified"])
	}
}

// TestUpgradeImpactOf_FlagsOnlyTheCollisions separates "the author changed this"
// from "the author changed this and so did you". Only the second is a reason to
// stop, and Studio Pro's update reports neither.
func TestUpgradeImpactOf_FlagsOnlyTheCollisions(t *testing.T) {
	drift := &Report{Findings: []Finding{
		finding("ENTITY", "Account", Modified, ""),
		finding("PAGE", "Account_Edit", Unchanged, ""),
	}}
	authorChanges := &Report{Findings: []Finding{
		finding("ENTITY", "Account", Modified, ""),        // touched by both
		finding("MICROFLOW", "Login", Modified, ""),       // touched by the author only
		finding("PAGE", "Account_Edit", Unchanged, ""),    // untouched
		finding("ENUMERATION", "Status", OnlyPackage, ""), // new in the target
	}}

	imp := UpgradeImpactOf(drift, authorChanges)
	if want := []string{"ENTITY Account", "ENUMERATION Status", "MICROFLOW Login"}; strings.Join(imp.Touched, "|") != strings.Join(want, "|") {
		t.Errorf("Touched = %v, want %v", imp.Touched, want)
	}
	if len(imp.Conflicts) != 1 || imp.Conflicts[0] != "ENTITY Account" {
		t.Fatalf("Conflicts = %v, want just ENTITY Account", imp.Conflicts)
	}

	d := NewDiffResult("Administration", "4.3.2", "11.12.1", drift)
	d.TargetVersion = "4.5.0"
	d.Upgrade = imp
	out := render(t, d)
	for _, want := range []string{"Upgrading to 4.5.0", "CONFLICT  ENTITY Account", "discard those local edits"} {
		if !strings.Contains(out, want) {
			t.Errorf("output should mention %q:\n%s", want, out)
		}
	}
}

// TestUpgradeImpactOf_NoConflictsSaysSo — the common case for a module nobody
// edited should read as an all-clear, not as silence.
func TestUpgradeImpactOf_NoConflictsSaysSo(t *testing.T) {
	drift := &Report{Findings: []Finding{finding("ENTITY", "Account", Unchanged, "")}}
	imp := UpgradeImpactOf(drift, &Report{Findings: []Finding{
		finding("MICROFLOW", "Login", Modified, ""),
	}})
	if len(imp.Conflicts) != 0 {
		t.Fatalf("no local edits means no conflicts, got %v", imp.Conflicts)
	}

	d := NewDiffResult("Administration", "4.3.2", "11.12.1", drift)
	d.TargetVersion = "4.5.0"
	d.Upgrade = imp
	if out := render(t, d); !strings.Contains(out, "none of which you have modified") {
		t.Errorf("a conflict-free upgrade should say so:\n%s", out)
	}
}
