// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// DiffResult is everything `marketplace diff` concluded, in the shape the JSON
// output uses. It is a distinct type from Report so the wire format is decided
// here rather than leaking whatever the comparator happens to hold.
type DiffResult struct {
	Module           string `json:"module"`
	InstalledVersion string `json:"installedVersion"`
	MendixVersion    string `json:"mendixVersion"`
	// TargetVersion is set only when --to was given.
	TargetVersion string `json:"targetVersion,omitempty"`

	// LocallyModified is the headline answer: has anyone changed this module
	// since it was installed?
	LocallyModified bool `json:"locallyModified"`
	// Verified is true only when every element was positively compared. False
	// means at least one element could not be read, so "unmodified" is not a
	// conclusion that can be drawn.
	Verified bool `json:"verified"`

	Modified       []string            `json:"modified,omitempty"`
	OnlyInstalled  []string            `json:"onlyInstalled,omitempty"`
	OnlyPackage    []string            `json:"onlyPackage,omitempty"`
	Unknown        []UnreadableElement `json:"unknown,omitempty"`
	UnchangedCount int                 `json:"unchangedCount"`

	// Upgrade is populated when --to was given: what changes between the
	// installed version and the target, and which of those the user has edited.
	Upgrade *UpgradeImpact `json:"upgrade,omitempty"`
}

// UnreadableElement is an element the comparison could not read, with the reason.
type UnreadableElement struct {
	Element string `json:"element"`
	Reason  string `json:"reason"`
}

// UpgradeImpact answers "what would upgrading touch, and does it collide with
// what I changed?".
type UpgradeImpact struct {
	// Touched is what differs between the installed version's package and the
	// target version's package — i.e. what the module author changed.
	Touched []string `json:"touched"`
	// Conflicts are elements the author changed *and* the user changed. These are
	// the ones an upgrade would silently destroy, which is what Studio Pro does.
	Conflicts []string `json:"conflicts"`
}

// NewDiffResult projects a comparison into the reported shape.
func NewDiffResult(module, installedVersion, mendixVersion string, rep *Report) *DiffResult {
	out := &DiffResult{
		Module:           module,
		InstalledVersion: installedVersion,
		MendixVersion:    mendixVersion,
		LocallyModified:  rep.LocallyModified(),
		Verified:         true,
	}
	for _, f := range rep.Findings {
		switch f.Verdict {
		case Unchanged:
			out.UnchangedCount++
		case Modified:
			out.Modified = append(out.Modified, f.Key.String())
		case OnlyInstalled:
			out.OnlyInstalled = append(out.OnlyInstalled, f.Key.String())
		case OnlyPackage:
			out.OnlyPackage = append(out.OnlyPackage, f.Key.String())
		case Unknown:
			out.Verified = false
			out.Unknown = append(out.Unknown, UnreadableElement{Element: f.Key.String(), Reason: f.Reason})
		}
	}
	sort.Strings(out.Modified)
	sort.Strings(out.OnlyInstalled)
	sort.Strings(out.OnlyPackage)
	return out
}

// WriteJSON emits the machine-readable form, for "fail the build if a
// marketplace module was edited" in CI.
func (d *DiffResult) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// WriteText emits the human form.
//
// The summary line states the limit of what was checked, not just the result.
// "27 of 27 elements verified unchanged" and "26 of 27 verified, 1 could not be
// read" are different claims, and a user deciding whether to risk a destructive
// upgrade needs the second one to look different from the first.
func (d *DiffResult) WriteText(w io.Writer) error {
	fmt.Fprintf(w, "%s — installed %s (Mendix %s)\n", d.Module, orNone(d.InstalledVersion), orNone(d.MendixVersion))

	total := d.UnchangedCount + len(d.Modified) + len(d.OnlyInstalled) + len(d.OnlyPackage) + len(d.Unknown)

	switch {
	case !d.LocallyModified && d.Verified:
		fmt.Fprintf(w, "\n  No local modifications: %d of %d elements verified unchanged.\n", d.UnchangedCount, total)
	case !d.LocallyModified:
		fmt.Fprintf(w, "\n  No local modifications found, but %d of %d elements could not be read —\n"+
			"  this is not a clean bill of health.\n", len(d.Unknown), total)
	default:
		fmt.Fprintf(w, "\n  Locally modified (%d of %d elements):\n",
			len(d.Modified)+len(d.OnlyInstalled), total)
		for _, e := range d.Modified {
			fmt.Fprintf(w, "    changed   %s\n", e)
		}
		for _, e := range d.OnlyInstalled {
			fmt.Fprintf(w, "    added     %s\n", e)
		}
	}

	if len(d.OnlyPackage) > 0 {
		fmt.Fprintf(w, "\n  Missing from the project (%d) — removed locally, or the package moved on:\n", len(d.OnlyPackage))
		for _, e := range d.OnlyPackage {
			fmt.Fprintf(w, "    removed   %s\n", e)
		}
	}

	if len(d.Unknown) > 0 {
		fmt.Fprintf(w, "\n  Not comparable (%d) — reported as unknown, never as unchanged:\n", len(d.Unknown))
		for _, u := range d.Unknown {
			fmt.Fprintf(w, "    unknown   %s (%s)\n", u.Element, u.Reason)
		}
	}

	if d.Upgrade != nil {
		fmt.Fprintf(w, "\n  Upgrading to %s would touch %d element(s)", d.TargetVersion, len(d.Upgrade.Touched))
		if len(d.Upgrade.Conflicts) == 0 {
			fmt.Fprintln(w, ", none of which you have modified.")
		} else {
			fmt.Fprintf(w, ", %d of which you have modified:\n", len(d.Upgrade.Conflicts))
			for _, e := range d.Upgrade.Conflicts {
				fmt.Fprintf(w, "    CONFLICT  %s\n", e)
			}
			fmt.Fprintln(w, "\n  Studio Pro's update would discard those local edits without asking.")
		}
	}

	fmt.Fprintln(w)
	return nil
}

func orNone(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

// UpgradeImpactOf works out what an upgrade would touch and where it collides
// with local edits.
//
// installedVsPackage is the drift report (project vs the version it was
// installed from); packageVsTarget compares the installed version's package
// against the target version's — the module author's own changes, uncontaminated
// by anything the user did.
func UpgradeImpactOf(installedVsPackage, packageVsTarget *Report) *UpgradeImpact {
	modified := map[string]bool{}
	for _, f := range installedVsPackage.Findings {
		if f.Verdict == Modified {
			modified[f.Key.String()] = true
		}
	}

	out := &UpgradeImpact{}
	for _, f := range packageVsTarget.Findings {
		if f.Verdict == Unchanged {
			continue
		}
		name := f.Key.String()
		out.Touched = append(out.Touched, name)
		if modified[name] {
			out.Conflicts = append(out.Conflicts, name)
		}
	}
	sort.Strings(out.Touched)
	sort.Strings(out.Conflicts)
	return out
}
