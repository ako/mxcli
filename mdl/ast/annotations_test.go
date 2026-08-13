// SPDX-License-Identifier: Apache-2.0

package ast

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// StatementAnnotations reads the Annotations field reflectively rather than
// through a type switch, because its whole job is to report annotations that
// were silently dropped — and a hand-written switch that silently skips a
// statement type reintroduces exactly that bug one level up.
//
// The accessor keys on one thing: a field NAMED "Annotations" whose type is
// *ActivityAnnotations. This test parses the package's own source for every
// struct field of that type and asserts each is named accordingly, so a type
// that spells it differently (or embeds it) fails here rather than silently
// falling out of annotation validation. (upstream #884)
func TestEveryActivityAnnotationsFieldIsNamedAnnotations(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parse ast package: %v", err)
	}

	type field struct{ typeName, fieldName, pos string }
	var found []field

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					return true
				}
				for _, f := range st.Fields.List {
					star, ok := f.Type.(*ast.StarExpr)
					if !ok {
						continue
					}
					id, ok := star.X.(*ast.Ident)
					if !ok || id.Name != "ActivityAnnotations" {
						continue
					}
					if len(f.Names) == 0 { // embedded
						found = append(found, field{ts.Name.Name, "<embedded>", fset.Position(f.Pos()).String()})
						continue
					}
					for _, name := range f.Names {
						found = append(found, field{ts.Name.Name, name.Name, fset.Position(name.Pos()).String()})
					}
				}
				return true
			})
		}
	}

	// A broken scan would make every assertion below pass vacuously.
	if len(found) < 10 {
		t.Fatalf("found only %d *ActivityAnnotations fields — the source scan is broken", len(found))
	}

	for _, f := range found {
		if f.fieldName != "Annotations" {
			t.Errorf("%s carries *ActivityAnnotations as %q (%s), but StatementAnnotations looks up "+
				"the field named \"Annotations\" — this type's annotations would be silently skipped "+
				"by validation, which is the bug #884 is about", f.typeName, f.fieldName, f.pos)
		}
	}
}

// The accessor itself, against real statement types: one that carries
// annotations, one that does not, and the nil cases.
func TestStatementAnnotations(t *testing.T) {
	ann := &ActivityAnnotations{UnknownNames: []string{"size"}}

	withAnnotations := []MicroflowStatement{
		&DeclareStmt{Annotations: ann},
		&ReturnStmt{Annotations: ann},
		&IfStmt{Annotations: ann},
		&LoopStmt{Annotations: ann},
	}
	for _, s := range withAnnotations {
		got := StatementAnnotations(s)
		if got == nil {
			t.Errorf("%T: got nil, want the annotations that were set", s)
			continue
		}
		if len(got.UnknownNames) != 1 || got.UnknownNames[0] != "size" {
			t.Errorf("%T: UnknownNames = %v, want [size]", s, got.UnknownNames)
		}
	}

	// A statement whose Annotations field is nil reads as nil, not as a panic.
	if got := StatementAnnotations(&DeclareStmt{}); got != nil {
		t.Errorf("unset Annotations = %v, want nil", got)
	}
	if got := StatementAnnotations(nil); got != nil {
		t.Errorf("nil statement = %v, want nil", got)
	}
	if got := StatementAnnotations((*DeclareStmt)(nil)); got != nil {
		t.Errorf("typed-nil statement = %v, want nil", got)
	}
}
