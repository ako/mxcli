// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func publishedRestWith(prop string) string {
	return `create or modify published rest service M.Api (
  Path: 'rest/api/v1',
  ` + prop + `
) {
  resource 'items' { get '' microflow M.GetItems; }
};`
}

// The methods keep the order written: Studio Pro stores them in the order they
// were ticked (ako/TestApp: `Microflow, Session`), so sorting would rewrite an
// unchanged service.
func TestPublishedRestAuthentication_Methods(t *testing.T) {
	cases := []struct {
		prop string
		want *ast.PublishedRestAuthentication
	}{
		{"Authentication: none", &ast.PublishedRestAuthentication{}},
		{"Authentication: (basic)", &ast.PublishedRestAuthentication{Methods: []string{"Basic"}}},
		{"Authentication: (microflow M.Auth, session)", &ast.PublishedRestAuthentication{Methods: []string{"Microflow", "Session"}, Microflow: "M.Auth"}},
		{"Authentication: (basic, session, microflow M.Auth,)", &ast.PublishedRestAuthentication{Methods: []string{"Basic", "Session", "Microflow"}, Microflow: "M.Auth"}},
	}
	for _, c := range cases {
		t.Run(c.prop, func(t *testing.T) {
			prog, errs := Build("mdl 1;\n" + publishedRestWith(c.prop))
			if len(errs) > 0 {
				t.Fatalf("errors: %v", errs)
			}
			stmt := prog.Statements[len(prog.Statements)-1].(*ast.CreatePublishedRestServiceStmt)
			if !reflect.DeepEqual(stmt.Authentication, c.want) {
				t.Errorf("Authentication = %+v, want %+v", stmt.Authentication, c.want)
			}
		})
	}
}

// Not stating it is not `none`: the executor keeps the stored setting.
func TestPublishedRestAuthentication_OmittedIsNil(t *testing.T) {
	prog, errs := Build(publishedRestWith("Version: '1.0.0'"))
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if a := prog.Statements[0].(*ast.CreatePublishedRestServiceStmt).Authentication; a != nil {
		t.Errorf("Authentication = %+v, want nil", a)
	}
}

func TestPublishedRestAuthentication_Rejections(t *testing.T) {
	cases := map[string]string{
		"Authentication: (basic, basic)":                 "listed twice",
		"Authentication: (microflow M.A, microflow M.B)": "listed twice",
		"Authentication: 'basic'":                        "takes",
		"Path: none":                                     "takes",
		"Version: (basic)":                               "takes",
	}
	for prop, want := range cases {
		t.Run(prop, func(t *testing.T) {
			_, errs := Build("mdl 1;\n" + publishedRestWith(prop))
			var all []string
			for _, e := range errs {
				all = append(all, e.Error())
			}
			if !strings.Contains(strings.Join(all, "\n"), want) {
				t.Errorf("errors %q do not mention %q", all, want)
			}
		})
	}
}

func TestAlterPublishedRestService_SetPropertyList(t *testing.T) {
	prog, errs := Build(`alter published rest service M.Api set (Version: '2.0.0', Authentication: (session));`)
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.AlterPublishedRestServiceStmt)
	set := stmt.Actions[0].(*ast.PublishedRestSetAction)
	if set.Changes["Version"] != "2.0.0" {
		t.Errorf("Changes = %v", set.Changes)
	}
	want := &ast.PublishedRestAuthentication{Methods: []string{"Session"}}
	if !reflect.DeepEqual(set.Authentication, want) {
		t.Errorf("Authentication = %+v, want %+v", set.Authentication, want)
	}
}
