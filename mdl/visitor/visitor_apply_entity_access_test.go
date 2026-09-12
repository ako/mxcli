// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func microflowStmt(t *testing.T, src string) *ast.CreateMicroflowStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse: %v", e)
		}
		t.FailNow()
	}
	for _, s := range prog.Statements {
		if mf, ok := s.(*ast.CreateMicroflowStmt); ok {
			return mf
		}
	}
	t.Fatalf("no CreateMicroflowStmt in:\n%s", src)
	return nil
}

// TestApplyEntityAccessAnnotation — absent is NOT false.
//
// The field is a pointer precisely so the executor can tell "the script did not
// mention it" (preserve the stored setting) from "the script said off". A plain
// bool made every rewrite that omitted the annotation turn the security setting
// off, which is the bug this parses for.
func TestApplyEntityAccessAnnotation(t *testing.T) {
	body := "create or modify microflow M.Secured ()\nbegin\n  return;\nend;"

	if got := microflowStmt(t, body).ApplyEntityAccess; got != nil {
		t.Errorf("absent annotation = %v, want nil (preserve the stored value)", *got)
	}

	on := microflowStmt(t, "@applyentityaccess\n"+body).ApplyEntityAccess
	if on == nil || !*on {
		t.Errorf("@applyentityaccess = %v, want true", on)
	}

	off := microflowStmt(t, "@applyentityaccess(false)\n"+body).ApplyEntityAccess
	if off == nil || *off {
		t.Errorf("@applyentityaccess(false) = %v, want false", off)
	}

	// The explicit-true spelling is accepted too, so a script can be emphatic.
	explicit := microflowStmt(t, "@applyentityaccess(true)\n"+body).ApplyEntityAccess
	if explicit == nil || !*explicit {
		t.Errorf("@applyentityaccess(true) = %v, want true", explicit)
	}
}

// TestApplyEntityAccessAnnotationOnRule — a rule carries the same property, and
// its create path had the same gap: rule_write.go plumbed it through while
// nothing ever set it.
func TestApplyEntityAccessAnnotationOnRule(t *testing.T) {
	src := "@applyentityaccess\ncreate or modify rule M.IsOk ()\nreturns Boolean\nbegin\n  return true;\nend;"
	prog, errs := Build(src)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("parse: %v", e)
		}
		t.FailNow()
	}
	for _, s := range prog.Statements {
		if r, ok := s.(*ast.CreateRuleStmt); ok {
			if r.ApplyEntityAccess == nil || !*r.ApplyEntityAccess {
				t.Errorf("rule @applyentityaccess = %v, want true", r.ApplyEntityAccess)
			}
			return
		}
	}
	t.Fatal("no CreateRuleStmt parsed")
}
