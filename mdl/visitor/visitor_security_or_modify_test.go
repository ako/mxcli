// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mxcli-formula1 suggested issue 12: `create module role` had no `or modify`
// form, so a security script failed on the first role that already existed and
// role creation had to live in its own run-once file.
func TestCreateModuleRole_OrModify(t *testing.T) {
	cases := []struct {
		src            string
		wantOrModify   bool
		wantName, desc string
	}{
		{"create module role Sec.ApiUser;", false, "ApiUser", ""},
		{"create or modify module role Sec.ApiUser;", true, "ApiUser", ""},
		{"create or modify module role Sec.ApiUser description 'API consumer';", true, "ApiUser", "API consumer"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			prog, errs := Build(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			var got *ast.CreateModuleRoleStmt
			for _, s := range prog.Statements {
				if r, ok := s.(*ast.CreateModuleRoleStmt); ok {
					got = r
				}
			}
			if got == nil {
				t.Fatal("no CreateModuleRoleStmt produced")
			}
			if got.CreateOrModify != tc.wantOrModify {
				t.Errorf("CreateOrModify = %v, want %v", got.CreateOrModify, tc.wantOrModify)
			}
			if got.Name.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", got.Name.Name, tc.wantName)
			}
			if got.DescriptionText() != tc.desc {
				t.Errorf("Description = %q, want %q", got.DescriptionText(), tc.desc)
			}
		})
	}
}

// `create module` and `create module role` differ only after the third token, so
// the optional OR MODIFY must not make one shadow the other.
func TestCreateModuleAndModuleRoleStayDistinct(t *testing.T) {
	prog, errs := Build("create or modify module Sec;\ncreate or modify module role Sec.ApiUser;")
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	var modules, roles int
	for _, s := range prog.Statements {
		switch s.(type) {
		case *ast.CreateModuleStmt:
			modules++
		case *ast.CreateModuleRoleStmt:
			roles++
		}
	}
	if modules != 1 || roles != 1 {
		t.Errorf("got %d module and %d module-role statements, want 1 each", modules, roles)
	}
}
