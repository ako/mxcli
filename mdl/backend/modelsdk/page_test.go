// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// TestReadSlice_Pages checks the page adapter: real pages carry a decoded title
// (requires the Texts$Text gen package to be registered) and the list includes
// page templates, matching legacy's prefix-matched ListPages. SHOW PAGES is
// cross-checked byte-for-byte against the legacy engine in the plan validation.
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
	// 16 Forms$Page + 46 Forms$PageTemplate in the fixture.
	if len(pgs) != 62 {
		t.Fatalf("ListPages count = %d, want 62 (pages + templates)", len(pgs))
	}

	var titled, template bool
	for _, p := range pgs {
		if p.Name == "Account_Edit" && p.Title != nil && p.Title.GetTranslation("en_US") == "Edit Account" {
			titled = true
		}
		if p.Name == "Blank" { // an Atlas page template
			template = true
		}
	}
	if !titled {
		t.Error("Account_Edit title not decoded as 'Edit Account' (Texts$Text registration?)")
	}
	if !template {
		t.Error("page templates not included in ListPages")
	}
}
