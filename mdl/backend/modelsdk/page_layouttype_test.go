// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
)

// gen exposes Layout.LayoutType(), and it is a phantom: it binds a key a
// Forms$Layout does not carry, so it reads "" for every layout ever written.
// Reading it there made DESCRIBE LAYOUT report all 22 Atlas layouts as
// Responsive, because the describe output defaulted "" to that.
//
// The value lives on the content wrapper. This pins layoutTypeOf to the wrapper
// and covers both platforms.
func TestLayoutTypeOf_ReadsTheContentWrapper(t *testing.T) {
	web := genPg.NewWebLayoutContent()
	web.SetLayoutType("ModalPopup")
	l := genPg.NewLayout()
	l.SetContent(web)
	if got := layoutTypeOf(l); string(got) != "ModalPopup" {
		t.Errorf("web: got %q, want ModalPopup", got)
	}

	native := genPg.NewNativeLayoutContent()
	native.SetLayoutType("Popup")
	l2 := genPg.NewLayout()
	l2.SetContent(native)
	if got := layoutTypeOf(l2); string(got) != "Popup" {
		t.Errorf("native: got %q, want Popup", got)
	}

	// A layout with no content reads empty rather than inventing a default —
	// an empty value means the read failed and must be reported as such.
	if got := layoutTypeOf(genPg.NewLayout()); got != "" {
		t.Errorf("no content: got %q, want empty", got)
	}
}
