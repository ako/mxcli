// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCreateRestClient_Basic(t *testing.T) {
	input := `CREATE REST CLIENT MyModule.PetAPI (
		BaseUrl: 'https://api.example.com/v1'
	) {
		OPERATION GetPets {
			Method: GET,
			Path: '/pets',
			Response: JSON AS $PetList
		}
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("Expected 1 statement, got %d", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateRestClientStmt)
	if !ok {
		t.Fatalf("Expected CreateRestClientStmt, got %T", prog.Statements[0])
	}
	if stmt.BaseUrl != "https://api.example.com/v1" {
		t.Errorf("Got BaseUrl %q", stmt.BaseUrl)
	}
	if stmt.Authentication != nil {
		t.Error("Expected nil Authentication")
	}
	if len(stmt.Operations) != 1 {
		t.Fatalf("Expected 1 operation, got %d", len(stmt.Operations))
	}
	op := stmt.Operations[0]
	if op.Name != "GetPets" {
		t.Errorf("Got Name %q", op.Name)
	}
	if op.Method != "get" {
		t.Errorf("Got Method %q", op.Method)
	}
}

func TestCreatePublishedRestService(t *testing.T) {
	input := `CREATE PUBLISHED REST SERVICE MyModule.OrderAPI (
		Path: '/api/v1',
		Version: '1.0.0',
		ServiceName: 'Orders'
	) {
		RESOURCE 'orders' {
			GET '/{id}' MICROFLOW MyModule.GetOrder;
			POST '/' MICROFLOW MyModule.CreateOrder;
		}
	};`
	prog, errs := Build(input)
	if len(errs) > 0 {
		for _, e := range errs {
			t.Errorf("Parse error: %v", e)
		}
		return
	}
	stmt, ok := prog.Statements[0].(*ast.CreatePublishedRestServiceStmt)
	if !ok {
		t.Fatalf("Expected CreatePublishedRestServiceStmt, got %T", prog.Statements[0])
	}
	if stmt.Path != "/api/v1" {
		t.Errorf("Got Path %q", stmt.Path)
	}
	if stmt.ServiceName != "Orders" {
		t.Errorf("Got ServiceName %q", stmt.ServiceName)
	}
	if len(stmt.Resources) != 1 {
		t.Fatalf("Expected 1 resource, got %d", len(stmt.Resources))
	}
	if stmt.Resources[0].Name != "orders" {
		t.Errorf("Got resource name %q", stmt.Resources[0].Name)
	}
	if len(stmt.Resources[0].Operations) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(stmt.Resources[0].Operations))
	}
}

// TestCreatePublishedRestService_EndResourceSyntax_NoPanic verifies that the
// unsupported `end resource` keyword syntax does not cause a SIGSEGV (issue #429).
// ANTLR error recovery produces a PublishedRestResourceContext with a nil
// STRING_LITERAL token; the nil guard in buildPublishedRestResourceDef must
// prevent the panic and let Build return without crashing.
func TestCreatePublishedRestService_EndResourceSyntax_NoPanic(t *testing.T) {
	input := "create published rest service MyModule.TestREST (Version: '1.0', Path: '/api') resource 'items' get '/all' microflow MyModule.GetItems; end resource;"
	// Must not panic — parse errors are acceptable, a crash is not.
	prog, _ := Build(input)
	_ = prog
}

// TestCreatePublishedRestService_NonStringPropertyValue_NoPanic reproduces
// mendixlabs/mxcli#1331: a property value that is not a string literal
// (`Authentication: microflow M.F`, `Authentication: Basic`,
// `Folder: microflow M.F`) crashed the binary with a nil pointer dereference in
// unquoteStringLit. Build walks a failed parse on purpose, so the rule context
// exists with no STRING_LITERAL child; the author must get the syntax error.
func TestCreatePublishedRestService_NonStringPropertyValue_NoPanic(t *testing.T) {
	for name, value := range map[string]string{
		"microflow":      "Authentication: microflow MyFirstModule.AuthMf",
		"keyword":        "Authentication: Basic",
		"folder-non-str": "Folder: microflow MyFirstModule.AuthMf",
	} {
		t.Run(name, func(t *testing.T) {
			input := `CREATE OR MODIFY PUBLISHED REST SERVICE MyFirstModule.TestApi (
  Path: 'rest/test/v1',
  Version: '1.0.0',
  ServiceName: 'Test API',
  ` + value + `
)
{
  RESOURCE 'items' {
    GET '' MICROFLOW MyFirstModule.GetItems;
  }
};`
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic for %q: %v", value, r)
				}
			}()
			_, errs := Build(input)
			if len(errs) == 0 {
				t.Fatalf("expected a syntax error for %q, got none", value)
			}
		})
	}
}
