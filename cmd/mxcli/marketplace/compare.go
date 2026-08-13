// SPDX-License-Identifier: Apache-2.0

package marketplace

import "sort"

// Verdict is what the comparison concluded about one element.
type Verdict string

const (
	// Unchanged: both sides described identically.
	Unchanged Verdict = "unchanged"
	// Modified: both sides described, and the MDL differs.
	Modified Verdict = "modified"
	// OnlyInstalled: present in the project, absent from the package.
	OnlyInstalled Verdict = "only-installed"
	// OnlyPackage: present in the package, absent from the project.
	OnlyPackage Verdict = "only-package"
	// Unknown: at least one side could not be described, so nothing can be
	// concluded. Never collapsed into Unchanged — an un-describable element
	// reported as clean is the failure mode that would make this dangerous
	// rather than merely incomplete.
	Unknown Verdict = "unknown"
)

// Finding is the comparison's conclusion for one element.
type Finding struct {
	Key     ElementKey
	Verdict Verdict
	// Reason explains an Unknown verdict; empty otherwise.
	Reason string
	// InstalledMDL / PackageMDL are the two descriptions, carried so a caller can
	// show the actual difference. Empty where the side is absent or unreadable.
	InstalledMDL string
	PackageMDL   string
}

// Report is the full comparison of an installed module against a package.
type Report struct {
	Module   string
	Findings []Finding
}

// Counts tallies the report by verdict.
func (r *Report) Counts() map[Verdict]int {
	out := make(map[Verdict]int, 5)
	for _, f := range r.Findings {
		out[f.Verdict]++
	}
	return out
}

// LocallyModified reports whether anything was changed in the project relative
// to the package. Unknown does not count as modified — but it does not count as
// clean either, which is why Clean is a separate question.
func (r *Report) LocallyModified() bool {
	for _, f := range r.Findings {
		if f.Verdict == Modified || f.Verdict == OnlyInstalled {
			return true
		}
	}
	return false
}

// Clean reports whether the module can be said to be untouched. It requires
// every element to be positively verified as unchanged: a single Unknown makes
// the answer "we cannot tell", which is deliberately not the same as yes.
func (r *Report) Clean() bool {
	for _, f := range r.Findings {
		if f.Verdict != Unchanged {
			return false
		}
	}
	return true
}

// Compare matches two snapshots of the same module by name+type and classifies
// each element.
//
// installed is the copy in the user's project; pkg is the copy built from the
// published marketplace package. The asymmetry matters for reporting: an element
// only in the project is something the user added, while one only in the package
// is something they deleted (or the version differs).
func Compare(installed, pkg *Snapshot) *Report {
	rep := &Report{Module: installed.Module}

	seen := make(map[ElementKey]bool, len(installed.Elements)+len(pkg.Elements))
	keys := make([]ElementKey, 0, len(seen))
	for _, k := range installed.Keys() {
		seen[k] = true
		keys = append(keys, k)
	}
	for _, k := range pkg.Keys() {
		if !seen[k] {
			keys = append(keys, k)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Type != keys[j].Type {
			return keys[i].Type < keys[j].Type
		}
		return keys[i].Name < keys[j].Name
	})

	for _, k := range keys {
		rep.Findings = append(rep.Findings, classify(k, installed.Elements[k], pkg.Elements[k],
			hasKey(installed, k), hasKey(pkg, k)))
	}
	return rep
}

func hasKey(s *Snapshot, k ElementKey) bool {
	_, ok := s.Elements[k]
	return ok
}

func classify(k ElementKey, inst, pkg Element, hasInst, hasPkg bool) Finding {
	switch {
	case hasInst && !hasPkg:
		// An element the user added is a local modification even though there is
		// nothing to compare it against, so it is not Unknown.
		return Finding{Key: k, Verdict: OnlyInstalled, InstalledMDL: inst.MDL}
	case !hasInst && hasPkg:
		return Finding{Key: k, Verdict: OnlyPackage, PackageMDL: pkg.MDL}
	}

	// Present on both sides: it can only be compared if both described.
	if !inst.Describable() || !pkg.Describable() {
		return Finding{Key: k, Verdict: Unknown, Reason: unknownReason(inst, pkg)}
	}
	// Identical text is solid evidence of "unchanged" whatever the type, so this
	// is checked before asking whether the output is conclusive — otherwise every
	// building block in Atlas would be reported as unknown for no reason.
	if inst.MDL == pkg.MDL {
		return Finding{Key: k, Verdict: Unchanged}
	}
	// They differ — but a difference is only evidence of an edit if the output
	// could carry one. See Element.Conclusive.
	if ok, why := inst.Conclusive(); !ok {
		return Finding{Key: k, Verdict: Unknown, Reason: why}
	}
	if ok, why := pkg.Conclusive(); !ok {
		return Finding{Key: k, Verdict: Unknown, Reason: why}
	}
	return Finding{Key: k, Verdict: Modified, InstalledMDL: inst.MDL, PackageMDL: pkg.MDL}
}

func unknownReason(inst, pkg Element) string {
	switch {
	case !inst.Describable() && !pkg.Describable():
		return "not describable on either side: " + inst.Err
	case !inst.Describable():
		return "not describable in the project: " + inst.Err
	default:
		return "not describable in the package: " + pkg.Err
	}
}
