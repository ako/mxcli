// SPDX-License-Identifier: Apache-2.0

package marketplace

import "testing"

func el(t, n, mdl string) Element {
	return Element{Key: ElementKey{Type: t, Name: n}, MDL: mdl}
}

func errEl(t, n, why string) Element {
	return Element{Key: ElementKey{Type: t, Name: n}, Err: why}
}

func snap(module string, els ...Element) *Snapshot {
	s := &Snapshot{Module: module, Elements: map[ElementKey]Element{}}
	for _, e := range els {
		s.Elements[e.Key] = e
	}
	return s
}

func findingFor(t *testing.T, r *Report, typ, name string) Finding {
	t.Helper()
	for _, f := range r.Findings {
		if f.Key.Type == typ && f.Key.Name == name {
			return f
		}
	}
	t.Fatalf("no finding for %s %s in %+v", typ, name, r.Findings)
	return Finding{}
}

func TestCompare_Verdicts(t *testing.T) {
	installed := snap("Administration",
		el("PAGE", "Account_Overview", "create page A;"),
		el("PAGE", "Account_Edit", "create page EDITED;"),
		el("MICROFLOW", "LocalOnly", "create microflow L;"),
	)
	pkg := snap("Administration",
		el("PAGE", "Account_Overview", "create page A;"),
		el("PAGE", "Account_Edit", "create page ORIGINAL;"),
		el("MICROFLOW", "PackageOnly", "create microflow P;"),
	)

	rep := Compare(installed, pkg)

	if got := findingFor(t, rep, "PAGE", "Account_Overview").Verdict; got != Unchanged {
		t.Errorf("identical MDL should be unchanged, got %s", got)
	}
	if got := findingFor(t, rep, "PAGE", "Account_Edit").Verdict; got != Modified {
		t.Errorf("differing MDL should be modified, got %s", got)
	}
	if got := findingFor(t, rep, "MICROFLOW", "LocalOnly").Verdict; got != OnlyInstalled {
		t.Errorf("element added by the user should be only-installed, got %s", got)
	}
	if got := findingFor(t, rep, "MICROFLOW", "PackageOnly").Verdict; got != OnlyPackage {
		t.Errorf("element missing from the project should be only-package, got %s", got)
	}

	if !rep.LocallyModified() {
		t.Error("a modified element must make the module count as locally modified")
	}
	if rep.Clean() {
		t.Error("a module with modifications must not report clean")
	}
}

// TestCompare_UnknownIsNeverClean is the honesty rule, and the single most
// important behaviour here: an element that cannot be described must not be
// silently treated as unchanged. Getting this wrong would tell a user their
// module is untouched when the tool simply could not look.
func TestCompare_UnknownIsNeverClean(t *testing.T) {
	installed := snap("M",
		el("PAGE", "Same", "x"),
		errEl("MENU", "Opaque", "no DESCRIBE support for MENU"),
	)
	pkg := snap("M",
		el("PAGE", "Same", "x"),
		errEl("MENU", "Opaque", "no DESCRIBE support for MENU"),
	)

	rep := Compare(installed, pkg)

	f := findingFor(t, rep, "MENU", "Opaque")
	if f.Verdict != Unknown {
		t.Fatalf("an un-describable element must be unknown, got %s", f.Verdict)
	}
	if f.Reason == "" {
		t.Error("an unknown verdict must say why, or the user cannot judge the risk")
	}
	if rep.Clean() {
		t.Error("a module containing an unknown element must not report clean — " +
			"'we could not tell' is not 'nothing changed'")
	}
	// It is not evidence of modification either.
	if rep.LocallyModified() {
		t.Error("unknown must not be reported as a local modification either")
	}
}

// TestCompare_UnknownWhenOneSideFails covers the asymmetric case: describable
// in the project, not in the package (or vice versa). Comparing a real
// description against nothing would report a spurious modification.
func TestCompare_UnknownWhenOneSideFails(t *testing.T) {
	installed := snap("M", el("PAGE", "P", "create page P;"))
	pkg := snap("M", errEl("PAGE", "P", "boom"))

	f := findingFor(t, Compare(installed, pkg), "PAGE", "P")
	if f.Verdict != Unknown {
		t.Errorf("one unreadable side must yield unknown, not a spurious diff; got %s", f.Verdict)
	}
	if f.Reason == "" {
		t.Error("expected a reason naming which side failed")
	}
}

// TestCompare_IdenticalSnapshotsAreClean is the control: a module compared with
// itself must report no drift at all. If this ever fails, the normaliser or the
// describe path is non-deterministic and every other result is noise.
func TestCompare_IdenticalSnapshotsAreClean(t *testing.T) {
	s := snap("M",
		el("PAGE", "A", "create page A;"),
		el("MICROFLOW", "B", "create microflow B;"),
		el("ENTITY", "C", "create entity C;"),
	)

	rep := Compare(s, s)
	if !rep.Clean() {
		t.Fatalf("a module compared with itself must be clean, got %+v", rep.Findings)
	}
	if rep.LocallyModified() {
		t.Error("a module compared with itself must not report modifications")
	}
	if n := rep.Counts()[Unchanged]; n != 3 {
		t.Errorf("expected 3 unchanged, got %d", n)
	}
}

// TestCompare_SameNameDifferentTypesDoNotCollide guards the join key. A module
// may hold a page and a microflow of the same name; keying on name alone would
// compare one against the other and report both as modified.
func TestCompare_SameNameDifferentTypesDoNotCollide(t *testing.T) {
	installed := snap("M",
		el("PAGE", "Overview", "page body"),
		el("MICROFLOW", "Overview", "microflow body"),
	)
	pkg := snap("M",
		el("PAGE", "Overview", "page body"),
		el("MICROFLOW", "Overview", "microflow body"),
	)

	rep := Compare(installed, pkg)
	if !rep.Clean() {
		t.Errorf("same-named elements of different types must not collide, got %+v", rep.Findings)
	}
}

func TestNormalizeMDL_IgnoresOnlyIncidentalFormatting(t *testing.T) {
	a := normalizeMDL("create page P;\n\n   \n  title: 'x';   \n")
	b := normalizeMDL("create page P;\n  title: 'x';\n\n")
	if a != b {
		t.Errorf("blank lines and trailing spaces must not read as a difference:\n%q\nvs\n%q", a, b)
	}

	// Indentation is meaningful in describe output (it shows nesting), so it must
	// survive normalisation.
	if normalizeMDL("  nested") == normalizeMDL("nested") {
		t.Error("leading indentation must be preserved — it encodes widget nesting")
	}
}
