// SPDX-License-Identifier: Apache-2.0

package mpr

import "testing"

// TestListUnitsByType_MatchesExactly is the regression guard for the page /
// page-template conflation.
//
// listUnitsByType used to match on a type *prefix*, and Mendix storage names
// nest: `Forms$Page` is a prefix of `Forms$PageTemplate`. So ListPages returned
// both, `show modules` reported the fixture's Atlas_Web_Content as having 46
// pages when it has none, and every one of those templates described as a page
// with an empty body — the template's content hangs off LayoutCall, which the
// page path never reads. Anything comparing describe output therefore judged a
// template unchanged without having looked inside it.
//
// The assertion is deliberately about the *pair*: a test that only counted
// Forms$Page would pass against the prefix match too, because the miscount was
// caused by the other type being swept in.
func TestListUnitsByType_MatchesExactly(t *testing.T) {
	r, err := Open(copyProject(t, "../../testdata/expr-checker", "minimal.mpr"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	pages, err := r.listUnitsByType("Forms$Page")
	if err != nil {
		t.Fatalf("listUnitsByType(Forms$Page): %v", err)
	}
	templates, err := r.listUnitsByType("Forms$PageTemplate")
	if err != nil {
		t.Fatalf("listUnitsByType(Forms$PageTemplate): %v", err)
	}

	if len(pages) == 0 || len(templates) == 0 {
		t.Fatalf("fixture should hold both types; got %d pages, %d templates",
			len(pages), len(templates))
	}

	// Neither query may return a unit of the other type.
	for _, u := range pages {
		if u.Type != "Forms$Page" {
			t.Fatalf("querying Forms$Page returned a %s — the match is by prefix, not exact", u.Type)
		}
	}
	for _, u := range templates {
		if u.Type != "Forms$PageTemplate" {
			t.Fatalf("querying Forms$PageTemplate returned a %s", u.Type)
		}
	}

	// And the page query must not be the union of the two.
	all, err := r.listUnitsByType("")
	if err != nil {
		t.Fatalf("listUnitsByType(\"\"): %v", err)
	}
	if len(all) <= len(pages)+len(templates) {
		t.Fatalf("the empty type should return every unit; got %d, with %d pages + %d templates",
			len(all), len(pages), len(templates))
	}
}

// TestListPages_ExcludesPageTemplates checks the symptom the user actually sees,
// one layer up from the cause.
func TestListPages_ExcludesPageTemplates(t *testing.T) {
	r, err := Open(copyProject(t, "../../testdata/expr-checker", "minimal.mpr"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer r.Close()

	pageUnits, err := r.listUnitsByType("Forms$Page")
	if err != nil {
		t.Fatalf("listUnitsByType: %v", err)
	}
	pages, err := r.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != len(pageUnits) {
		t.Errorf("ListPages returned %d pages for %d Forms$Page units — page templates are being counted as pages",
			len(pages), len(pageUnits))
	}

	templates, err := r.ListPageTemplates()
	if err != nil {
		t.Fatalf("ListPageTemplates: %v", err)
	}
	if len(templates) == 0 {
		t.Fatal("page templates must still be readable under their own type")
	}
	byName := make(map[string]bool, len(pages))
	for _, p := range pages {
		byName[p.Name] = true
	}
	for _, tpl := range templates {
		if byName[tpl.Name] {
			t.Errorf("%q is reported as both a page and a page template", tpl.Name)
		}
	}
}
