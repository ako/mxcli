// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
)

// A profile's login page and not-found page must survive a write -> read cycle.
// Neither did: the reader accepted only the $Types gen declares for those two
// slots, and the documents (Studio Pro's and mxcli's alike) carry different ones
// -- Forms$FormSettings for the login settings, Navigation$HomePage for the
// not-found page. Both read back empty, so DESCRIBE NAVIGATION dropped the
// clauses and a describe -> exec round-trip deleted them from the project.
//
// The control is the legacy engine, which prints both from the same document.
func TestGetNavigation_ProfilePagesRoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	nav, err := b.GetNavigation()
	if err != nil || nav == nil || len(nav.Profiles) == 0 {
		t.Skipf("no navigation profiles in fixture: %v", err)
	}
	prof := nav.Profiles[0].Name

	if err := b.UpdateNavigationProfile(nav.ID, prof, types.NavigationProfileSpec{
		HomePages:    []types.NavHomePageSpec{{IsPage: true, Target: "MyFirstModule.Home"}},
		LoginPage:    "MyFirstModule.SignIn",
		NotFoundPage: "MyFirstModule.NotFound",
	}); err != nil {
		t.Fatalf("UpdateNavigationProfile: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	nav2, err := b2.GetNavigation()
	if err != nil {
		t.Fatalf("GetNavigation(2): %v", err)
	}
	var p *types.NavigationProfile
	for _, x := range nav2.Profiles {
		if x.Name == prof {
			p = x
		}
	}
	if p == nil {
		t.Fatalf("profile %q gone after update", prof)
	}
	if p.LoginPage != "MyFirstModule.SignIn" {
		t.Errorf("LoginPage = %q, want MyFirstModule.SignIn (stored as Forms$FormSettings/Form)", p.LoginPage)
	}
	if p.NotFoundPage != "MyFirstModule.NotFound" {
		t.Errorf("NotFoundPage = %q, want MyFirstModule.NotFound (stored as Navigation$HomePage/Page)", p.NotFoundPage)
	}
}
