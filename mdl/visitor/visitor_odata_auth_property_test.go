// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

func odataServiceWith(props, tail string) string {
	return "create or modify published odata service M.S ( Path: 'odata/s/v1', Namespace: 'M'" + props + " )" + tail + ";"
}

// The property is the canonical spelling (R9). It keeps the order written and,
// unlike the clause, can say `none`.
func TestPublishedODataAuthenticationProperty(t *testing.T) {
	cases := []struct {
		props   string
		types   []string
		mf      string
		wantSet bool
	}{
		{"", nil, "", false},
		{", Authentication: none", nil, "", true},
		{", Authentication: (session, basic)", []string{"Session", "Basic"}, "", true},
		{", Authentication: (microflow M.Auth, basic,)", []string{"Microflow", "Basic"}, "M.Auth", true},
		{", Authentication: (guest)", []string{"Guest"}, "", true},
	}
	for _, c := range cases {
		t.Run(c.props, func(t *testing.T) {
			prog, errs := Build("mdl 1;\n" + odataServiceWith(c.props, ""))
			if len(errs) > 0 {
				t.Fatalf("errors: %v", errs)
			}
			s := prog.Statements[len(prog.Statements)-1].(*ast.CreateODataServiceStmt)
			if s.AuthenticationSet != c.wantSet || !reflect.DeepEqual(s.AuthenticationTypes, c.types) || s.AuthMicroflow != c.mf {
				t.Errorf("got set=%v %v %q, want set=%v %v %q", s.AuthenticationSet, s.AuthenticationTypes, s.AuthMicroflow, c.wantSet, c.types, c.mf)
			}
			if len(prog.Deprecations) != 0 {
				t.Errorf("the property recorded %v", deprecationCodes(prog))
			}
		})
	}
}

// The clause still builds the same statement, and records MDL-DEPR139.
func TestPublishedODataAuthenticationClauseIsAnAlias(t *testing.T) {
	old := mustBuild(t, odataServiceWith("", " authentication basic, microflow M.Auth"))
	canonical := mustBuild(t, odataServiceWith(", Authentication: (basic, microflow M.Auth)", ""))
	if got := deprecationCodes(old); !reflect.DeepEqual(got, []string{deprecation.ODataAuthenticationClause}) {
		t.Errorf("clause recorded %v, want [%s]", got, deprecation.ODataAuthenticationClause)
	}
	o, c := old.Statements[0].(*ast.CreateODataServiceStmt), canonical.Statements[0].(*ast.CreateODataServiceStmt)
	if !reflect.DeepEqual(o.AuthenticationTypes, c.AuthenticationTypes) || o.AuthMicroflow != c.AuthMicroflow || !o.AuthenticationSet {
		t.Errorf("clause built %v %q set=%v, property %v %q", o.AuthenticationTypes, o.AuthMicroflow, o.AuthenticationSet, c.AuthenticationTypes, c.AuthMicroflow)
	}
}

func TestPublishedODataAuthentication_Rejections(t *testing.T) {
	cases := map[string]string{
		odataServiceWith(", Authentication: (basic)", " authentication session"): "both",
		odataServiceWith(", Authentication: (basic, basic)", ""):                 "listed twice",
		odataServiceWith(", Authentication: (Custom)", ""):                       "Custom",
		odataServiceWith(", Authentication: 'basic'", ""):                        "Authentication takes",
	}
	for src, want := range cases {
		t.Run(want, func(t *testing.T) {
			_, errs := Build("mdl 1;\n" + src)
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

func TestAlterPublishedODataService_SetsAuthentication(t *testing.T) {
	for src, want := range map[string]*ast.AlterODataServiceStmt{
		"alter published odata service M.S set (Version: '2', Authentication: (session));": {AuthenticationSet: true, AuthenticationTypes: []string{"Session"}},
		"alter published odata service M.S set (Authentication: none);":                    {AuthenticationSet: true},
		"alter published odata service M.S set (Version: '2');":                            {},
	} {
		t.Run(src, func(t *testing.T) {
			prog := mustBuild(t, src)
			s := prog.Statements[0].(*ast.AlterODataServiceStmt)
			if s.AuthenticationSet != want.AuthenticationSet || !reflect.DeepEqual(s.AuthenticationTypes, want.AuthenticationTypes) {
				t.Errorf("got set=%v %v, want set=%v %v", s.AuthenticationSet, s.AuthenticationTypes, want.AuthenticationSet, want.AuthenticationTypes)
			}
			if _, ok := s.Changes["Authentication"]; ok {
				t.Errorf("Authentication also landed in Changes: %v", s.Changes)
			}
		})
	}
}
