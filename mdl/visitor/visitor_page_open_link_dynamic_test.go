// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A link button can read its address from an attribute at runtime — Studio
// Pro's "Address: attribute" choice, stored as a Forms$StaticOrDynamicString
// with IsDynamic true. MDL could only spell the static form, so DESCRIBE had
// nothing to emit and a describe → exec cycle could not keep the action.
// Measured: FeedbackModule.PopupSuccess (Feedback v4.0.2) binds
// FeedbackModule.ResponseHelper.URL this way.
func TestAction_OpenLinkDynamicAddress(t *testing.T) {
	src := "create page M.P (Title: 'x', Layout: A.L) { actionbutton b (Action: open_link $currentObject/URL) };"
	raw := actionSlotValue(t, src, "Action")
	action, ok := raw.(*ast.ActionV3)
	if !ok {
		t.Fatalf("Action = %T (%v), want *ast.ActionV3", raw, raw)
	}
	if action.Type != "openLink" {
		t.Errorf("Type = %q, want openLink", action.Type)
	}
	if action.LinkAttribute != "URL" || action.LinkVariable != "$currentObject" {
		t.Errorf("LinkVariable/LinkAttribute = %q/%q, want $currentObject/URL", action.LinkVariable, action.LinkAttribute)
	}
	if action.LinkURL != "" {
		t.Errorf("LinkURL = %q, want empty for a dynamic address", action.LinkURL)
	}
}

// The static form is unchanged.
func TestAction_OpenLinkStaticAddressUnchanged(t *testing.T) {
	src := "create page M.P (Title: 'x', Layout: A.L) { actionbutton b (Action: open_link 'https://x.io') };"
	action, ok := actionSlotValue(t, src, "Action").(*ast.ActionV3)
	if !ok || action.Type != "openLink" || action.LinkURL != "https://x.io" || action.LinkAttribute != "" {
		t.Errorf("static open_link = %+v", action)
	}
}
