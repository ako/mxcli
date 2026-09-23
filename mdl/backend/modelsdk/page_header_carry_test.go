// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#541 (header half) — a describe → exec round trip of a Studio Pro page
// reports "Replaced page", not "Unchanged page", because pageToGen writes three
// editor properties as constants instead of carrying the stored values:
//
//	Autofocus    Off  → DesktopOnly
//	CanvasWidth  800  → 1200
//
// These are not constants in Studio Pro. Measured across all 67 pages of
// ako/TestApp at 11.14.0:
//
//	CanvasWidth   800 ×33, 1198 ×20, 1200 ×4, 802 ×2, 900 ×2, 2000 ×1, 1800 ×1
//	Autofocus     DesktopOnly ×58, Off ×9
//	CanvasHeight  600 ×66, 500 ×1
//
// mxcli's hardcoded 1200 matches 4 of 67, so a round trip moved the canvas of 63
// of them. MDL has no spelling for any of the three and nobody asks to change
// them, which is what makes this carry-through rather than new syntax.
package modelsdkbackend

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/backend/bsonnav"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// storedHeader reads the three properties straight off the stored unit, rather
// than through GetPage — the semantic model does not carry them, which is the
// whole reason they were lost.
//
// The dimensions are read width-agnostically for the same reason the fix is:
// asserting one width here would re-encode the assumption under test.
func storedHeader(t *testing.T, b *Backend, id model.ID) (autofocus string, width, height int64) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return bsonnav.DGetString(d, "Autofocus"),
		bsonInt(bsonnav.DGet(d, "CanvasWidth")),
		bsonInt(bsonnav.DGet(d, "CanvasHeight"))
}

// setStoredHeader rewrites the three properties on the stored unit, standing in
// for a page Studio Pro authored with non-default editor state.
//
// The caller chooses the numeric width, because it is load-bearing: Studio Pro
// stores both canvas dimensions as int64, and the first version of this fix
// passed a test whose fixture wrote int32 while moving the canvas of every real
// document. See TestUpdatePage_CarriesStoredCanvasAtEitherWidth.
func setStoredHeader(t *testing.T, b *Backend, id model.ID, autofocus string, width, height any) {
	t.Helper()
	raw, err := b.reader.GetRawUnitBytes(string(id))
	if err != nil {
		t.Fatalf("GetRawUnitBytes: %v", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bsonnav.DSet(d, "Autofocus", autofocus)
	bsonnav.DSet(d, "CanvasWidth", width)
	bsonnav.DSet(d, "CanvasHeight", height)
	out, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := b.writer.UpdateRawUnit(string(id), out); err != nil {
		t.Fatalf("UpdateRawUnit: %v", err)
	}
}

func headerFixture(t *testing.T) (*Backend, *pages.Page) {
	t.Helper()
	b := New()
	if err := b.Connect(copyFixture(t)); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	page := &pages.Page{ContainerID: mod.ID, Name: "ZzHeaderPage", URL: "zz-header"}
	if err := b.CreatePage(page); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	return b, page
}

// A rewrite must leave the stored editor state alone.
func TestUpdatePage_CarriesStoredHeaderProperties(t *testing.T) {
	b, page := headerFixture(t)

	// Stand in for a Studio Pro page: values that differ from every mxcli
	// default, so nothing here can pass by coincidence, at the width Studio Pro
	// actually writes (int64, measured on all 67 TestApp pages).
	setStoredHeader(t, b, page.ID, "Off", int64(800), int64(500))

	page.URL = "zz-header-updated"
	if err := b.UpdatePage(page); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}

	autofocus, width, height := storedHeader(t, b, page.ID)
	if autofocus != "Off" {
		t.Errorf("Autofocus = %q, want Off — the stored value was overwritten", autofocus)
	}
	if width != 800 {
		t.Errorf("CanvasWidth = %d, want 800 — the stored value was overwritten", width)
	}
	if height != 500 {
		t.Errorf("CanvasHeight = %d, want 500 — the stored value was overwritten", height)
	}

	// Control: the property the statement DOES author still lands, so this is
	// not "the update stopped writing anything".
	got, err := b.GetPage(page.ID)
	if err != nil {
		t.Fatalf("GetPage: %v", err)
	}
	if got.URL != "zz-header-updated" {
		t.Errorf("URL = %q, want zz-header-updated — the authored change was lost", got.URL)
	}
}

// Control: a genuinely NEW page has no stored document to carry from, so it
// still gets the defaults. Without this, "carry the stored value" could be
// implemented as "never write these", leaving a new page with no canvas at all.
func TestCreatePage_StillWritesHeaderDefaults(t *testing.T) {
	b, page := headerFixture(t)

	autofocus, width, height := storedHeader(t, b, page.ID)
	if autofocus != "DesktopOnly" {
		t.Errorf("new page Autofocus = %q, want DesktopOnly", autofocus)
	}
	if width != 1200 {
		t.Errorf("new page CanvasWidth = %d, want 1200", width)
	}
	if height != 600 {
		t.Errorf("new page CanvasHeight = %d, want 600", height)
	}
}

// The width the value is stored at must not decide whether it is carried.
// Studio Pro writes int64; the gen setter takes int32; the natural assertion is
// therefore the wrong one, and it fails silently rather than loudly. This ran
// green against a fix that only handled int32 — which is exactly why it asserts
// both widths rather than the one the author expected.
func TestUpdatePage_CarriesStoredCanvasAtEitherWidth(t *testing.T) {
	for _, tc := range []struct {
		name  string
		width any
	}{
		{"int64 (what Studio Pro stores)", int64(1198)},
		{"int32", int32(1198)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, page := headerFixture(t)
			setStoredHeader(t, b, page.ID, "DesktopOnly", tc.width, int64(600))

			if err := b.UpdatePage(page); err != nil {
				t.Fatalf("UpdatePage: %v", err)
			}
			if _, width, _ := storedHeader(t, b, page.ID); width != 1198 {
				t.Errorf("CanvasWidth = %d, want 1198 — a %s value was not carried", width, tc.name)
			}
		})
	}
}
