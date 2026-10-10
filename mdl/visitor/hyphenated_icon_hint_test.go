// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"
)

// TestHyphenatedIconNameHint covers the ChipCoV6 finding: an Atlas icon whose
// name has a hyphen must quote that last segment, `Atlas_Core.Atlas."add-circle"`.
// Unquoted, the parse error pointed at a dot; quoting the whole name read as a
// stray string. Neither said what to write, and it cost two retries.
func TestHyphenatedIconNameHint(t *testing.T) {
	const want = "Atlas_Core.Atlas.\"add-circle\""
	for _, src := range []string{
		"mdl 1;\ncreate or modify navigation Responsive\n  home page M.Home\n{\n" +
			"  menu item 'Report' ( OnClick: show page M.P, Icon: Atlas_Core.Atlas.add-circle )\n};",
		"mdl 1;\ncreate or modify navigation Responsive\n  home page M.Home\n{\n" +
			"  menu item 'Report' ( OnClick: show page M.P, Icon: 'Atlas_Core.Atlas.add-circle' )\n};",
		"mdl 1;\ncreate page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" +
			"  actionbutton b (Caption: 'Add', Icon: Atlas_Core.Atlas.add-circle)\n};",
	} {
		_, errs := Build(src)
		if len(errs) == 0 {
			t.Fatalf("expected a syntax error for:\n%s", src)
		}
		joined := errsText(errs)
		if !strings.Contains(joined, want) {
			t.Errorf("no quoted-segment hint naming %s in:\n%s", want, joined)
		}
		if n := strings.Count(joined, "quote the icon name"); n > 1 {
			t.Errorf("hint repeated %d times for one mistake:\n%s", n, joined)
		}
	}
}

// The control: the quoted segment parses, and an unrelated error on a line
// with an icon gets no icon hint.
func TestHyphenatedIconNameHintControls(t *testing.T) {
	ok := "mdl 1;\ncreate page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  actionbutton b (Caption: 'Add', Icon: Atlas_Core.Atlas.\"add-circle\")\n};"
	if _, errs := Build(ok); len(errs) != 0 {
		t.Errorf("quoted segment refused: %v", errs)
	}
	bad := "mdl 1;\ncreate page M.P (Title: 'P', Layout: Atlas_Core.Atlas_Default) {\n" +
		"  actionbutton b (Caption: 'Add' Icon: Atlas_Core.Atlas.home)\n};"
	_, errs := Build(bad)
	if strings.Contains(errsText(errs), "quote the icon name") {
		t.Errorf("icon hint on an error that is not about the icon name:\n%s", errsText(errs))
	}
}
