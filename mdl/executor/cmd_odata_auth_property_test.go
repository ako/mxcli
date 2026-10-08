// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// authenticatedService is existingPublishedService with stored authentication.
func authenticatedService(t *testing.T, types []string, mf string) (*model.PublishedODataService, **model.PublishedODataService, *ExecContext) {
	t.Helper()
	svc, mb, h := existingPublishedService()
	svc.AuthenticationTypes, svc.AuthMicroflow = types, mf
	updated := new(*model.PublishedODataService)
	mb.UpdatePublishedODataServiceFunc = func(s *model.PublishedODataService) error { *updated = s; return nil }
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return svc, updated, ctx
}

func odataAuthOf(s *model.PublishedODataService) string {
	if s == nil {
		return "<not written>"
	}
	return strings.Join(s.AuthenticationTypes, ",") + " / " + s.AuthMicroflow
}

// Stated authentication replaces the stored methods and microflow together;
// `none` clears them, which the clause could not say. Unstated keeps them.
func TestModifyODataService_AuthenticationProperty(t *testing.T) {
	cases := []struct {
		name string
		stmt *ast.CreateODataServiceStmt
		want string
	}{
		{"unstated keeps", &ast.CreateODataServiceStmt{}, "Microflow,Basic / MyModule.Auth"},
		{"none clears", &ast.CreateODataServiceStmt{AuthenticationSet: true}, " / "},
		{"methods replace", &ast.CreateODataServiceStmt{AuthenticationSet: true, AuthenticationTypes: []string{"Session"}}, "Session / "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, updated, ctx := authenticatedService(t, []string{"Microflow", "Basic"}, "MyModule.Auth")
			c.stmt.Name = ast.QualifiedName{Module: "MyModule", Name: "CatalogService"}
			c.stmt.CreateOrModify = true
			assertNoError(t, createODataService(ctx, c.stmt))
			if got := odataAuthOf(*updated); got != c.want {
				t.Errorf("authentication = %q, want %q", got, c.want)
			}
		})
	}
}

// `alter … set (Authentication: …)` is new: before it there was no way to
// change a published service's authentication short of restating the service.
func TestAlterODataService_AuthenticationProperty(t *testing.T) {
	cases := []struct {
		name string
		stmt *ast.AlterODataServiceStmt
		want string
	}{
		{"unstated keeps", &ast.AlterODataServiceStmt{Changes: map[string]any{"Version": "2.0.0"}}, "Microflow,Basic / MyModule.Auth"},
		{"none clears", &ast.AlterODataServiceStmt{AuthenticationSet: true}, " / "},
		{"methods replace", &ast.AlterODataServiceStmt{AuthenticationSet: true, AuthenticationTypes: []string{"Basic", "Session"}}, "Basic,Session / "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, updated, ctx := authenticatedService(t, []string{"Microflow", "Basic"}, "MyModule.Auth")
			c.stmt.Name = ast.QualifiedName{Module: "MyModule", Name: "CatalogService"}
			assertNoError(t, alterODataService(ctx, c.stmt))
			if got := odataAuthOf(*updated); got != c.want {
				t.Errorf("authentication = %q, want %q", got, c.want)
			}
		})
	}
}

// describe prints the property in the stored order, in the canonical form: its
// output parses with no deprecation and builds the stored setting back.
func TestDescribeODataService_AuthenticationProperty(t *testing.T) {
	cases := []struct {
		types []string
		mf    string
		line  string
	}{
		{[]string{"Basic"}, "", "  Authentication: (basic)"},
		{[]string{"Microflow", "Session"}, "MyModule.Auth", "  Authentication: (microflow MyModule.Auth, session)"},
		{[]string{"Guest"}, "", "  Authentication: (guest)"},
		{nil, "", ""},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			svc, _, ctx := authenticatedService(t, c.types, c.mf)
			var out bytes.Buffer
			ctx.Output = &out
			assertNoError(t, outputPublishedODataServiceMDL(ctx, svc, "MyModule", ""))
			text := out.String()
			if c.line == "" {
				if strings.Contains(text, "uthentication") {
					t.Fatalf("printed the default (none):\n%s", text)
				}
				return
			}
			if !strings.Contains(text, c.line) {
				t.Fatalf("describe lacks %q:\n%s", c.line, text)
			}
			prog, errs := visitor.Build(text)
			if len(errs) > 0 || len(prog.Deprecations) > 0 {
				t.Fatalf("describe output: errors %v, deprecations %v\n%s", errs, prog.Deprecations, text)
			}
			s := prog.Statements[0].(*ast.CreateODataServiceStmt)
			if !s.AuthenticationSet || !reflect.DeepEqual(s.AuthenticationTypes, c.types) || s.AuthMicroflow != c.mf {
				t.Errorf("re-parsed as %v %q, want %v %q", s.AuthenticationTypes, s.AuthMicroflow, c.types, c.mf)
			}
		})
	}
}

// A stored setting MDL cannot state is a comment, not the property, so
// replaying the output keeps it rather than writing something else. Printing
// `Microflow M.F` for a microflow stored without the Microflow method used to
// add that method on replay.
func TestDescribeODataService_UnspellableAuthenticationIsAComment(t *testing.T) {
	for name, st := range map[string]struct {
		types []string
		mf    string
	}{
		"microflow without the method": {[]string{"Basic"}, "MyModule.Auth"},
		"microflow alone":              {nil, "MyModule.Auth"},
		"method without the microflow": {[]string{"Microflow"}, ""},
		"unknown method":               {[]string{"Custom"}, ""},
	} {
		t.Run(name, func(t *testing.T) {
			svc, _, ctx := authenticatedService(t, st.types, st.mf)
			var out bytes.Buffer
			ctx.Output = &out
			assertNoError(t, outputPublishedODataServiceMDL(ctx, svc, "MyModule", ""))
			text := out.String()
			if strings.Contains(text, "Authentication:") || strings.Contains(text, "\nauthentication ") {
				t.Errorf("printed an unspellable setting:\n%s", text)
			}
			if !strings.Contains(text, "-- Authentication is stored as") {
				t.Errorf("no comment for the stored setting:\n%s", text)
			}
			if _, errs := visitor.Build(text); len(errs) > 0 {
				t.Errorf("output does not parse: %v\n%s", errs, text)
			}
		})
	}
}
