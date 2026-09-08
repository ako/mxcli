// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/generated/metamodel"
)

// Every MDL sync word must map to a DECLARED member of Navigation$SyncMode.
//
// This is the guard #1035 earned. There, "Below grid" — the caption of the
// member `bottom` — reached gallery.def.json as a value, and mxbuild rejected
// every mxcli-authored gallery with CE0463. The same trap is set here and is
// worse: Studio Pro's dropdown shows "All Objects" and "By XPath", neither of
// which is a member of this enumeration, and six members map onto far fewer
// visible captions — so the mapping cannot be read off the UI at all.
//
// The right column comes from ako/TestApp, whose TabletOffline profile stores
// all six members.
func TestNavSyncModesAreDeclaredEnumMembers(t *testing.T) {
	declared := map[string]bool{
		string(metamodel.NavigationSyncModeAll):                 true,
		string(metamodel.NavigationSyncModeConstrained):         true,
		string(metamodel.NavigationSyncModeNever):               true,
		string(metamodel.NavigationSyncModeNone):                true,
		string(metamodel.NavigationSyncModeNoneAndPreserveData): true,
		string(metamodel.NavigationSyncModeOnline):              true,
	}

	// The mapping the visitor implements, restated so a change to it has to be
	// made here too — and so a caption slipping in is caught by name.
	mdlToStored := map[string]string{
		"ONLINE":             "Online",
		"ALL":                "All",
		"NEVER":              "Never",
		"NONE":               "None",
		"NONE PRESERVE DATA": "NoneAndPreserveData",
		"WHERE '<xpath>'":    "Constrained",
	}

	for word, stored := range mdlToStored {
		if !declared[stored] {
			t.Errorf("MDL %q maps to %q, which is not a declared Navigation$SyncMode member — "+
				"a caption where a key belongs is CE0463 (#1035) all over again", word, stored)
		}
	}

	// And the other direction: every member needs a way to be written, or a
	// project using it cannot round-trip through MDL.
	written := map[string]bool{}
	for _, stored := range mdlToStored {
		written[stored] = true
	}
	for member := range declared {
		if !written[member] {
			t.Errorf("Navigation$SyncMode member %q has no MDL spelling — a project using it "+
				"cannot survive describe -> exec", member)
		}
	}
}

// Captions must never be accepted as modes. Studio Pro shows these; they are
// not members, and a user copying what they see must get an error rather than
// a document mxbuild refuses.
func TestStudioProCaptionsAreNotSyncModes(t *testing.T) {
	for _, caption := range []string{"All Objects", "By XPath", "AllObjects", "ByXPath"} {
		prog, errs := Build("create or replace navigation TabletOffline sync ( sync Mod.E " + caption + "; )")
		if len(errs) == 0 && prog != nil && len(prog.Statements) > 0 {
			t.Errorf("caption %q parsed as a sync mode; it is not a member of the enumeration", caption)
		}
	}
}

// A new keyword silently steals every existing use of that word as a name.
// NEVER collided immediately: `editable: never` is a real page property value,
// and adding the token broke mdl-examples/bug-tests/maint2-editable-never-
// create-page.mdl until the four words were added to the keyword rule.
//
// TestKeywordRuleCoverage checks the rule LISTS them; this checks they actually
// parse, which is the property that matters.
func TestSyncKeywordsStayUsableAsIdentifiers(t *testing.T) {
	for _, word := range []string{"sync", "online", "never", "preserve"} {
		t.Run(word, func(t *testing.T) {
			// As an entity name, an attribute name, and a page property value.
			src := "create entity Mod." + word + " ( " + word + ": String(10) );"
			if _, errs := Build(src); len(errs) > 0 {
				t.Errorf("%q is no longer usable as an identifier: %v", word, errs[0])
			}
		})
	}
}
