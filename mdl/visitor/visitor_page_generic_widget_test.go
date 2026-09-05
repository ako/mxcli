// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func buildOnePage(t *testing.T, src string) *ast.CreatePageStmtV3 {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v\nsource:\n%s", errs, src)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	pg, ok := prog.Statements[0].(*ast.CreatePageStmtV3)
	if !ok {
		t.Fatalf("got %T, want *ast.CreatePageStmtV3", prog.Statements[0])
	}
	return pg
}

// Slice 2: a widget named by its own MDL name reaches the AST as that type, and
// is marked generic so the validator knows it must resolve to a definition.
func TestGenericWidgetTypeReachesTheAST(t *testing.T) {
	pg := buildOnePage(t, `create page P.Q (Title: 'x', Layout: A.B) {
  htmlelement frame (tagName: 'div')
}`)
	if len(pg.Widgets) != 1 {
		t.Fatalf("got %d widgets, want 1", len(pg.Widgets))
	}
	w := pg.Widgets[0]
	if w.Type != "htmlelement" {
		t.Errorf("Type = %q, want %q", w.Type, "htmlelement")
	}
	if w.Name != "frame" {
		t.Errorf("Name = %q, want \"frame\" — the name must still be the widget's own IDENTIFIER, "+
			"not swallowed by the generic type alternative", w.Name)
	}
	if !w.TypeIsGeneric {
		t.Error("TypeIsGeneric = false; a name that is not an enumerated widget token must be " +
			"marked generic, or MDL-WIDGET25 cannot tell a typo from a built-in")
	}
}

// The control: an enumerated widget keyword must NOT be marked generic, or every
// built-in would be required to resolve to a widget definition.
func TestEnumeratedWidgetTypeIsNotGeneric(t *testing.T) {
	pg := buildOnePage(t, `create page P.Q (Title: 'x', Layout: A.B) {
  container c1 (Class: 'x')
}`)
	w := pg.Widgets[0]
	if w.Type != "container" {
		t.Fatalf("Type = %q, want container", w.Type)
	}
	if w.TypeIsGeneric {
		t.Error("TypeIsGeneric = true for `container`, an enumerated widget token — " +
			"the flag must come from which grammar alternative matched, not from a name lookup")
	}
}

// Slice 3: a container whose keyword lexes as a KEYWORD token, not IDENTIFIER.
// This is the construct from mendixlabs/mxcli#1036 — `attribute` inside an HTML
// Element — which failed with "mismatched input" before.
func TestKeywordContainerParsesInsideAWidgetBody(t *testing.T) {
	pg := buildOnePage(t, `create page P.Q (Title: 'x', Layout: A.B) {
  htmlelement frame (tagName: 'div') {
    attribute a1 (attributeName: 'title')
    event e1 (eventName: 'onClick')
  }
}`)
	w := pg.Widgets[0]
	if len(w.Children) != 2 {
		t.Fatalf("got %d children, want 2 (attribute, event)", len(w.Children))
	}
	for i, want := range []string{"attribute", "event"} {
		if w.Children[i].Type != want {
			t.Errorf("child %d Type = %q, want %q", i, w.Children[i].Type, want)
		}
		if !w.Children[i].TypeIsGeneric {
			t.Errorf("child %d (%s) not marked generic", i, want)
		}
	}
}

// pageBodyV3's alternative ORDER is load-bearing since slices 2-3, and this is
// the trap that a diff of `mxcli check` output cannot detect.
//
// SLOT, PLACEHOLDER and USE are all inside the `keyword` rule, so widgetV3's
// generic alternative can match them. With widgetV3 ordered first, `slot body`
// parsed as a widget of type `slot` named `body`, and `placeholder Main { … }`
// as a widget named Main. Both still parsed and `check` still exited 0 — the
// damage is to the AST, not to the diagnostics, which is why 515 example
// scripts showed zero difference while two visitor tests failed.
//
// Anyone reordering pageBodyV3 for tidiness reintroduces it silently.
func TestSpecificPageBodyFormsWinOverTheGenericWidget(t *testing.T) {
	pg := buildOnePage(t, `create page P.Q (Title: 'x', Layout: A.B) {
  placeholder Main {
    dynamictext t (Content: 'hi')
  }
}`)
	if len(pg.Placeholders) != 1 {
		t.Fatalf("placeholder blocks = %d, want 1 — `placeholder` was swallowed by the generic "+
			"widget alternative; the specific alternatives must precede widgetV3 in pageBodyV3",
			len(pg.Placeholders))
	}
	if len(pg.Widgets) != 0 {
		t.Errorf("bare widgets = %d, want 0 — the placeholder block was parsed as a widget", len(pg.Widgets))
	}
}

func TestSlotMarkerWinsOverTheGenericWidget(t *testing.T) {
	prog, errs := Build(`DEFINE FRAGMENT Card AS {
		CONTAINER wrap (Class: 'c') {
			SLOT content
		}
	};`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	// `slot` is in the `keyword` rule, so the generic widget alternative can
	// match it. The fragment must still see a slot, not a widget named `body`.
	if got := fmt.Sprintf("%#v", prog.Statements[0]); strings.Contains(strings.ToLower(got), `type:"slot"`) {
		t.Errorf("`slot body` became a widget of type slot — slotMarkerV3 must precede widgetV3 in pageBodyV3:\n%s", got)
	}
}
