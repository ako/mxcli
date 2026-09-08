// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"testing"
)

// projectCheckExemptDocTypes lists the doc types stmtCreateInfo can return that
// deliberately have NO project-existence check, with the reason. An entry here
// is a decision; anything else missing from setFor is the bug this test exists
// to catch.
var projectCheckExemptDocTypes = map[string]string{
	// CREATE MODULE on an existing module is a no-op: execCreateModule prints
	// "already exists" and returns nil (exit 0). `create module M;` is the
	// standard script preamble, so flagging it would be a false positive on
	// essentially every script.
	"module": "CREATE MODULE is idempotent at exec — it prints 'already exists' and exits 0",
}

// docTypesFromSwitch parses validate_duplicates.go and returns, for the named
// function, either the first string literal of every return statement (kind
// "return") or every case-clause string literal (kind "case").
//
// Reading the real source rather than restating the lists is the point: this is
// the "two lists, nothing comparing them" defect class, and a guard that keeps
// its own third copy of the list would join the problem rather than fix it.
func docTypesFromSwitch(t *testing.T, funcName, kind string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "validate_duplicates.go", nil, 0)
	if err != nil {
		t.Fatalf("parse validate_duplicates.go: %v", err)
	}

	out := map[string]bool{}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == funcName {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatalf("function %s not found in validate_duplicates.go", funcName)
	}

	lit := func(e ast.Expr) (string, bool) {
		bl, ok := e.(*ast.BasicLit)
		if !ok || bl.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(bl.Value)
		if err != nil || v == "" {
			return "", false
		}
		return v, true
	}

	ast.Inspect(fn, func(n ast.Node) bool {
		switch kind {
		case "return":
			if r, ok := n.(*ast.ReturnStmt); ok && len(r.Results) > 0 {
				if v, ok := lit(r.Results[0]); ok {
					out[v] = true
				}
			}
		case "case":
			if c, ok := n.(*ast.CaseClause); ok {
				for _, e := range c.List {
					if v, ok := lit(e); ok {
						out[v] = true
					}
				}
			}
		}
		return true
	})
	if len(out) == 0 {
		t.Fatalf("extracted no doc types from %s (%s) — the guard would pass vacuously", funcName, kind)
	}
	return out
}

// TestEveryCreateDocTypeIsProjectChecked is the guard for the defect behind the
// `check --references` under-report: stmtCreateInfo classified more document
// types than projectNameSets.setFor knew about, so a plain CREATE of an
// existing association, rule or javascript action passed `check --references`
// and then died half-way through `exec`, leaving the project part-modified.
//
// Nothing compared the two switches, so the gap was silent. This does.
func TestEveryCreateDocTypeIsProjectChecked(t *testing.T) {
	created := docTypesFromSwitch(t, "stmtCreateInfo", "return")
	checked := docTypesFromSwitch(t, "setFor", "case")

	var missing []string
	for dt := range created {
		if checked[dt] {
			continue
		}
		if _, exempt := projectCheckExemptDocTypes[dt]; exempt {
			continue
		}
		missing = append(missing, dt)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("stmtCreateInfo returns doc types that projectNameSets.setFor does not check: %v\n"+
			"A plain CREATE of one of these against a project that already has it passes "+
			"`check --references` and then fails at exec, part-way through the script.\n"+
			"Add a set for it in projectNameSets/loadProjectNameSets/setFor, or, if the CREATE is "+
			"genuinely idempotent at exec, record it in projectCheckExemptDocTypes with the reason.",
			missing)
	}

	// The exemption list must not outlive its reason: an entry that setFor has
	// since grown a case for, or that stmtCreateInfo no longer produces, is
	// stale and would mask a real gap.
	for dt := range projectCheckExemptDocTypes {
		if !created[dt] {
			t.Errorf("projectCheckExemptDocTypes has %q, which stmtCreateInfo no longer returns — drop it", dt)
		}
		if checked[dt] {
			t.Errorf("projectCheckExemptDocTypes has %q, but setFor now checks it — drop the exemption", dt)
		}
	}
}

