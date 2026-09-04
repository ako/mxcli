// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/model"
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
// quietly written. Without this the test above could pass because the default
// branch had been softened, which is the exact failure the legacy engine had.
func TestClientActionToGen_StillRefusesWhatItCannotWrite(t *testing.T) {
	_, err := clientActionToGen(&pages.LinkClientAction{
		BaseElement: model.BaseElement{ID: "link-id"},
		Address:     "https://example.com",
	})
	if err == nil {
		t.Fatal("OPEN_LINK was accepted; it has no writer, so accepting it means dropping it")
	}
	if !strings.Contains(err.Error(), "not yet supported") {
		t.Errorf("unexpected message: %v", err)
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
