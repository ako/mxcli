// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

// knownActivityAnnotations restates the visitor's switch arms, and the two must
// not drift: a name in the visitor but not here is REJECTED though it works, and
// a name here but not in the visitor is ACCEPTED though it does nothing — which
// is the silent-drop bug MDL059 exists to end.
//
// Rather than trust the copy, parse extractMicroflowAnnotations' case labels out
// of the visitor's source and compare. (upstream #884)
func TestKnownAnnotationsMatchTheVisitor(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../visitor/visitor_microflow_statements.go", nil, 0)
	if err != nil {
		t.Fatalf("parse visitor: %v", err)
	}

	visitorNames := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "extractMicroflowAnnotations" {
			return true
		}
		ast.Inspect(fn.Body, func(m ast.Node) bool {
			cc, ok := m.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				lit, ok := e.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if s, err := strconv.Unquote(lit.Value); err == nil {
					visitorNames[s] = true
				}
			}
			return true
		})
		return false
	})

	// hasLaterActivityAnnotation lists a subset of the same names; the scan above
	// is scoped to extractMicroflowAnnotations so it cannot pick those up twice.
	if len(visitorNames) == 0 {
		t.Fatal("no case labels found in extractMicroflowAnnotations — the scan is broken, " +
			"and a broken scan makes this test pass vacuously")
	}

	for name := range visitorNames {
		if !knownActivityAnnotations[name] {
			t.Errorf("the visitor implements @%s but knownActivityAnnotations omits it — "+
				"MDL059 would reject an annotation that actually works", name)
		}
	}
	for name := range knownActivityAnnotations {
		if !visitorNames[name] {
			t.Errorf("knownActivityAnnotations lists @%s but the visitor has no case for it — "+
				"it would be accepted and silently do nothing", name)
		}
	}
}
