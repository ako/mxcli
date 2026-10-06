// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"reflect"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
)

// restCallFlow wraps a call rest service statement in a microflow.
func restCallFlow(stmt string) string {
	return "create microflow M.F ($T: String, $U: String, $Doc: M.Download, $Fb: M.Feedback) begin " + stmt + "; end;"
}

// restCallPairs are the clause form (MDL-DEPR720) and the settings list
// (ADR-0013) of the same activity: every body and every result handling.
var restCallPairs = []struct{ name, old, canon string }{
	{"headers and timeout",
		"$R = call rest service get 'https://x/{1}' with ({1} = $T) header 'Accept' = 'text/html' header XKey = $U timeout 300 returns String",
		"$R = call rest service get 'https://x/{1}' with ({1} = $T) (Headers: ('Accept': 'text/html', 'XKey': $U), Timeout: 300) returns String"},
	{"auth and template body",
		"$R = call rest service post 'https://x' auth basic $U password $T body '{\"a\": \"{1}\"}' with ({1} = $T) returns response",
		"$R = call rest service post 'https://x' (Authentication: basic (Username: $U, Password: $T), Body: template '{\"a\": \"{1}\"}' with ({1} = $T)) returns response"},
	{"template body without parameters",
		"call rest service post 'https://x' body 'ping' returns nothing",
		"call rest service post 'https://x' (Body: template 'ping') returns nothing"},
	{"expression body",
		"call rest service put 'https://x' body $T returns nothing",
		"call rest service put 'https://x' (Body: $T) returns nothing"},
	{"expression body with parameters",
		"call rest service put 'https://x' body $T with ({1} = $U) returns nothing",
		"call rest service put 'https://x' (Body: $T with ({1} = $U)) returns nothing"},
	{"binary body, file document result",
		"$R = call rest service post 'https://x' body binary $Doc/Contents returns M.Download",
		"$R = call rest service post 'https://x' (Body: binary $Doc/Contents) returns M.Download"},
	{"mapping body, mapping result",
		"$R = call rest service post 'https://x' body mapping M.EXM from $Fb returns mapping M.IMM as M.Helper",
		"$R = call rest service post 'https://x' (Body: mapping M.EXM from $Fb) returns mapping M.IMM as M.Helper"},
	{"list result and error handling",
		"$R = call rest service get 'https://x' timeout 5 returns mapping M.IMM as list of M.Helper on error continue",
		"$R = call rest service get 'https://x' (Timeout: 5) returns mapping M.IMM as list of M.Helper on error continue"},
	{"upper case",
		"$R = CALL REST SERVICE DELETE 'https://x' HEADER 'A' = 'b' AUTH BASIC 'u' PASSWORD 'p' TIMEOUT 1 RETURNS STRING",
		"$R = CALL REST SERVICE DELETE 'https://x' (Headers: ('A': 'b'), Authentication: BASIC (Username: 'u', Password: 'p'), Timeout: 1) RETURNS STRING"},
}

// The clause form is a respelling of the settings list: both build the same
// statement, only the clauses record MDL-DEPR720, and the rewrite fmt --upgrade
// applies is attached.
func TestRestCallClausesAreAnAliasOfTheSettingsList(t *testing.T) {
	for _, p := range restCallPairs {
		t.Run(p.name, func(t *testing.T) {
			old := mustBuild(t, restCallFlow(p.old))
			canon := mustBuild(t, restCallFlow(p.canon))
			if got := deprecationCodes(canon); len(got) != 0 {
				t.Errorf("settings list recorded %v, want none", got)
			}
			if got := deprecationCodes(old); len(got) != 1 || got[0] != deprecation.RestCallClauses {
				t.Fatalf("clause form recorded %v, want [%s]", got, deprecation.RestCallClauses)
			}
			if old.Deprecations[0].Fix == nil {
				t.Errorf("MDL-DEPR720 recorded without a rewrite (%s)", old.Deprecations[0].NoFix)
			}
			if !reflect.DeepEqual(old.Statements, canon.Statements) {
				t.Errorf("clause form and settings list build different statements:\n old:   %#v\n canon: %#v",
					restCallOf(t, old), restCallOf(t, canon))
			}
		})
	}
}

