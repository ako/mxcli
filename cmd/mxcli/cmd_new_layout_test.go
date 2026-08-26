// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"text/template"

	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func renderScaffold(t *testing.T, data scaffoldLayoutData) string {
	t.Helper()
	var out strings.Builder
	tmpl := template.Must(template.New("layout").Parse(scaffoldLayoutMDL))
	if err := tmpl.Execute(&out, data); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// The scaffold is MDL rather than Go so a new project contains something the
// user could have written — which is only true if it parses.
func TestScaffoldLayoutMDL_Parses(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module:      "MyFirstModule",
		Layout:      scaffoldLayoutName,
		Profile:     "Responsive",
		RepointFrom: []string{"Atlas_Core.Atlas_TopBar"},
	})
	if _, errs := visitor.Build(script); len(errs) > 0 {
		t.Fatalf("the scaffold script does not parse: %v\n---\n%s", errs[0], script)
	}
}

// With no page to move, the script must be the layout alone — no repoint
// statement, so `mxcli new` does not print "Repointed 0 page(s)" for a source
// that matches nothing.
func TestScaffoldLayoutMDL_OmitsTheRepointWhenThereIsNothingToMove(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "MyFirstModule", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if strings.Contains(script, "alter pages") {
		t.Errorf("emitted a repoint with no source layouts:\n%s", script)
	}
	if _, errs := visitor.Build(script); len(errs) > 0 {
		t.Fatalf("the layout-only script does not parse: %v", errs[0])
	}
}

// The placeholder name is API: every page binds to it as Module.Layout.<Name>,
// and Atlas names it Main. A scaffold that named it anything else would refuse
// every repoint it then attempts.
func TestScaffoldLayoutMDL_DeclaresMain(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if !strings.Contains(script, "placeholder Main") {
		t.Errorf("the scaffold must declare a placeholder named Main:\n%s", script)
	}
}

// The region classes are what Atlas — and every mxcli theme — styles. A
// scaffold that used its own names would render unstyled, which looks like a
// broken app rather than a different layout.
//
// region-sidebar is deliberately absent: Atlas's sidebar is a drawer the
// Forms$SidebarToggleButton opens, and mxcli cannot author that button, so an
// always-open 232px rail would be a difference from the layout being replaced
// rather than a match for it.
func TestScaffoldLayoutMDL_KeepsTheAtlasRegionClasses(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if strings.Contains(script, "region-sidebar") {
		t.Errorf("the scaffold carries a sidebar it has no toggle button for:\n%s", script)
	}
	for _, class := range []string{"region-topbar", "region-content", "topbar-content"} {
		if !strings.Contains(script, "'"+class+"'") {
			t.Errorf("the scaffold does not carry the %q class Atlas styles:\n%s", class, script)
		}
	}
}

// The profile is substituted, not hardcoded: a template shipping a differently
// named web profile would otherwise get a menu that resolves to nothing.
func TestScaffoldLayoutMDL_UsesTheResolvedProfile(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "WebProfile",
	})
	if !strings.Contains(script, "profile: 'WebProfile'") {
		t.Errorf("the menu must draw from the resolved profile:\n%s", script)
	}
	if strings.Contains(script, "profile: 'Responsive'") {
		t.Errorf("a hardcoded profile survived substitution:\n%s", script)
	}
}

// The layout's own CSS class is what makes Atlas's chrome apply: ~24 of Atlas's
// layout rules are scoped to `.layout-atlas` and its variants. A scaffold
// without it builds cleanly and renders with no topbar bar — measured in a
// browser, which is the only place it shows.
func TestScaffoldLayoutMDL_CarriesTheAtlasLayoutClass(t *testing.T) {
	script := renderScaffold(t, scaffoldLayoutData{
		Module: "M", Layout: scaffoldLayoutName, Profile: "Responsive",
	})
	if !strings.Contains(script, "class: 'layout-atlas") {
		t.Errorf("the scaffold must carry the layout-atlas class:\n%s", script)
	}
}
