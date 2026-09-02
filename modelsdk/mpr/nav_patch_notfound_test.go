// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// navpNotFoundHomepageOf patches a bare web profile with the given spec and
// returns the NotFoundHomepage it wrote.
func navpNotFoundHomepageOf(t *testing.T, spec types.NavigationProfileSpec) (interface{}, bool) {
	t.Helper()
	return navpTestEntry(navpPatchWebProfile(bson.D{}, spec), "NotFoundHomepage")
}

// Studio Pro's "Fallback page" is its own type. The NotFoundHomepage property is
// declared Navigation$NotFoundHomePage, NOT the Navigation$HomePage the home-page
// slot takes, and .NET refuses the assignment on load:
//
//	System.ArgumentException: Object of type
//	'Mendix.Modeler.WebUI.Navigation.HomePage' cannot be converted to type
//	'Mendix.Modeler.WebUI.Navigation.NotFoundHomePage'.
//
// The project is then unopenable — and because the failure happens while LOADING
// the model, `mx check` prints that trace INSTEAD OF its "The app contains: N
// errors" line, so a caller reading the count rather than the exit status sees a
// run that reported nothing at all (mendixlabs/mxcli#1000).
//
// This writer had no test of its own: reverting just this $Type left the whole
// suite green.
func TestNavpPatchWebProfile_NotFoundPageUsesItsOwnType(t *testing.T) {
	nfp, present := navpNotFoundHomepageOf(t, types.NavigationProfileSpec{
		NotFoundPage: "MyFirstModule.NotFound",
	})
	if !present {
		t.Fatal("NotFoundHomepage key missing")
	}
	d, ok := nfp.(bson.D)
	if !ok {
		t.Fatalf("NotFoundHomepage = %#v, want a document", nfp)
	}
	if typ, _ := navpTestEntry(d, "$Type"); typ != "Navigation$NotFoundHomePage" {
		t.Errorf("$Type = %v, want Navigation$NotFoundHomePage (NOT Navigation$HomePage)", typ)
	}
	if page, _ := navpTestEntry(d, "Page"); page != "MyFirstModule.NotFound" {
		t.Errorf("Page = %v", page)
	}
	if _, present := navpTestEntry(d, "$ID"); !present {
		t.Error("every stored element needs its own $ID")
	}
}

// The two slots are adjacent, take the same Page/Microflow pair, and differ only
// in $Type — so a fix applied one `sed` too wide silently converts the home page
// as well. HomePage keeps Navigation$HomePage.
func TestNavpPatchWebProfile_HomePageKeepsTheHomePageType(t *testing.T) {
	doc := navpPatchWebProfile(bson.D{}, types.NavigationProfileSpec{
		HomePages:    []types.NavHomePageSpec{{IsPage: true, Target: "MyFirstModule.Home"}},
		NotFoundPage: "MyFirstModule.NotFound",
	})
	hp, present := navpTestEntry(doc, "HomePage")
	if !present {
		t.Fatal("HomePage key missing")
	}
	d, ok := hp.(bson.D)
	if !ok {
		t.Fatalf("HomePage = %#v, want a document", hp)
	}
	if typ, _ := navpTestEntry(d, "$Type"); typ != "Navigation$HomePage" {
		t.Errorf("HomePage $Type = %v, want Navigation$HomePage", typ)
	}
}

// No fallback page is an explicit null, not an element with a blank Page: a
// NotFoundHomePage pointing at "" is a dangling reference where absent is the
// modelled default.
func TestNavpPatchWebProfile_NoNotFoundPageStaysNull(t *testing.T) {
	nfp, present := navpNotFoundHomepageOf(t, types.NavigationProfileSpec{})
	if !present {
		t.Fatal("the NotFoundHomepage key must be written even when unset")
	}
	if nfp != nil {
		t.Errorf("NotFoundHomepage = %#v, want nil", nfp)
	}
}
