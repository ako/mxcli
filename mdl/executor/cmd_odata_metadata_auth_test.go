// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// mxcli-formula1 findings #23: CREATE ODATA CLIENT accepts UseAuthentication /
// HttpUsername / HttpPassword and stores them for the runtime, but the
// design-time $metadata fetch was a bare client.Get. Against a service behind
// `authentication basic` that is a 401, and because the fetch failure is only a
// warning the client is created with no cached entity types — so the
// CREATE EXTERNAL ENTITIES that follows imports nothing and the script looks
// like it succeeded.
func TestFetchODataMetadata_SendsCredentialsAndHeaders(t *testing.T) {
	const body = `<?xml version="1.0"?><edmx:Edmx xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx"/>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "f1api" || pass != "s3cret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.Header.Get("X-Probe") != "yes" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	auth := &metadataFetchAuth{
		Username: "f1api",
		Password: "s3cret",
		Headers:  map[string]string{"X-Probe": "yes"},
	}
	got, hash, err := fetchODataMetadata(srv.URL, auth)
	if err != nil {
		t.Fatalf("fetch with credentials failed: %v", err)
	}
	if got != body {
		t.Errorf("body = %q, want the served metadata", got)
	}
	if hash == "" {
		t.Error("no hash computed for a successful fetch")
	}

	// Without them, the same service is a 401 — which is what shipped.
	if _, _, err := fetchODataMetadata(srv.URL, nil); err == nil {
		t.Error("unauthenticated fetch succeeded, so the test server proves nothing")
	}
}

// A literal is usable at design time; a constant reference is not — the runtime
// resolves those, mxcli has nothing to resolve them against. Sending the
// constant's *name* as the password would be worse than sending nothing, so the
// name is reported instead.
func TestMetadataAuthFromStmt_LiteralsOnly(t *testing.T) {
	stmt := &ast.CreateODataClientStmt{
		HttpUsername:          "f1api",
		HttpUsernameIsLiteral: true,
		HttpPassword:          "Module.ApiPassword", // a constant reference
		Headers: []ast.HeaderDef{
			{Key: "X-Probe", Value: "yes", ValueIsLiteral: true},
			{Key: "X-Token", Value: "Module.Token"},
		},
	}
	auth := metadataAuthFromStmt(nil, stmt)

	if auth.Username != "f1api" {
		t.Errorf("Username = %q, want the literal f1api", auth.Username)
	}
	if auth.Password != "" {
		t.Errorf("Password = %q, want empty — a constant reference is not a value", auth.Password)
	}
	if auth.Headers["X-Probe"] != "yes" {
		t.Errorf("literal header dropped: %v", auth.Headers)
	}
	if _, ok := auth.Headers["X-Token"]; ok {
		t.Error("a constant-reference header was sent as its own name")
	}
	// Both unresolved names are reported, sorted, so the user learns why the
	// fetch went out unauthenticated.
	want := []string{"HttpPassword (Module.ApiPassword)", "header X-Token (Module.Token)"}
	if len(auth.Unresolved) != len(want) {
		t.Fatalf("Unresolved = %v, want %v", auth.Unresolved, want)
	}
	for i := range want {
		if auth.Unresolved[i] != want[i] {
			t.Errorf("Unresolved[%d] = %q, want %q", i, auth.Unresolved[i], want[i])
		}
	}
}

// mxcli-formula1 #23 follow-up: literal credentials worked after the first fix,
// but a constant reference still produced a 401 and an empty client — and the
// same release made a constant `ServiceUrl` mandatory, so the shape mxcli
// insists on for the URL was the shape whose credentials it would not read.
//
// The quoted spelling was the sharp edge. `'@Module.ApiUser'` is a STRING_LITERAL,
// so the isLiteral flag says "literal" and the naive reading sent the fifteen
// characters `@Module.ApiUser` as the username: worse than a 401, because it
// looks like it tried.
func TestResolveCredential(t *testing.T) {
	consts := map[string]string{
		"m.apiuser": "f1api",
		"m.apipass": "s3cret",
		"m.empty":   "",
	}
	cases := []struct {
		name      string
		value     string
		isLiteral bool
		want      string
		wantOK    bool
	}{
		{"a literal is itself", "f1api", true, "f1api", true},
		{"a quoted constant reference resolves", "@M.ApiUser", true, "f1api", true},
		{"a bare constant reference resolves", "@M.ApiUser", false, "f1api", true},
		{"an unquoted qualified name is a reference too", "M.ApiPass", false, "s3cret", true},
		{"case-insensitive, as MDL is elsewhere", "@m.APIUSER", true, "f1api", true},
		// Unresolvable cases must report themselves rather than send something
		// that merely looks like a credential.
		{"an unknown constant is unresolved", "@M.Nope", true, "", false},
		{"a constant with no default is unresolved", "@M.Empty", true, "", false},
		{"an empty value is nothing", "", true, "", false},
		// A literal that happens to contain a dot is still a literal — passwords
		// contain dots, and that must not be read as a reference.
		{"a dotted literal stays a literal", "s3.cret", true, "s3.cret", true},
		// The value is a Mendix EXPRESSION. Studio Pro stores a literal
		// credential as the string literal `'MxAdmin'`, quotes included, and so
		// does MDL's `HttpUsername: '''MxAdmin'''`. The fetch must send its
		// content, not the quotes — sending `'MxAdmin'` is a 401 against the
		// very service the odata-data-sharing walkthrough imports from.
		{"a string-literal expression sends its content", "'MxAdmin'", true, "MxAdmin", true},
		{"a doubled quote inside it is one quote", "'it''s'", true, "it's", true},
		{"an empty string literal is an empty credential", "''", true, "", false},
		// A compound expression cannot be evaluated here. Sending its text
		// looks like it tried; reporting it unresolved says what happened.
		{"a compound expression is unresolved", "'Key ' + @M.ApiUser", true, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveCredential(tc.value, tc.isLiteral, consts)
			if ok != tc.wantOK {
				t.Fatalf("resolved = %v, want %v (value %q, literal %v)", ok, tc.wantOK, tc.value, tc.isLiteral)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The spelling the odata-data-sharing skill now teaches, end to end: parse the
// MDL, then build the credentials the design-time fetch will send. The stored
// value must be the expression `'MxAdmin'` (what Studio Pro stores) and the
// fetch must send `MxAdmin` — before the fix, fixing the skill traded a runtime
// defect for a 401 at design time.
func TestMetadataAuth_StringLiteralCredentialFromMDL(t *testing.T) {
	prog := parseMDL(t, `create odata client M.Api (
  ODataVersion: OData4,
  MetadataUrl: 'http://localhost:8080/odata/api/v1/$metadata',
  UseAuthentication: Yes,
  HttpUsername: '''MxAdmin''',
  HttpPassword: '''1'''
);`)
	stmt := prog.Statements[0].(*ast.CreateODataClientStmt)
	if stmt.HttpUsername != "'MxAdmin'" {
		t.Fatalf("stored HttpUsername = %q, want the string-literal expression %q", stmt.HttpUsername, "'MxAdmin'")
	}
	auth := metadataAuthFromStmt(&ExecContext{}, stmt)
	if auth.Username != "MxAdmin" || auth.Password != "1" {
		t.Errorf("fetch sends %q / %q, want MxAdmin / 1", auth.Username, auth.Password)
	}
	if len(auth.Unresolved) != 0 {
		t.Errorf("Unresolved = %v, want none", auth.Unresolved)
	}
}
