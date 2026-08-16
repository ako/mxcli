// SPDX-License-Identifier: Apache-2.0

// A published OData service can answer GraphQL as well — one boolean, the same
// resources, and the SAME endpoint (clients POST a query to the service
// location rather than GET a resource path).
//
// Verified against a running Mendix 11.13 app rather than only against the
// build, because "the model stores a flag" and "the app answers GraphQL" are
// different claims:
//
//	POST /odata/charts/  { __schema { queryType { name } } }
//	  -> {"data": {"__schema": {"queryType": {"name": "Query"}}}}
//	POST /odata/charts/  { monthCategories { period category total } }
//	  -> {"data":{"monthCategories":[]}}
package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

func parseService(t *testing.T, src string) *ast.CreateODataServiceStmt {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateODataServiceStmt)
	if !ok {
		t.Fatalf("statement is %T, want CreateODataServiceStmt", prog.Statements[0])
	}
	return stmt
}

const gqlService = `create odata service M.Api (
  Path: 'odata/charts/', ServiceName: 'Api', Namespace: 'M.Charts',
  Version: '1.0.0', ODataVersion: OData4, SupportsGraphQL: %s
) { publish entity M.Row as 'Rows' expose ( K (KEY) ) }
/`

// TestSupportsGraphQLParses — the property used to be rejected outright by
// MDL-ODATA01 as unknown, which is the whole reason a project could not enable
// GraphQL from a script.
func TestSupportsGraphQLParses(t *testing.T) {
	for _, tc := range []struct {
		literal string
		want    bool
	}{
		{"Yes", true}, {"true", true}, {"No", false}, {"false", false},
	} {
		stmt := parseService(t, strings.Replace(gqlService, "%s", tc.literal, 1))
		if stmt.SupportsGraphQL != tc.want {
			t.Errorf("SupportsGraphQL: %s parsed as %v, want %v", tc.literal, stmt.SupportsGraphQL, tc.want)
		}
		if !stmt.SupportsGraphQLSet {
			t.Errorf("SupportsGraphQL: %s did not record that the author said anything", tc.literal)
		}
		if len(stmt.UnknownProperties) > 0 {
			t.Errorf("SupportsGraphQL landed in UnknownProperties: %v", stmt.UnknownProperties)
		}
	}
}

// TestSupportsGraphQLUnsetIsNotSet — an omitted value must leave Set false, so
// `create or modify` does not turn GraphQL OFF on a service that has it on
// merely by not mentioning it. There is no useful default to infer here, unlike
// PublishAssociations: false is what every service was before the property
// existed.
func TestSupportsGraphQLUnsetIsNotSet(t *testing.T) {
	stmt := parseService(t, `create odata service M.Api (
  Path: 'odata/charts/', ServiceName: 'Api', Namespace: 'M.Charts', Version: '1.0.0'
) { publish entity M.Row as 'Rows' expose ( K (KEY) ) }
/`)
	if stmt.SupportsGraphQLSet {
		t.Error("an omitted SupportsGraphQL was recorded as set")
	}
	if stmt.SupportsGraphQL {
		t.Error("an omitted SupportsGraphQL defaulted to true")
	}
}

// TestSupportsGraphQLIsAKnownProperty guards the surface that rejected it:
// unknown OData properties are an error (MDL-ODATA01), so a name missing from
// the list is not silently dropped — it fails the check.
func TestSupportsGraphQLIsAKnownProperty(t *testing.T) {
	var found bool
	for _, p := range knownODataServiceProps {
		if p == "SupportsGraphQL" {
			found = true
		}
	}
	if !found {
		t.Errorf("SupportsGraphQL missing from knownODataServiceProps: %v", knownODataServiceProps)
	}
}

// TestGraphQLRefusesObjectIdAssociations — Mendix rejects the pair outright:
// CE8055 "A service that supports GraphQL must publish associations as a link."
// GraphQL has no representation for an associated object id.
//
// Refused at execute rather than warned about, because unlike
// PublishAssociations: No on its own — a legitimate mode for a service whose
// key is arranged in Studio Pro — this combination can never build, whatever
// else the author does. Measured on 11.13 against a real OQL view entity: the
// same service is 0 errors with Yes, and CE7375 + CE8055 with No.
func TestGraphQLRefusesObjectIdAssociations(t *testing.T) {
	stmt := parseService(t, `create odata service M.Api (
  Path: 'odata/charts/', ServiceName: 'Api', Namespace: 'M.Charts',
  Version: '1.0.0', ODataVersion: OData4,
  PublishAssociations: No, SupportsGraphQL: Yes
) { publish entity M.Row as 'Rows' expose ( K (KEY) ) }
/`)
	if !stmt.SupportsGraphQL || stmt.PublishAssociations || !stmt.PublishAssociationsSet {
		t.Fatalf("fixture did not parse as GraphQL+object-id: graphql=%v assoc=%v set=%v",
			stmt.SupportsGraphQL, stmt.PublishAssociations, stmt.PublishAssociationsSet)
	}
	// The executor refuses this pair before writing; see createODataService.
	// Pinned here so the fixture that triggers it cannot drift silently.
}
