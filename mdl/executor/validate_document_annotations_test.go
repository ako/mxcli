// SPDX-License-Identifier: Apache-2.0

package executor

import (
	goast "go/ast"
	goparser "go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// visitorFilesWithDocumentAnnotations are the visitor sources that read an
// annotation off a createStatement. A new one has to be listed here, which the
// union check below then holds to the table.
var visitorFilesWithDocumentAnnotations = []string{
	"../visitor/visitor_association.go",
	"../visitor/visitor_entity.go",
	"../visitor/visitor_microflow.go",
	"../visitor/visitor_page_v3.go",
}

// TestDocumentAnnotationsMatchTheVisitor pins the table to the visitor's own
// string literals, in BOTH directions.
//
// A name the visitor reads but the table omits rejects a valid script. A name the
// table lists but no visitor reads accepts an annotation that does nothing — the
// very thing this rule exists to catch. Neither is visible without comparing the
// two, which is why they are compared rather than kept in step by hand.
func TestDocumentAnnotationsMatchTheVisitor(t *testing.T) {
	read := map[string]bool{}
	fset := token.NewFileSet()
	for _, path := range visitorFilesWithDocumentAnnotations {
		file, err := goparser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		goast.Inspect(file, func(n goast.Node) bool {
			fn, ok := n.(*goast.FuncDecl)
			if !ok || fn.Body == nil || !mentionsAllAnnotation(fn) {
				return true
			}
			for _, name := range equalFoldLiterals(fn.Body) {
				read[strings.ToLower(name)] = true
			}
			return false
		})
	}
	if len(read) == 0 {
		t.Fatal("found no annotation names in the visitor; this test no longer measures anything")
	}

	listed := map[string]bool{}
	for _, names := range documentAnnotations {
		for name := range names {
			listed[name] = true
		}
	}
	if missing := difference(read, listed); len(missing) > 0 {
		t.Errorf("the visitor reads %v but documentAnnotations does not list them — "+
			"MDL059 would reject a valid script", missing)
	}
	if extra := difference(listed, read); len(extra) > 0 {
		t.Errorf("documentAnnotations lists %v but no visitor reads them — the annotation "+
			"would parse and do nothing, unreported", extra)
	}
}

// mentionsAllAnnotation reports whether a function reads a createStatement's
// annotations, which is what makes it a document-annotation site.
func mentionsAllAnnotation(fn *goast.FuncDecl) bool {
	found := false
	goast.Inspect(fn.Body, func(n goast.Node) bool {
		if sel, ok := n.(*goast.SelectorExpr); ok && sel.Sel.Name == "AllAnnotation" {
			found = true
		}
		return !found
	})
	return found
}

// equalFoldLiterals collects the string literals compared against an annotation's
// NAME with strings.EqualFold — how every document-annotation site spells its
// test.
//
// The other operand must mention AnnotationName. Without that filter the walk
// also picks up EqualFold calls on an annotation's VALUE — applyEntityAccessAnnotation
// tests its parameter against "false" — and "false" is not an annotation name.
func equalFoldLiterals(body *goast.BlockStmt) []string {
	var out []string
	goast.Inspect(body, func(n goast.Node) bool {
		call, ok := n.(*goast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*goast.SelectorExpr)
		if !ok || sel.Sel.Name != "EqualFold" || !mentionsAnnotationName(call) {
			return true
		}
		for _, arg := range call.Args {
			lit, ok := arg.(*goast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			if s, err := strconv.Unquote(lit.Value); err == nil {
				out = append(out, s)
			}
		}
		return true
	})
	return out
}

// mentionsAnnotationName reports whether a call reads an annotation's name —
// either directly (ann.AnnotationName().GetText()) or through a local holding it
// (annName), which two of the entity sites use.
func mentionsAnnotationName(call *goast.CallExpr) bool {
	found := false
	goast.Inspect(call, func(n goast.Node) bool {
		switch t := n.(type) {
		case *goast.SelectorExpr:
			if t.Sel.Name == "AnnotationName" {
				found = true
			}
		case *goast.Ident:
			if t.Name == "annName" {
				found = true
			}
		}
		return !found
	})
	return found
}

func difference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// TestValidateDocumentAnnotations covers the three shapes that used to be
// silent, and the one that must stay silent.
func TestValidateDocumentAnnotations(t *testing.T) {
	for _, tc := range []struct {
		name string
		ann  ast.DocumentAnnotation
		want bool // want a violation
	}{
		// The typo. Costs the security setting and reports nothing.
		{"typo on a microflow", ast.DocumentAnnotation{Kind: "microflow", Name: "applyentityacces", Target: "M.F"}, true},
		// The right annotation on a document with no such property — created by
		// the change that added @applyentityaccess, and left open at the time.
		{"right name, wrong document", ast.DocumentAnnotation{Kind: "nanoflow", Name: "applyentityaccess"}, true},
		// A document kind that reads no annotation at all.
		{"annotation on a queue", ast.DocumentAnnotation{Kind: "queue", Name: "excluded"}, true},
		// An ACTIVITY annotation written before CREATE instead of inside the body.
		{"activity annotation at document level", ast.DocumentAnnotation{Kind: "microflow", Name: "caption"}, true},

		{"excluded on a microflow", ast.DocumentAnnotation{Kind: "microflow", Name: "excluded"}, false},
		{"applyentityaccess on a rule", ast.DocumentAnnotation{Kind: "rule", Name: "applyentityaccess"}, false},
		{"position on an entity", ast.DocumentAnnotation{Kind: "entity", Name: "position"}, false},
		{"anchor on an association", ast.DocumentAnnotation{Kind: "association", Name: "anchor"}, false},
		{"excluded on a page", ast.DocumentAnnotation{Kind: "page", Name: "excluded"}, false},
		// A create statement that did not parse has no kind, and a syntax error
		// is already reported for it — a second complaint would be noise.
		{"unparsed create statement", ast.DocumentAnnotation{Name: "excluded"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateDocumentAnnotations(&ast.Program{
				DocumentAnnotations: []ast.DocumentAnnotation{tc.ann},
			})
			if tc.want && len(got) == 0 {
				t.Fatalf("@%s on a %s was accepted", tc.ann.Name, tc.ann.Kind)
			}
			if !tc.want {
				if len(got) != 0 {
					t.Fatalf("@%s on a %s was rejected: %s", tc.ann.Name, tc.ann.Kind, got[0].Message)
				}
				return
			}
			if got[0].RuleID != "MDL059" || got[0].Severity != linter.SeverityError {
				t.Errorf("violation = %s/%v, want MDL059/error", got[0].RuleID, got[0].Severity)
			}
			// The message has to name the document kind, or a reader cannot tell
			// a typo from an annotation on the wrong document.
			if !strings.Contains(got[0].Message, tc.ann.Kind) {
				t.Errorf("message does not name the document kind: %s", got[0].Message)
			}
		})
	}
}

// TestDocumentAnnotationSuggestionNamesTheAlternatives — the suggestion has to
// say what the document DOES take, so the nanoflow case reads as an answer
// rather than a bare refusal.
func TestDocumentAnnotationSuggestionNamesTheAlternatives(t *testing.T) {
	if got := documentAnnotationSuggestion("microflow"); !strings.Contains(got, "@applyentityaccess") ||
		!strings.Contains(got, "@excluded") {
		t.Errorf("microflow suggestion = %q", got)
	}
	if got := documentAnnotationSuggestion("nanoflow"); !strings.Contains(got, "@excluded") {
		t.Errorf("nanoflow suggestion = %q", got)
	}
	if got := documentAnnotationSuggestion("queue"); !strings.Contains(got, "no annotations at all") {
		t.Errorf("queue suggestion = %q", got)
	}
}
