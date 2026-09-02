// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// navNotFoundEntry returns the value of a key and whether it was present,
// separating an absent key from an explicitly null one.
func navNotFoundEntry(d bson.D, key string) (interface{}, bool) {
	for _, e := range d {
		if e.Key == key {
			return e.Value, true
		}
	}
	return nil, false
}

// notFoundHomepageOf patches a bare web profile with the given spec and returns
// the NotFoundHomepage it wrote.
func notFoundHomepageOf(t *testing.T, spec NavigationProfileSpec) (interface{}, bool) {
	t.Helper()
	return navNotFoundEntry(patchWebProfile(bson.D{}, spec), "NotFoundHomepage")
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
// This is the writer the default engine uses, and the one that produced the
// unopenable project in #1000's report. It had no test: reverting just this
// $Type left the whole suite green.
func TestPatchWebProfile_NotFoundPageUsesItsOwnType(t *testing.T) {
	nfp, present := notFoundHomepageOf(t, NavigationProfileSpec{
		NotFoundPage: "MyFirstModule.NotFound",
	})
	if !present {
		t.Fatal("NotFoundHomepage key missing")
	}
	d, ok := nfp.(bson.D)
	if !ok {
		t.Fatalf("NotFoundHomepage = %#v, want a document", nfp)
	}
	if typ, _ := navNotFoundEntry(d, "$Type"); typ != "Navigation$NotFoundHomePage" {
		t.Errorf("$Type = %v, want Navigation$NotFoundHomePage (NOT Navigation$HomePage)", typ)
	}
	if page, _ := navNotFoundEntry(d, "Page"); page != "MyFirstModule.NotFound" {
		t.Errorf("Page = %v", page)
	}
	if _, present := navNotFoundEntry(d, "$ID"); !present {
		t.Error("every stored element needs its own $ID")
	}
}

// The two slots are adjacent, take the same Page/Microflow pair, and differ only
// in $Type — so a fix applied one `sed` too wide silently converts the home page
// as well. HomePage keeps Navigation$HomePage.
func TestPatchWebProfile_HomePageKeepsTheHomePageType(t *testing.T) {
	doc := patchWebProfile(bson.D{}, NavigationProfileSpec{
		HomePages:    []NavHomePageSpec{{IsPage: true, Target: "MyFirstModule.Home"}},
		NotFoundPage: "MyFirstModule.NotFound",
	})
	hp, present := navNotFoundEntry(doc, "HomePage")
	if !present {
		t.Fatal("HomePage key missing")
	}
	d, ok := hp.(bson.D)
	if !ok {
		t.Fatalf("HomePage = %#v, want a document", hp)
	}
	if typ, _ := navNotFoundEntry(d, "$Type"); typ != "Navigation$HomePage" {
		t.Errorf("HomePage $Type = %v, want Navigation$HomePage", typ)
	}
}

// No fallback page is an explicit null, not an element with a blank Page: a
// NotFoundHomePage pointing at "" is a dangling reference where absent is the
// modelled default.
func TestPatchWebProfile_NoNotFoundPageStaysNull(t *testing.T) {
	nfp, present := notFoundHomepageOf(t, NavigationProfileSpec{})
	if !present {
		t.Fatal("the NotFoundHomepage key must be written even when unset")
	}
	if nfp != nil {
		t.Errorf("NotFoundHomepage = %#v, want nil", nfp)
	}
}
