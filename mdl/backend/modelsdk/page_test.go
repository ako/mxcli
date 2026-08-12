// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestReadSlice_Pages checks the page adapter: real pages carry a decoded title
// (requires the Texts$Text gen package to be registered), and page templates are
// NOT in the list.
//
// The exclusion is the point. Both engines used to return pages + templates
// together, because legacy asked the reader for "Forms$Page" and that query was
// prefix-matched — `Forms$PageTemplate` starts with `Forms$Page`. The modelsdk
// backend then bolted templates on deliberately to match. The fixture's
// Atlas_Web_Content has 46 templates and no pages, and reported 46 pages.
func TestReadSlice_Pages(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	pgs, err := b.ListPages()
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	// 16 Forms$Page in the fixture; the 46 Forms$PageTemplate units are not pages.
	if len(pgs) != 16 {
		t.Fatalf("ListPages count = %d, want 16 (pages only, no templates)", len(pgs))
	}

	var titled bool
	for _, p := range pgs {
		if p.Name == "Account_Edit" && p.Title != nil && p.Title.GetTranslation("en_US") == "Edit Account" {
			titled = true
		}
		if p.Name == "Blank" { // an Atlas page template
			t.Errorf("page template %q is being returned as a page", p.Name)
		}
	}
	if !titled {
		t.Error("Account_Edit title not decoded as 'Edit Account' (Texts$Text registration?)")
	}
}

// TestReadSlice_PageTemplates is the other half: templates are still readable,
// under their own type. Removing them from ListPages must not make them
// invisible — a document nothing can enumerate is worse than one filed wrong,
// because nothing reports its absence.
func TestReadSlice_PageTemplates(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	tmpls, err := b.ListPageTemplates()
	if err != nil {
		t.Fatalf("ListPageTemplates: %v", err)
	}
	if len(tmpls) != 46 {
		t.Fatalf("ListPageTemplates count = %d, want 46", len(tmpls))
	}
	var found bool
	for _, pt := range tmpls {
		if pt.Name == "Blank" {
			found = true
			if pt.ContainerID == "" {
				t.Error("template has no container, so its module cannot be resolved")
			}
		}
	}
	if !found {
		t.Error("the Blank page template was not read")
	}
}
