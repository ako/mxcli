// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// `create module role` was its own securityStatement rule with only an
// `or modify` prefix, so it took none of the prefixes every other document type
// gets from createStatement (#731): `or replace` was a parse error, and a doc
// comment in front of it was not attached. It is a createStatement kind now.
func TestCreateModuleRole_IsACreateStatementKind(t *testing.T) {
	for _, tc := range []struct {
		src        string
		orModify   bool
		desc       string
		deprecated bool
	}{
		{"create module role M.Admin;", false, "", false},
		{"create or modify module role M.Admin description 'Full access';", true, "Full access", false},
		{"create or replace module role M.Admin;", true, "", true},
		{"/** doc */ create or modify module role M.Admin;", true, "", false},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs[0])
		}
		if len(prog.Statements) != 1 {
			t.Fatalf("%s: got %d statements", tc.src, len(prog.Statements))
		}
		s, ok := prog.Statements[0].(*ast.CreateModuleRoleStmt)
		if !ok {
			t.Fatalf("%s: got %T", tc.src, prog.Statements[0])
		}
		if s.Name.String() != "M.Admin" || s.CreateOrModify != tc.orModify || s.DescriptionText() != tc.desc {
			t.Errorf("%s: got %+v", tc.src, s)
		}
		if got := len(prog.Deprecations) > 0; got != tc.deprecated {
			t.Errorf("%s: deprecations %v", tc.src, prog.Deprecations)
		}
	}
}

// `create module Role;` names a module called Role; moving module role into
// createStatement must not take that spelling away.
func TestCreateModuleNamedRole_StillAModule(t *testing.T) {
	prog, errs := Build("create module Role;")
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	if s, ok := prog.Statements[0].(*ast.CreateModuleStmt); !ok || s.Name != "Role" {
		t.Fatalf("got %#v", prog.Statements[0])
	}
}

func TestDropModuleRole_IfExistsIsParsed(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want bool
	}{
		{"drop module role if exists M.Admin;", true},
		{"drop module role M.Admin;", false},
	} {
		prog, errs := Build(tc.src)
		if len(errs) > 0 {
			t.Fatalf("%s: parse: %v", tc.src, errs[0])
		}
		s, ok := prog.Statements[0].(*ast.DropModuleRoleStmt)
		if !ok {
			t.Fatalf("%s: got %T", tc.src, prog.Statements[0])
		}
		if s.IfExists != tc.want || s.Name.String() != "M.Admin" {
			t.Errorf("%s: got %+v", tc.src, s)
		}
	}
}