// The settings list sets what the clauses set: a direct check on the AST, so
// a builder that dropped the list entirely would not pass by building nothing
// on both sides.
func TestRestCallSettingsListBuildsEverySetting(t *testing.T) {
	prog := mustBuild(t, restCallFlow("$R = call rest service post 'https://x' (\n"+
		"  Headers: ('Accept': 'text/html', 'X-Key': $U),\n"+
		"  Authentication: basic (Username: $U, Password: $T),\n"+
		"  Body: template '{1}' with ({1} = $T),\n"+
		"  Timeout: 300,\n"+
		") returns String"))
	rc := restCallOf(t, prog)
	if len(rc.Headers) != 2 || rc.Headers[0].Name != "Accept" || rc.Headers[1].Name != "X-Key" {
		t.Errorf("Headers = %+v", rc.Headers)
	}
	if rc.Auth == nil || rc.Auth.Username == nil || rc.Auth.Password == nil {
		t.Errorf("Auth = %+v", rc.Auth)
	}
	if rc.Body == nil || rc.Body.Type != ast.RestBodyCustom || len(rc.Body.TemplateParams) != 1 {
		t.Errorf("Body = %+v", rc.Body)
	}
	if rc.Timeout == nil {
		t.Errorf("Timeout not set")
	}
}

// R11: the list is new syntax, so a wrong key, a repeated key or a misshapen
// value is an error, and the two forms cannot be mixed.
func TestRestCallSettingsListIsStrict(t *testing.T) {
	cases := map[string]string{
		"unknown key":             "call rest service get 'https://x' (Header: ('A': 'b')) returns nothing",
		"repeated key":            "call rest service get 'https://x' (Timeout: 1, Timeout: 2) returns nothing",
		"headers not a map":       "call rest service get 'https://x' (Headers: 'A') returns nothing",
		"header name a word":      "call rest service get 'https://x' (Headers: (Accept: 'x')) returns nothing",
		"repeated header":         "call rest service get 'https://x' (Headers: ('A': 'x', 'a': 'y')) returns nothing",
		"auth not basic":          "call rest service get 'https://x' (Authentication: $U) returns nothing",
		"auth missing pw":         "call rest service get 'https://x' (Authentication: basic (Username: $U)) returns nothing",
		"auth unknown key":        "call rest service get 'https://x' (Authentication: basic (User: $U, Password: $T)) returns nothing",
		"timeout misshapen":       "call rest service get 'https://x' (Timeout: binary $T) returns nothing",
		"body a map":              "call rest service get 'https://x' (Body: ('a': 'b')) returns nothing",
		"mixed: list then clause": "call rest service get 'https://x' (Timeout: 1) header 'A' = 'b' returns nothing",
		"mixed: clause then list": "call rest service get 'https://x' header 'A' = 'b' (Timeout: 1) returns nothing",
	}
	for name, stmt := range cases {
		t.Run(name, func(t *testing.T) {
			if _, errs := Build(restCallFlow(stmt)); len(errs) == 0 {
				t.Errorf("%s: no error", stmt)
			}
		})
	}
}

func restCallOf(t *testing.T, prog *ast.Program) *ast.RestCallStmt {
	t.Helper()
	mf, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok || len(mf.Body) == 0 {
		t.Fatalf("not a microflow with a body: %#v", prog.Statements[0])
	}
	rc, ok := mf.Body[0].(*ast.RestCallStmt)
	if !ok {
		t.Fatalf("first statement is %T, want *ast.RestCallStmt", mf.Body[0])
	}
	return rc
}
