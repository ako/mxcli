// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mendixlabs/mxcli#1314: "the doc comment before `create or modify module
// MyFirstModule` is lost … that statement cannot store documentation, so the
// comment is ignored [MDL089]". A module has no documentation of its own, but
// its domain model does (DomainModels$DomainModel.Documentation, what lint reads
// as modules().domain_model_documentation), and the doc comment on `create
// module` is where that is spelled — as describe module prints it.
func TestCreateModuleDocCommentIsStored(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want *ast.CreateModuleStmt
	}{{
		name: "create or modify with a comment",
		src:  "/** Sales data */\ncreate or modify module MyFirstModule;",
		want: &ast.CreateModuleStmt{Name: "MyFirstModule", Documentation: "Sales data",
			DocumentationSet: true, CreateOrModify: true},
	}, {
		name: "plain create with a comment",
		src:  "/** Sales data */ create module Sales;",
		want: &ast.CreateModuleStmt{Name: "Sales", Documentation: "Sales data", DocumentationSet: true},
	}, {
		// An empty comment is the clearing spelling (mendixlabs/mxcli#1018).
		name: "empty comment clears",
		src:  "/** */ create or modify module Sales;",
		want: &ast.CreateModuleStmt{Name: "Sales", DocumentationSet: true, CreateOrModify: true},
	}, {
		// Control: no comment says nothing about documentation, so a rerun of
		// a script that never mentioned it cannot blank what Studio Pro wrote.
		name: "control: no comment leaves it unset",
		src:  "create or modify module Sales;",
		want: &ast.CreateModuleStmt{Name: "Sales", CreateOrModify: true},
	}} {
		t.Run(tc.name, func(t *testing.T) {
			prog := mustBuild(t, tc.src)
			if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], tc.want) {
				t.Fatalf("got %#v, want %#v", prog.Statements, tc.want)
			}
			if len(prog.DetachedDocComments) != 0 {
				t.Errorf("doc comment reported as lost (MDL089): %+v", prog.DetachedDocComments)
			}
		})
	}
}

// mendixlabs/mxcli#1314: "alter app security has no property "AdminUserName"".
// Security$ProjectSecurity.AdminUserName is the name of the built-in
// administrator account (MxAdmin by default); the password stays out of MDL
// (mendixlabs/mxcli#624).
func TestAlterAppSecurityAdminUserName(t *testing.T) {
	prog := mustBuild(t, "alter app security (AdminUserName: 'appadmin');")
	want := &ast.AlterProjectSecurityStmt{AdminUserName: "appadmin"}
	if len(prog.Statements) != 1 || !reflect.DeepEqual(prog.Statements[0], want) {
		t.Fatalf("got %#v, want %#v", prog.Statements, want)
	}
	for src, msg := range map[string]string{
		"alter app security ( AdminUserName: '' );":   "empty",
		"alter app security ( AdminUserName: true );": "name",
		"alter app security ( AdminUserName: 12 );":   "name",
	} {
		if _, errs := Build(src); len(errs) == 0 {
			t.Errorf("%s: accepted, want an error mentioning %q", src, msg)
		}
	}
}
