// SPDX-License-Identifier: Apache-2.0

package translations

import (
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// The point of the walk: it names the site a text lives at without knowing
// anything about the document type. A widget caption and a page title are found
// by the same code, which is what the hand-written catalog extractor could not do.
func TestSitesIn_NamesTheOwnerAndPropertyOfEveryText(t *testing.T) {
	pageID := "11111111-1111-1111-1111-111111111111"
	btnID := "22222222-2222-2222-2222-222222222222"

	doc := bson.D{
		{Key: "$ID", Value: mpr.IDToBsonBinary(pageID)},
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Title", Value: text(tr("en_US", "Home"))},
		{Key: "Widgets", Value: bson.A{
			bson.D{
				{Key: "$ID", Value: mpr.IDToBsonBinary(btnID)},
				{Key: "$Type", Value: "Forms$ActionButton"},
				{Key: "Caption", Value: text(tr("en_US", "Save"), tr("nl_NL", "Opslaan"))},
				{Key: "Tooltip", Value: text(tr("en_US", "Save this record"))},
			},
		}},
	}

	got := SitesIn(doc)
	if len(got) != 3 {
		t.Fatalf("want 3 sites, got %d: %+v", len(got), got)
	}

	byProp := map[string]Site{}
	for _, s := range got {
		byProp[s.OwnerType+"."+s.Property] = s
	}

	title, ok := byProp["Forms$Page.Title"]
	if !ok {
		t.Fatalf("page title site missing; got %v", siteKeys(byProp))
	}
	if title.Targets["en_US"] != "Home" {
		t.Errorf("title en_US = %q, want %q", title.Targets["en_US"], "Home")
	}
	if title.ElementID != pageID {
		t.Errorf("title ElementID = %q, want the page's %q", title.ElementID, pageID)
	}

	// The caption is the one the hand-written builder never reached.
	capt, ok := byProp["Forms$ActionButton.Caption"]
	if !ok {
		t.Fatalf("action button caption site missing; got %v", siteKeys(byProp))
	}
	if capt.Targets["nl_NL"] != "Opslaan" {
		t.Errorf("caption nl_NL = %q, want %q", capt.Targets["nl_NL"], "Opslaan")
	}
	if capt.ElementID != btnID {
		t.Errorf("caption ElementID = %q, want the button's %q, not the page's", capt.ElementID, btnID)
	}

	if _, ok := byProp["Forms$ActionButton.Tooltip"]; !ok {
		t.Errorf("tooltip site missing; got %v", siteKeys(byProp))
	}
}

// Two texts on sibling elements of the same type must stay distinguishable, or a
// consumer grouping by (document, property) folds them into one — which is the
// QUAL005 defect the ElementID exists to let a caller avoid.
func TestSitesIn_SiblingElementsOfOneTypeKeepSeparateIDs(t *testing.T) {
	lowID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	highID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	doc := bson.D{
		{Key: "$Type", Value: "Enumerations$Enumeration"},
		{Key: "Values", Value: bson.A{
			bson.D{
				{Key: "$ID", Value: mpr.IDToBsonBinary(lowID)},
				{Key: "$Type", Value: "Enumerations$EnumerationValue"},
				{Key: "Caption", Value: text(tr("en_US", "Low"), tr("nl_NL", "Laag"))},
			},
			bson.D{
				{Key: "$ID", Value: mpr.IDToBsonBinary(highID)},
				{Key: "$Type", Value: "Enumerations$EnumerationValue"},
				{Key: "Caption", Value: text(tr("en_US", "High"))}, // untranslated
			},
		}},
	}

	got := SitesIn(doc)
	if len(got) != 2 {
		t.Fatalf("want 2 sites, got %d", len(got))
	}
	if got[0].ElementID == got[1].ElementID {
		t.Fatalf("sibling values share ElementID %q — a consumer cannot tell them apart", got[0].ElementID)
	}
	if got[0].ElementID != lowID || got[1].ElementID != highID {
		t.Errorf("ElementIDs = %q, %q; want %q, %q", got[0].ElementID, got[1].ElementID, lowID, highID)
	}
}

// A text directly on the unit root has no enclosing element but is still a real
// site — dropping it would silently lose the document's own title.
func TestSitesIn_TextOnTheUnitRootIsStillASite(t *testing.T) {
	doc := bson.D{
		{Key: "$Type", Value: "Forms$Page"},
		{Key: "Title", Value: text(tr("en_US", "Home"))},
	}
	got := SitesIn(doc)
	if len(got) != 1 {
		t.Fatalf("want 1 site, got %d", len(got))
	}
	if got[0].OwnerType != "Forms$Page" || got[0].Property != "Title" {
		t.Errorf("site = %s.%s, want Forms$Page.Title", got[0].OwnerType, got[0].Property)
	}
}

func siteKeys(m map[string]Site) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
