// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
)

// `mxcli lint` QUAL002 reported "Page 'X' has no documentation" against a page
// carrying a javadoc comment, the catalog's Description column was blank for
// every page and snippet, and `describe page` emitted no documentation — so the
// comment looked, from every angle, like it had been dropped (ako/CapTrackV4
// R12).
//
// It had not. The comment reaches the AST, the executor sets it, the writer
// stores it: `mxcli bson dump --type page` shows Documentation with the right
// value, and the LEGACY reader parses it back correctly. Only this engine's
// readers — the default — failed to carry it, and page and snippet were the two
// that did, out of five sibling readers in this file (layout, building block and
// page template all had it).
//
// That is why the report read as "javadoc does not work for pages": every
// symptom is downstream of the read.
func TestPageFromGen_CarriesDocumentation(t *testing.T) {
	p := genPg.NewPage()
	p.SetName("PG_Doc")
	p.SetDocumentation("A documented page.")

	out := pageFromGen(p, "container-1")
	if out.Documentation != "A documented page." {
		t.Errorf("Documentation = %q, want %q — QUAL002 then reports a documented "+
			"page as undocumented, and DESCRIBE cannot round-trip it",
			out.Documentation, "A documented page.")
	}
}

// CONTROL: an undocumented page stays undocumented, so the fix cannot be "always
// report something".
func TestPageFromGen_EmptyDocumentationStaysEmpty(t *testing.T) {
	p := genPg.NewPage()
	p.SetName("PG_Plain")

	if out := pageFromGen(p, "container-1"); out.Documentation != "" {
		t.Errorf("Documentation = %q, want empty", out.Documentation)
	}
}
