// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func widget27(t *testing.T, src string) []linter.Violation {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	registry := LoadWidgetRegistry(fixtureProject(t))
	if registry == nil {
		t.Fatal("no registry")
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		for _, v := range ValidateWidgetPropertiesForStatement(stmt, registry) {
			if v.RuleID == "MDL-WIDGET27" {
				out = append(out, v)
			}
		}
	}
	return out
}

const page27 = `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  htmlelement frame ( tagName: 'div', %s )
}`

// mendixlabs/mxcli#999. A repeatable widget property written as a property
// VALUE had two failure modes, and the dangerous one looked like success:
//
//	[(configMode: simple)]              parsed as a list of expressions,
//	                                    checked CLEAN, exec'd, and the property
//	                                    vanished from storage
//	[(configMode: simple, x: y)]        died as `missing ')' at ','`
//
// The reporter's own framing: "check/exec-pass-then-discard". On FileUploader
// the dropped property was `allowedFileFormats`, which restricts uploads — so a
// silent drop is a correctness AND a mild security problem in the built app.
func TestObjectEntryProperty_IsReported(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s", `attributes: [(attributeName: 'data-x')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1: %+v", len(got), got)
	}
	if got[0].Severity != linter.SeverityError {
		t.Errorf("severity = %v, want error — a warning would still let exec write the "+
			"page and discard the entries, which is the bug", got[0].Severity)
	}
	// The message has to name the container keyword, or it says "wrong" without
	// saying what right looks like.
	if !strings.Contains(got[0].Message, "attribute") {
		t.Errorf("message does not name the container keyword: %q", got[0].Message)
	}
	if !strings.Contains(got[0].Message, "attributes") {
		t.Errorf("message does not name the property: %q", got[0].Message)
	}
}

// The multi-key shape is the one that would not parse at all. It now reaches the
// same diagnostic instead of `missing ')' at ','`, which named a paren and left
// the author to guess.
func TestObjectEntryProperty_MultiKeyReachesTheSameError(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s",
		`attributes: [(attributeName: 'data-x', attributeValueType: 'expression')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1: %+v", len(got), got)
	}
}

// Several entries, which is what a real allowedFileFormats list looks like.
func TestObjectEntryProperty_SeveralEntries(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s",
		`attributes: [(attributeName: 'a'), (attributeName: 'b')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1 per property: %+v", len(got), got)
	}
}

// THE CONTROL. The container form is the spelling that works, and it must stay
// silent — otherwise the rule is "report every object list" and the assertions
// above prove nothing about the property form specifically.
func TestObjectEntryProperty_ContainerFormIsSilent(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  htmlelement frame ( tagName: 'div' ) {
    attribute a1 (attributeName: 'data-x', attributeValueType: 'expression')
  }
}`
	if got := widget27(t, src); len(got) != 0 {
		t.Errorf("the working container form was reported: %+v", got)
	}
}

// The second control: an ordinary array property must keep working. `[…]` is a
// real value shape (DesignProperties, ContentParams), and a rule that fired on
// every bracket would break scripts that are correct today.
func TestObjectEntryProperty_OrdinaryArrayIsSilent(t *testing.T) {
	src := `create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: '{1}', ContentParams: [{1} = Name])
}`
	if got := widget27(t, src); len(got) != 0 {
		t.Errorf("an ordinary array property was reported: %+v", got)
	}
}

// A property the widget does not declare as an object list still gets the error:
// the SHAPE is wrong regardless, and staying quiet would put us back to writing
// a page that silently discards it.
func TestObjectEntryProperty_ReportedEvenWhenTheKeyIsUnknown(t *testing.T) {
	got := widget27(t, strings.Replace(page27, "%s", `notARealProperty: [(a: 'b')]`, 1))
	if len(got) != 1 {
		t.Fatalf("got %d MDL-WIDGET27, want 1: %+v", len(got), got)
	}
}

// And with no project at all — `make check-mdl` runs that way, so a rule that
// needed a definition would be inert in CI.
func TestObjectEntryProperty_FiresWithoutAProject(t *testing.T) {
	src := strings.Replace(page27, "%s", `attributes: [(attributeName: 'data-x')]`, 1)
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	registry := LoadWidgetRegistry("")
	var n int
	for _, stmt := range prog.Statements {
		for _, v := range ValidateWidgetPropertiesForStatement(stmt, registry) {
			if v.RuleID == "MDL-WIDGET27" {
				n++
			}
		}
	}
	if n != 1 {
		t.Errorf("got %d MDL-WIDGET27 with no project, want 1", n)
	}
}

// The AST shape, asserted directly — the #1036 lesson is that a corpus diff of
// check output is blind to a construct that parses into the wrong shape, and
// this construct's whole problem was that it parsed into a []string nobody
// claimed.
func TestObjectEntryProperty_ParsesToItsOwnType(t *testing.T) {
	prog, errs := visitor.Build(strings.Replace(page27, "%s",
		`attributes: [(attributeName: 'data-x', attributeValueType: 'expression')]`, 1))
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	oel, ok := w.Properties["attributes"].(*ast.ObjectEntryListV3)
	if !ok {
		t.Fatalf("attributes is %T, want *ast.ObjectEntryListV3", w.Properties["attributes"])
	}
	if len(oel.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(oel.Entries))
	}
	if oel.Entries[0]["attributeName"] != "data-x" {
		t.Errorf("entry = %+v, want attributeName data-x", oel.Entries[0])
	}
}

// The shape control for the grammar change. The new alternative is ordered
// BEFORE the expression array, so the risk is that it swallows an ordinary
// `[…]` value and changes its AST type — which a corpus diff of check output
// cannot see, because a value read as the wrong type still produces no
// diagnostic (the slice 2-3 lesson from mendixlabs/mxcli#1036).
//
// Measured alongside this: 0 of 532 example scripts changed their check output.
// That is necessary and not sufficient; this is the sufficient half.
func TestOrdinaryArrayKeepsItsAstShape(t *testing.T) {
	prog, errs := visitor.Build(`create page M.P (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: '{1}', ContentParams: [{1} = Name])
}`)
	if len(errs) > 0 {
		t.Fatalf("parsing: %v", errs)
	}
	w := prog.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	got := w.Properties["ContentParams"]
	if _, wrong := got.(*ast.ObjectEntryListV3); wrong {
		t.Fatalf("an ordinary array was captured by the object-entry alternative: %#v", got)
	}
	if _, ok := got.([]ast.ParamAssignmentV3); !ok {
		t.Errorf("ContentParams is %T, want []ast.ParamAssignmentV3 — the ordinary array "+
			"shape is unchanged", got)
	}

	// A plain literal array too, which is the shape closest to `[(…)]` and so the
	// likeliest to be captured by mistake.
	prog2, errs2 := visitor.Build(`create page M.P2 (Title: 'x', Layout: Atlas_Core.Atlas_Default) {
  dynamictext t (Content: 'x', DesignProperties: ['Spacing top': 'Large'])
}`)
	if len(errs2) > 0 {
		t.Fatalf("parsing the design-property array: %v", errs2)
	}
	w2 := prog2.Statements[0].(*ast.CreatePageStmtV3).Widgets[0]
	if _, wrong := w2.Properties["DesignProperties"].(*ast.ObjectEntryListV3); wrong {
		t.Errorf("a design-property array was captured by the object-entry alternative: %#v",
			w2.Properties["DesignProperties"])
	}
}
