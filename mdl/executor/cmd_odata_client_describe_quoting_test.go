// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/model"
)

// DESCRIBE ODATA CLIENT must print each stored value so that executing the
// output stores the same value again. HttpUsername/HttpPassword/header values
// hold Mendix EXPRESSIONS, and Studio Pro stores a literal credential as the
// expression `'abc'` — quotes included. formatExprValue passed any value that
// already started and ended with a quote through unchanged, so describe printed
// `HttpUsername: 'abc'`, the visitor unquoted it, and re-executing stored `abc`:
// an identifier, not a string. Measured on a Studio Pro-authored client
// (ako/TestApp@37e0cc0, Odata.Bug1073). ClientCertificate, header keys and the
// plain string properties were printed as a raw '%s', unescaped.
//
// Since these properties became first-class expressions (§6.4 of
// PROPOSAL_first_class_expressions.md) describe prints the stored expression
// as-is and the visitor stores it as written, so `'abc'` round-trips as `'abc'`.
// The contract these tests hold — exec(describe(x)) stores x — is unchanged.

// describeAndReparse runs DESCRIBE on stored and parses the output with the real
// visitor, returning what a re-exec would store.
func describeAndReparse(t *testing.T, stored *model.ConsumedODataService, folder string) (*ast.CreateODataClientStmt, string) {
	t.Helper()
	var out bytes.Buffer
	ctx := &ExecContext{Output: &out}
	if err := outputConsumedODataServiceMDL(ctx, stored, "Odata", folder); err != nil {
		t.Fatalf("describe: %v", err)
	}
	prog := parseMDL(t, out.String())
	for _, s := range prog.Statements {
		if c, ok := s.(*ast.CreateODataClientStmt); ok {
			return c, out.String()
		}
	}
	t.Fatalf("describe output has no create odata client statement:\n%s", out.String())
	return nil, ""
}

// The reported shape, exactly as Studio Pro stores it.
func TestDescribeODataClient_LiteralCredentialSurvivesReExec(t *testing.T) {
	stored := &model.ConsumedODataService{
		Name:         "Bug1073",
		ODataVersion: "OData4",
		HttpConfiguration: &model.HttpConfiguration{
			UseAuthentication: true,
			Username:          "'abc'",
			// A constant reference already round-tripped: the control.
			Password: "@Clients.OrdersRestClient_password",
		},
	}
	got, out := describeAndReparse(t, stored, "")
	if got.HttpUsername != "'abc'" {
		t.Errorf("HttpUsername: stored %q, re-exec of describe output stores %q\n%s", "'abc'", got.HttpUsername, out)
	}
	if got.HttpPassword != stored.HttpConfiguration.Password {
		t.Errorf("HttpPassword (control): stored %q, re-exec stores %q\n%s", stored.HttpConfiguration.Password, got.HttpPassword, out)
	}
}

// Every other value the describer prints, with the characters that need escaping.
func TestDescribeODataClient_StoredValuesSurviveReExec(t *testing.T) {
	stored := &model.ConsumedODataService{
		Name:         "Bug1073",
		Version:      "1.0'b",
		ODataVersion: "OData4",
		MetadataUrl:  "file:///tmp/o'reilly/$metadata.xml",
		HttpConfiguration: &model.HttpConfiguration{
			OverrideLocation:  true,
			CustomLocation:    "@Odata.Bug1073_Location",
			ClientCertificate: "'my-cert'",
			HeaderEntries: []*model.HttpHeaderEntry{
				{Key: "X-Api-Key", Value: "'Key ' + @Odata.ApiKey"},
				{Key: "X-O'Key", Value: "'it''s'"},
				{Key: "Accept", Value: `'a\b'`},
				// Written by an older mxcli from `'abc'`: not a valid string
				// expression, but describe must reproduce it, not repair it.
				{Key: "X-Legacy", Value: "abc"},
			},
		},
	}
	got, out := describeAndReparse(t, stored, "Api/O'Clients")

	cfg := stored.HttpConfiguration
	// ServiceUrl names a constant: describe prints the bare name and exec adds
	// the @ back (serviceURLConstant), so compare what exec would store.
	location, err := serviceURLConstant(got.ServiceUrl)
	if err != nil {
		t.Fatalf("describe printed a ServiceUrl exec refuses: %v\n%s", err, out)
	}
	for _, c := range []struct{ field, want, got string }{
		{"Version", stored.Version, got.Version},
		{"MetadataUrl", stored.MetadataUrl, got.MetadataUrl},
		{"Folder", "Api/O'Clients", got.Folder},
		{"ServiceUrl", cfg.CustomLocation, location},
		{"ClientCertificate", cfg.ClientCertificate, got.ClientCertificate},
	} {
		if c.got != c.want {
			t.Errorf("%s: stored %q, re-exec of describe output stores %q\n%s", c.field, c.want, c.got, out)
		}
	}

	if len(got.Headers) != len(cfg.HeaderEntries) {
		t.Fatalf("headers: stored %d, re-exec stores %d\n%s", len(cfg.HeaderEntries), len(got.Headers), out)
	}
	for i, h := range cfg.HeaderEntries {
		if got.Headers[i].Key != h.Key || got.Headers[i].Value != h.Value {
			t.Errorf("header %d: stored %q: %q, re-exec stores %q: %q",
				i, h.Key, h.Value, got.Headers[i].Key, got.Headers[i].Value)
		}
	}
}
