// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Promoting NOTHING to a real actionExprV3 alternative changes what reaches the
// builder: the slot used to hold a string the builder never saw, and now holds
// an *ast.ActionV3 with Type "none". buildClientActionV3's switch ends in a
// `default:` that returns "unsupported action type", so without the matching
// case this change would turn a documented, working spelling into an exec
// failure — the grammar and the builder have to move together
// (mendixlabs/mxcli#1062).
//
// The resulting element is the same Forms$NoAction the fall-through produced, so
// nothing changes on disk. Measured end to end: a page written by the pre-fix
// binary and re-run by the post-fix binary reports `Unchanged page`, which is
// the repo's own canonical comparison (ADR-0008) saying the documents are
// semantically identical.
func TestBuildClientAction_NothingIsNoAction(t *testing.T) {
	pb := &pageBuilder{}

	got, err := pb.buildClientActionV3(&ast.ActionV3{Type: "none"})
	if err != nil {
		t.Fatalf("buildClientActionV3(none): %v — `Action: NOTHING` is shipped syntax", err)
	}
	no, ok := got.(*pages.NoClientAction)
	if !ok {
		t.Fatalf("action type = %T, want *pages.NoClientAction", got)
	}
	if no.TypeName != "Forms$NoAction" {
		t.Errorf("TypeName = %q, want Forms$NoAction", no.TypeName)
	}
	if no.ID == "" {
		t.Error("no element ID minted")
	}
}

// The control: an action type the builder does not know must still be refused,
// or the case above would have been better written as a silent default.
func TestBuildClientAction_UnknownTypeIsStillRefused(t *testing.T) {
	pb := &pageBuilder{}
	if _, err := pb.buildClientActionV3(&ast.ActionV3{Type: "totallyMadeUp"}); err == nil {
		t.Fatal("an unknown action type was accepted — the default branch is what keeps a " +
			"new grammar form from being written as nothing")
	}
}
