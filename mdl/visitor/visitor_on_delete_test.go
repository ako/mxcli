// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// CapTrackV2 FINDINGS §1 — `DELETE_BEHAVIOR PREVENT` wrote an app that will not
// start:
//
//	ERROR - M2EE: An error occurred while initializing the Runtime: None.get
//	java.util.NoSuchElementException: None.get
//	  at …SchemeFactory$.…$setDeleteBehavior(SchemeFactory.scala:515)
//
// mxcli wrote ChildDeleteBehavior "DeleteMeIfNoReferences" with a null
// ChildErrorMessage, and MDL had no way to say the message. A Studio Pro
// reference (ako/TestApp, Mappings.Order_Customer) shows what the modeller
// writes instead — an ordinary Texts$Text:
//
//	"ChildDeleteBehavior": "DeleteMeIfNoReferences",
//	"ChildErrorMessage": { "$Type": "Texts$Text",
//	                       "Items": [ 3, { "$Type": "Texts$Translation",
//	                                       "LanguageCode": "en_US",
//	                                       "Text": "Aha here it is" } ] }
//
// The syntax is SQL's, because Mendix's three behaviours ARE SQL's referential
// actions and MDL's FROM/TO already matches a foreign key's direction (measured
// on that same reference: ParentPointer -> Order, the FK owner and the FROM;
// ChildPointer -> Customer, referenced and the TO). So `ON DELETE RESTRICT`
// reads the way it does in CREATE TABLE, and needs no knowledge of which side
// Mendix calls the child — which is the complaint the old spelling earned.

func parseAssoc(t *testing.T, src string) *ast.CreateAssociationStmt {
	t.Helper()
	prog, errs := Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse errors for %q: %v", src, errs)
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("got %d statements, want 1", len(prog.Statements))
	}
	stmt, ok := prog.Statements[0].(*ast.CreateAssociationStmt)
	if !ok {
		t.Fatalf("got %T, want *ast.CreateAssociationStmt", prog.Statements[0])
	}
	return stmt
}

// Each SQL action maps onto exactly one Mendix behaviour.
func TestOnDelete_MapsTheSQLReferentialActions(t *testing.T) {
	cases := []struct {
		clause string
		want   ast.DeleteBehavior
	}{
		{"ON DELETE CASCADE", ast.DeleteCascade},
		{"ON DELETE RESTRICT", ast.DeleteIfNoReferences},
		{"ON DELETE SET NULL", ast.DeleteKeepReferences},
	}
	for _, c := range cases {
		stmt := parseAssoc(t, "CREATE ASSOCIATION Shop.Order_Customer FROM Shop.Order TO Shop.Customer "+c.clause+";")
		if stmt.DeleteBehavior != c.want {
			t.Errorf("%q gave %v, want %v", c.clause, stmt.DeleteBehavior, c.want)
		}
	}
}

// The message the runtime needs, and the thing MDL could not say at all.
func TestOnDelete_CarriesTheErrorMessage(t *testing.T) {
	stmt := parseAssoc(t, "CREATE ASSOCIATION Shop.Order_Customer FROM Shop.Order TO Shop.Customer "+
		"ON DELETE RESTRICT ERROR_MESSAGE 'A customer with orders cannot be deleted';")

	if stmt.DeleteBehavior != ast.DeleteIfNoReferences {
		t.Fatalf("behaviour = %v, want DeleteIfNoReferences", stmt.DeleteBehavior)
	}
	if stmt.DeleteErrorMessage != "A customer with orders cannot be deleted" {
		t.Errorf("message = %q, want the authored text", stmt.DeleteErrorMessage)
	}
}

// CONTROL: the older spelling still parses and still means the same thing. It is
// in every script written before this, and breaking it to improve readability
// would be a poor trade.
func TestOnDelete_LegacyDeleteBehaviorStillWorks(t *testing.T) {
	for _, src := range []string{
		"CREATE ASSOCIATION Shop.A_B FROM Shop.A TO Shop.B DELETE_BEHAVIOR PREVENT;",
		"CREATE ASSOCIATION Shop.A_B FROM Shop.A TO Shop.B DELETE_BEHAVIOR CASCADE;",
		"CREATE ASSOCIATION Shop.A_B FROM Shop.A TO Shop.B DELETE_BEHAVIOR DELETE_IF_NO_REFERENCES;",
	} {
		parseAssoc(t, src) // a parse error fails the helper
	}
	stmt := parseAssoc(t, "CREATE ASSOCIATION Shop.A_B FROM Shop.A TO Shop.B DELETE_BEHAVIOR PREVENT;")
	if stmt.DeleteBehavior != ast.DeleteIfNoReferences {
		t.Errorf("PREVENT = %v, want DeleteIfNoReferences", stmt.DeleteBehavior)
	}
}

// …and it can carry the message too, so an existing script gains the fix without
// being rewritten to the new spelling.
func TestOnDelete_LegacySpellingTakesTheMessage(t *testing.T) {
	stmt := parseAssoc(t, "CREATE ASSOCIATION Shop.A_B FROM Shop.A TO Shop.B "+
		"DELETE_BEHAVIOR PREVENT ERROR_MESSAGE 'still referenced';")
	if stmt.DeleteErrorMessage != "still referenced" {
		t.Errorf("message = %q", stmt.DeleteErrorMessage)
	}
}

// CONTROL: no clause at all is still the default, with no message.
func TestOnDelete_OmittedIsTheDefault(t *testing.T) {
	stmt := parseAssoc(t, "CREATE ASSOCIATION Shop.A_B FROM Shop.A TO Shop.B;")
	if stmt.DeleteBehavior != ast.DeleteKeepReferences {
		t.Errorf("behaviour = %v, want the keep-references default", stmt.DeleteBehavior)
	}
	if stmt.DeleteErrorMessage != "" {
		t.Errorf("message = %q, want empty", stmt.DeleteErrorMessage)
	}
}

// A quoted message containing a quote must survive, doubled the way MDL spells
// escapes everywhere else.
func TestOnDelete_MessageWithAQuote(t *testing.T) {
	stmt := parseAssoc(t, "CREATE ASSOCIATION Shop.A_B FROM Shop.A TO Shop.B "+
		"ON DELETE RESTRICT ERROR_MESSAGE 'the customer''s orders block this';")
	if !strings.Contains(stmt.DeleteErrorMessage, "customer's orders") {
		t.Errorf("message = %q, want the apostrophe unescaped", stmt.DeleteErrorMessage)
	}
}
