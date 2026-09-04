// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// CapTrackV2 FINDINGS §10 — `ACTIONBUTTON … (Action: SIGN_OUT)` was refused
// outright by the default engine:
//
//	client action *pages.SignOutClientAction not yet supported by the
//	modelsdk engine — rerun with MXCLI_ENGINE=legacy
//
// The refusal was honest; the advice was not. The legacy writer had no case for
// the action either and fell through to Forms$NoAction, so the recommended
// escape hatch produced a button that rendered, said "Sign out", and did
// nothing — with check, exec and mx check all clean.
//
// The document is one property, pinned against a Studio Pro-authored button
// (ako/TestApp, Mendix 11):
//
//	{ "$Type": "Forms$SignOutClientAction", "DisabledDuringExecution": true }
//
// That reference is provably Studio Pro's rather than mxcli's, because until
// this change NEITHER engine could emit the type.
func TestClientActionToGen_SignOut(t *testing.T) {
	el, err := clientActionToGen(&pages.SignOutClientAction{
		BaseElement: model.BaseElement{ID: "action-id"},
	})
	if err != nil {
		t.Fatalf("SIGN_OUT is still refused by the modelsdk engine: %v", err)
	}
	g, ok := el.(*genPg.SignOutClientAction)
	if !ok {
		t.Fatalf("got %T, want *pages.SignOutClientAction", el)
	}
	if g.TypeName() != "Forms$SignOutClientAction" {
		t.Errorf("$Type = %q, want Forms$SignOutClientAction", g.TypeName())
	}
	if !g.DisabledDuringExecution() {
		t.Error("DisabledDuringExecution is false; the Studio Pro reference stores true")
	}
}

// CONTROL 1: an action that is still unimplemented must still be REFUSED, not
// quietly written. Without this the tests above could pass because the default
// branch had been softened, which is the exact failure the legacy engine had.
//
// ShowHomePage is the stand-in: gen has no type for it, generated/metamodel has
// no Pages counterpart, and no MDL statement builds one — so it is a semantic
// type nothing can write, which is precisely what this control needs. (The
// earlier draft used LinkClientAction, which stopped being a valid control the
// moment OPEN_LINK was implemented.)
func TestClientActionToGen_StillRefusesWhatItCannotWrite(t *testing.T) {
	_, err := clientActionToGen(&pages.ShowHomePageClientAction{
		BaseElement: model.BaseElement{ID: "home-id"},
	})
	if err == nil {
		t.Fatal("an action with no writer was accepted, which means dropping it")
	}
	if !strings.Contains(err.Error(), "not yet supported") {
		t.Errorf("unexpected message: %v", err)
	}
}

// OPEN_LINK, the other action that used to fall through. gen calls it
// OpenLinkClientAction — the storage name differs from the semantic type's
// "Forms$LinkClientAction", a wrong $Type that never reached disk only because
// nothing could write the action at all.
//
// Pinned against 31 Studio Pro-authored link buttons (ako/TestApp,
// FeedbackModule): five keys, LinkType "Web" in all 31, address nested as a
// Forms$StaticOrDynamicString.
func TestClientActionToGen_OpenLink(t *testing.T) {
	el, err := clientActionToGen(&pages.LinkClientAction{
		BaseElement: model.BaseElement{ID: "link-id"},
		LinkType:    pages.LinkTypeWeb,
		Address:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("OPEN_LINK is still refused: %v", err)
	}
	g, ok := el.(*genPg.OpenLinkClientAction)
	if !ok {
		t.Fatalf("got %T, want *pages.OpenLinkClientAction", el)
	}
	if g.TypeName() != "Forms$OpenLinkClientAction" {
		t.Errorf("$Type = %q — Mendix stores OpenLink, not Link", g.TypeName())
	}
	if g.LinkType() != "Web" {
		t.Errorf("LinkType = %q, want Web", g.LinkType())
	}
	addr, ok := g.Address().(*genPg.StaticOrDynamicString)
	if !ok {
		t.Fatalf("Address is %T, want *pages.StaticOrDynamicString", g.Address())
	}
	if addr.IsDynamic() {
		t.Error("IsDynamic is true; MDL authors the static form only")
	}
	if addr.Value() != "https://example.com" {
		t.Errorf("Value = %q, want the authored URL", addr.Value())
	}
}

// An empty LinkType must not reach storage: Mendix's enum is Call/Email/Text/Web
// and every one of the 31 references is Web, so that is the default rather than
// writing a blank a build would reject.
func TestClientActionToGen_OpenLinkDefaultsLinkType(t *testing.T) {
	el, err := clientActionToGen(&pages.LinkClientAction{
		BaseElement: model.BaseElement{ID: "link-id"},
		Address:     "https://example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if lt := el.(*genPg.OpenLinkClientAction).LinkType(); lt != "Web" {
		t.Errorf("LinkType = %q, want Web", lt)
	}
}

// CONTROL 2: the actions that already worked are untouched.
func TestClientActionToGen_ExistingActionsUnchanged(t *testing.T) {
	for _, a := range []pages.ClientAction{
		&pages.SaveChangesClientAction{BaseElement: model.BaseElement{ID: "a"}},
		&pages.ClosePageClientAction{BaseElement: model.BaseElement{ID: "b"}},
		&pages.DeleteClientAction{BaseElement: model.BaseElement{ID: "c"}},
	} {
		if _, err := clientActionToGen(a); err != nil {
			t.Errorf("%T was refused: %v", a, err)
		}
	}
}
