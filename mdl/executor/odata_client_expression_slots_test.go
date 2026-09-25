// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// PROPOSAL_first_class_expressions.md §6.4, decided: HttpUsername, HttpPassword,
// ClientCertificate and header values hold a Mendix EXPRESSION, and MDL writes
// that expression as-is. `HttpUsername: 'admin'` is the string 'admin' — the
// value Studio Pro stores, quotes included — where it used to store the
// identifier `admin` and a string needed `'''admin'''`.

func odataClientFrom(t *testing.T, src string) *ast.CreateODataClientStmt {
	t.Helper()
	for _, s := range parseMDL(t, src).Statements {
		if c, ok := s.(*ast.CreateODataClientStmt); ok {
			return c
		}
	}
	t.Fatalf("no create odata client statement in %q", src)
	return nil
}

func TestODataClientExpressionSlots_StoreTheExpressionAsWritten(t *testing.T) {
	stmt := odataClientFrom(t, `create odata client M.Api (
  ODataVersion: OData4,
  MetadataUrl: 'https://example.com/$metadata',
  UseAuthentication: Yes,
  HttpUsername: 'admin',
  HttpPassword: @M.ApiPassword,
  ClientCertificate: 'it''s'
)
headers (
  'X-Api-Key': 'Key ' + @M.ApiKey,
  'Accept': 'application/json'
);`)

	for _, c := range []struct{ field, got, want string }{
		{"HttpUsername", stmt.HttpUsername, "'admin'"},
		{"HttpPassword", stmt.HttpPassword, "@M.ApiPassword"},
		{"ClientCertificate", stmt.ClientCertificate, "'it''s'"},
	} {
		if c.got != c.want {
			t.Errorf("%s stored %q, want the expression %q", c.field, c.got, c.want)
		}
	}
	want := map[string]string{"X-Api-Key": "'Key ' + @M.ApiKey", "Accept": "'application/json'"}
	for _, h := range stmt.Headers {
		if h.Value != want[h.Key] {
			t.Errorf("header %s stored %q, want %q", h.Key, h.Value, want[h.Key])
		}
	}
	// Control: a plain-value property keeps its unquoted value.
	if stmt.MetadataUrl != "https://example.com/$metadata" {
		t.Errorf("MetadataUrl = %q, want the unquoted URL", stmt.MetadataUrl)
	}
}

func TestODataClientExpressionSlots_AlterStoresTheExpression(t *testing.T) {
	prog := parseMDL(t, `alter odata client M.Api set HttpUsername = 'admin', HttpPassword = 'a' + @M.C;`)
	stmt := prog.Statements[0].(*ast.AlterODataClientStmt)
	if got := stmt.Changes["HttpUsername"]; got != "'admin'" {
		t.Errorf("HttpUsername set to %q, want %q", got, "'admin'")
	}
	if got := stmt.Changes["HttpPassword"]; got != "'a' + @M.C" {
		t.Errorf("HttpPassword set to %q, want %q", got, "'a' + @M.C")
	}
}

// A compound expression in a property that takes a plain value must be refused,
// not read as an empty value: widening the value rule for the four slots must
// not open a silent drop everywhere else.
func TestODataExpressionInAPlainProperty_IsAnError(t *testing.T) {
	for _, src := range []string{
		`create odata client M.Api (ODataVersion: OData4, MetadataUrl: 'https://x/' + '$metadata');`,
		`create odata service M.S (Path: 'odata/' + 'v1', Version: '1.0') {};`,
		`alter odata client M.Api set MetadataUrl = 'a' + 'b';`,
	} {
		_, errs := visitor.Build(src)
		if len(errs) == 0 {
			t.Errorf("accepted an expression in a plain-value property: %s", src)
			continue
		}
		if !strings.Contains(errs[0].Error(), "expression") {
			t.Errorf("error %q should say the property does not take an expression", errs[0])
		}
	}
}

// MDL-ODATA07: the two spellings whose meaning changed. Both stay parseable, and
// both would now silently store something else, so check refuses them and names
// the new spelling.
func TestMDLODATA07_LegacyCredentialSpelling(t *testing.T) {
	cases := []struct {
		name, value string
		want        int
	}{
		{"doubled quotes, the old string spelling", `'''admin'''`, 1},
		{"quoted constant reference", `'@M.ApiUser'`, 1},
		{"control: a string", `'admin'`, 0},
		{"control: a constant reference", `@M.ApiUser`, 0},
		{"control: a compound expression", `'Bearer ' + @M.Token`, 0},
		// A literal that merely starts with @ is a legitimate string.
		{"control: an @ that is not a qualified name", `'@home'`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := parseMDL(t, `create odata client M.Api (
  ODataVersion: OData4, MetadataUrl: 'https://x/$metadata',
  UseAuthentication: Yes, HttpUsername: `+tc.value+`
)
headers ('X-Token': `+tc.value+`);`)
			var got []string
			for _, v := range ValidateODataProperties(prog) {
				if v.RuleID == "MDL-ODATA07" {
					got = append(got, v.Message)
				}
			}
			// One for HttpUsername, one for the header.
			if len(got) != 2*tc.want {
				t.Fatalf("MDL-ODATA07: got %d, want %d: %v", len(got), 2*tc.want, got)
			}
		})
	}
}
