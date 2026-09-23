// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#541 (action half) — a describe → exec round trip of a Studio Pro page
// reports "Replaced page", not "Unchanged page", and one reason is that four
// client actions are written with the wrong constants:
//
//	FooterWidgets/[1]/Action/DisabledDuringExecution  True → (key absent)
//	FooterWidgets/[2]/Action/DisabledDuringExecution  True → (key absent)
//	FooterWidgets/[1]/Action/SyncAutomatically       False → True
//
// Measured across all 67 pages of ako/TestApp at 11.14.0. For the four types
// changed here the stored value is unanimous — 39 of 39:
//
//	CancelChangesClientAction  True ×16      ClosePageClientAction  True ×10
//	SaveChangesClientAction    True × 8      DeleteClientAction     True × 5
//	SaveChangesClientAction.SyncAutomatically = False   8 of 8
//
// so neither is one user's unticked box. Seven of mxcli's action cases already
// wrote DisabledDuringExecution; the four simple ones did not, which is the
// "every constant the rebuild writes is a candidate" audit from
// docs-wiki/bug-patterns/rewrite-drops-unauthored-state.md.
//
// Unanimity does NOT hold across every type carrying the property, and the
// first sweep here missed that by filtering on names ending in "ClientAction":
// Forms$NoAction stores False on 83 of 7,300 and Forms$MicroflowAction on 5 of
// 81. Those minorities are a CARRY problem (a stored value mxcli overwrites),
// not a default problem, they predate this change, and they are out of its
// scope — see the follow-up noted on ako/mxcli#541. The lesson is the filter:
// a population selected by name can confirm whatever it excluded.
package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// disabledDuringExecution reads the flag off whichever action type was built.
// A type missing from this switch is a new action that has not been measured
// against a Studio Pro reference, so it fails rather than silently passing.
func disabledDuringExecution(t *testing.T, el any) bool {
	t.Helper()
	switch g := el.(type) {
	case *genPg.SaveChangesClientAction:
		return g.DisabledDuringExecution()
	case *genPg.CancelChangesClientAction:
		return g.DisabledDuringExecution()
	case *genPg.ClosePageClientAction:
		return g.DisabledDuringExecution()
	case *genPg.DeleteClientAction:
		return g.DisabledDuringExecution()
	case *genPg.PageClientAction:
		return g.DisabledDuringExecution()
	case *genPg.MicroflowClientAction:
		return g.DisabledDuringExecution()
	default:
		t.Fatalf("no DisabledDuringExecution accessor wired for %T — measure it "+
			"against a Studio Pro reference and add it here", el)
		return false
	}
}

// Studio Pro writes DisabledDuringExecution on every client action it stores.
// The four simple ones did not get it; the two at the end are the positive
// control, since they always did — without them a fix that broke the shared
// path would still pass.
func TestClientActionToGen_DisabledDuringExecution(t *testing.T) {
	cases := []struct {
		name   string
		action pages.ClientAction
	}{
		{"save_changes", &pages.SaveChangesClientAction{BaseElement: model.BaseElement{ID: "a"}}},
		{"cancel_changes", &pages.CancelChangesClientAction{BaseElement: model.BaseElement{ID: "b"}}},
		{"close_page", &pages.ClosePageClientAction{BaseElement: model.BaseElement{ID: "c"}}},
		{"delete_object", &pages.DeleteClientAction{BaseElement: model.BaseElement{ID: "d"}}},
		// Controls: these already carried it.
		{"show_page (control)", &pages.PageClientAction{BaseElement: model.BaseElement{ID: "e"}, PageName: "M.P"}},
		{"microflow (control)", &pages.MicroflowClientAction{BaseElement: model.BaseElement{ID: "f"}, MicroflowName: "M.MF"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			el, err := clientActionToGen(tc.action)
			if err != nil {
				t.Fatalf("clientActionToGen: %v", err)
			}
			if !disabledDuringExecution(t, el) {
				t.Errorf("DisabledDuringExecution = false; Studio Pro writes true on " +
					"39 of 39 stored actions of these four types across ako/TestApp")
			}
		})
	}
}

// SyncAutomatically is the opposite error: mxcli wrote true where all eight
// Studio Pro SaveChanges actions store false.
func TestClientActionToGen_SaveChangesSyncAutomatically(t *testing.T) {
	el, err := clientActionToGen(&pages.SaveChangesClientAction{
		BaseElement: model.BaseElement{ID: "a"},
	})
	if err != nil {
		t.Fatalf("clientActionToGen: %v", err)
	}
	g := el.(*genPg.SaveChangesClientAction)
	if g.SyncAutomatically() {
		t.Error("SyncAutomatically = true; Studio Pro stores false on 8 of 8")
	}
	// Control: the property the statement DOES author is still honoured, so
	// this is not a blanket "write false to everything".
	el2, _ := clientActionToGen(&pages.SaveChangesClientAction{
		BaseElement: model.BaseElement{ID: "b"}, ClosePage: true,
	})
	if !el2.(*genPg.SaveChangesClientAction).ClosePage() {
		t.Error("ClosePage was lost — the authored property must survive")
	}
}
