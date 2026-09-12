// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `statictext` writes Forms$Text, and Mendix 11 has no such type. The project
// that comes out cannot be LOADED — `mx check` and Studio Pro both stop at
// TypeCacheUnknownTypeException before any validation runs — so the page is
// unopenable and cannot be repaired in the modeler.
//
// Both engines emit it, so this is not something MXCLI_ENGINE works around; the
// refusal is in the builder and the check, not in a writer.
func TestValidateRetiredWidgetKind_StaticTextIsRefused(t *testing.T) {
	// Deliberately no project: the type is unknown to every Mendix version, so
	// unlike MDL-WIDGET25 this must fire without one. Passing "" is the
	// assertion, not a convenience.
	got := validateRetiredWidgetKind(&ast.WidgetV3{Type: "statictext", Name: "t1"}, "page X")
	if len(got) == 0 || got[0].RuleID != "MDL-WIDGET27" {
		t.Fatalf("statictext was accepted; got %v", got)
	}
	if !strings.Contains(got[0].Message, "Forms$Text") {
		t.Errorf("the message does not name the type that is missing: %s", got[0].Message)
	}
	if !strings.Contains(got[0].Suggestion, "dynamictext") {
		t.Errorf("the suggestion does not name the replacement: %s", got[0].Suggestion)
	}

	// And it reaches the tree walk, or `mxcli check` never runs it.
	if v := widgetKindViolations(t, "", []*ast.WidgetV3{{Type: "statictext", Name: "t1"}}); !containsRule(v, "MDL-WIDGET27") {
		t.Errorf("not reported by the widget-tree walk: %v", v)
	}
}

// The control. Without it this test would pass just as well against a rule that
// refused every widget — and `dynamictext` is the exact widget the refusal above
// tells the author to use, so it is the one spelling that must stay clean.
func TestValidateRetiredWidgetKind_DynamicTextIsSilent(t *testing.T) {
	got := widgetKindViolations(t, "", []*ast.WidgetV3{
		{Type: "dynamictext", Name: "t1", Properties: map[string]any{"Content": "hello"}},
	})
	if containsRule(got, "MDL-WIDGET27") {
		t.Errorf("dynamictext was refused: %v", got)
	}
}

// The builder is the backstop for `text`, which shares buildTextWidgetV3 but
// reaches the validator as a generic type — so MDL-WIDGET25 only catches it when
// a project is available to resolve against. Neither spelling may reach a write.
func TestBuildTextWidget_RefusesBothSpellings(t *testing.T) {
	for _, kind := range []string{"text", "statictext"} {
		t.Run(kind, func(t *testing.T) {
			pb := &pageBuilder{}
			if _, err := pb.buildTextWidgetV3(&ast.WidgetV3{Type: kind, Name: "t1"}); err == nil {
				t.Fatalf("%s built a Forms$Text widget", kind)
			} else if !containsText([]string{err.Error()}, "Forms$Text") {
				t.Errorf("the error does not name the type: %v", err)
			}
		})
	}
}