// TestEveryDropDocTypeIsCreatable is the mirror: stmtDropInfo feeds the
// droppedFromProject registry that suppresses a conflict after a DROP. A doc
// type DROP knows and CREATE does not is harmless, but the reverse means a
// `drop X; create X;` pair reports a conflict the script already resolved.
func TestEveryDropDocTypeIsCreatable(t *testing.T) {
	created := docTypesFromSwitch(t, "stmtCreateInfo", "return")
	dropped := docTypesFromSwitch(t, "stmtDropInfo", "return")

	var missing []string
	for dt := range created {
		if dropped[dt] {
			continue
		}
		missing = append(missing, dt)
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("stmtCreateInfo returns doc types stmtDropInfo does not classify: %v\n"+
			"`drop X; create X;` would then report a project conflict the script already resolved.",
			missing)
	}
}

// TestIfNotExistsCountsAsIdempotent guards the third idempotency spelling.
//
// stmtCreateInfo reported OR MODIFY and OR REPLACE but not IF NOT EXISTS, so a
// re-runnable domain script — the form that exists precisely to be re-run —
// was told its `create entity if not exists` conflicted with the project, for
// a statement exec skips with "already exists — skipped".
//
// The guard reads both sides out of source: every `case *ast.XStmt` in
// stmtCreateInfo whose AST type declares an IfNotExists field must mention
// IfNotExists in that case's return. Neither list is restated here.
func TestIfNotExistsCountsAsIdempotent(t *testing.T) {
	// Which mdl/ast CREATE types declare an IfNotExists field.
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "../ast", nil, 0)
	if err != nil {
		t.Fatalf("parse mdl/ast: %v", err)
	}
	hasIfNotExists := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			ast.Inspect(f, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				st, ok := ts.Type.(*ast.StructType)
				if !ok || st.Fields == nil {
					return true
				}
				for _, fld := range st.Fields.List {
					for _, nm := range fld.Names {
						if nm.Name == "IfNotExists" {
							hasIfNotExists[ts.Name.Name] = true
						}
					}
				}
				return true
			})
		}
	}
	if len(hasIfNotExists) == 0 {
		t.Fatal("found no mdl/ast type with an IfNotExists field — the guard would pass vacuously")
	}

	// Which of them stmtCreateInfo handles, and whether its case consults the field.
	fset2 := token.NewFileSet()
	f, err := parser.ParseFile(fset2, "validate_duplicates.go", nil, 0)
	if err != nil {
		t.Fatalf("parse validate_duplicates.go: %v", err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "stmtCreateInfo" {
			fn = fd
			break
		}
	}
	if fn == nil {
		t.Fatal("stmtCreateInfo not found")
	}

	checked := 0
	ast.Inspect(fn, func(n ast.Node) bool {
		c, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, e := range c.List {
			star, ok := e.(*ast.StarExpr)
			if !ok {
				continue
			}
			sel, ok := star.X.(*ast.SelectorExpr)
			if !ok || !hasIfNotExists[sel.Sel.Name] {
				continue
			}
			checked++
			consulted := false
			ast.Inspect(c, func(m ast.Node) bool {
				if id, ok := m.(*ast.Ident); ok && id.Name == "IfNotExists" {
					consulted = true
				}
				return true
			})
			if !consulted {
				t.Errorf("stmtCreateInfo case *ast.%s ignores its IfNotExists field: "+
					"`create ... if not exists` on an element the project already has would be "+
					"reported as a conflict, for a statement exec cleanly skips. "+
					"Return `s.CreateOrModify || s.IfNotExists`.", sel.Sel.Name)
			}
		}
		return true
	})
	if checked == 0 {
		t.Fatal("stmtCreateInfo handles no type with an IfNotExists field — the guard would pass vacuously")
	}
}
