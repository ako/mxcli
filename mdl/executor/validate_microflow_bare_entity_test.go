// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// upstream #973: "An unassigned CREATE statement writes a create activity
// without an Entity, silently."
//
// The reported trigger does not reproduce — the reporter's script, run verbatim
// on both engines, stores `Entity: "Repro2.Thing"` and mx check is clean, so
// omitting `$X =` is not what breaks it. (One thing it did expose: describe
// prints `$NewObject = create …` for an activity whose output variable is empty,
// a name invented at format time, which is why an empty variable looks bound
// when it is read back.)
//
// What DOES reproduce is an entity reference with no module prefix, and it is
// worse than the reported CE6005 — measured on mxbuild 11.13.0:
//
//	create Thing (Name = 'Damaged') COMMIT;   -- unqualified
//
//	$ mxcli check --references
//	✓ All references valid — Check passed!
//
//	$ mx check
//	ERROR: Mendix.Modeler.Storage.StorageLoadException: One or more invalid
//	  values were detected while loading the project:
//	   - Change in has an invalid value '' for property Attribute.
//	     The text 'Name' is not a valid AttributeIdentifier.
//
// The activity has no Entity key at all, so the member assignments cannot be
// qualified and go to disk as the bare word `Name`. A `retrieve` is the same
// shape from the other side: it stores `Entity: ".Thing"`.
//
// The hole was in the reference collector, which skipped any entity reference
// whose Module was empty — so an unqualified name was never validated and was
// then written as nothing. MDL008 already said this for microflow PARAMETERS;
// it now says it for the body too, and being a static property of the statement
// it fires with no project at hand.
//
// Control: the same statement written qualified-but-wrong IS caught, and always
// was — `entity not found: Repro2.NoSuchEntity (referenced by create)`. It is the
// absent module prefix, not the absent entity, that opened the hole.

func bareEntityViolations(t *testing.T, body ...ast.MicroflowStatement) []linter.Violation {
	t.Helper()
	return ValidateMicroflow(&ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "U", Name: "MF"},
		Body: body,
	})
}

func firstViolationWithRule(vs []linter.Violation, ruleID string) *linter.Violation {
	for i := range vs {
		if vs[i].RuleID == ruleID {
			return &vs[i]
		}
	}
	return nil
}

func TestMicroflowBody_RefusesABareEntityReference(t *testing.T) {
	cases := []struct {
		name string
		stmt ast.MicroflowStatement
		want string
	}{
		{
			"create",
			&ast.CreateObjectStmt{
				Variable:   "New",
				EntityType: ast.QualifiedName{Name: "Thing"},
			},
			"Thing",
		},
		{
			"retrieve",
			&ast.RetrieveStmt{
				Variable: "Items",
				Source:   ast.QualifiedName{Name: "Thing"},
			},
			"Thing",
		},
		{
			"create list of",
			&ast.CreateListStmt{
				Variable:   "Items",
				EntityType: ast.QualifiedName{Name: "Thing"},
			},
			"Thing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := firstViolationWithRule(bareEntityViolations(t, tc.stmt), "MDL008")
			if v == nil {
				t.Fatalf("a bare entity reference must be reported: it is written as nothing "+
					"and the .mpr will not load. statement: %T", tc.stmt)
			}
			if v.Severity != linter.SeverityError {
				t.Errorf("severity = %v, want error", v.Severity)
			}
			if !strings.Contains(v.Message, tc.want) {
				t.Errorf("message should name %q, got: %s", tc.want, v.Message)
			}
		})
	}
}

// The controls. A qualified reference is the normal case and must stay silent —
// including one naming an entity that does not exist, which is the reference
// check's business and needs a project this rule does not have.
func TestMicroflowBody_AcceptsAQualifiedEntityReference(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.CreateObjectStmt{Variable: "New", EntityType: ast.QualifiedName{Module: "U", Name: "Thing"}},
		&ast.RetrieveStmt{Variable: "Items", Source: ast.QualifiedName{Module: "U", Name: "Thing"}},
		&ast.CreateListStmt{Variable: "L", EntityType: ast.QualifiedName{Module: "System", Name: "Image"}},
		&ast.RetrieveStmt{Variable: "Ghost", Source: ast.QualifiedName{Module: "U", Name: "NoSuchEntity"}},
	}
	if v := firstViolationWithRule(bareEntityViolations(t, body...), "MDL008"); v != nil {
		t.Errorf("a qualified reference must not be flagged, got: %s", v.Message)
	}
}

// An association retrieve names an ASSOCIATION, not an entity, and its source is
// unqualified by construction in the `retrieve $x from $obj/Assoc` form. Flagging
// it would reject valid MDL.
func TestMicroflowBody_IgnoresAnAssociationRetrieve(t *testing.T) {
	body := []ast.MicroflowStatement{
		&ast.RetrieveStmt{
			Variable:      "Lines",
			StartVariable: "Order",
			Source:        ast.QualifiedName{Name: "Order_Line"},
		},
	}
	if v := firstViolationWithRule(bareEntityViolations(t, body...), "MDL008"); v != nil {
		t.Errorf("an association retrieve must not be flagged as a bare entity, got: %s", v.Message)
	}
}

// The rule already covered parameters; that must keep working.
func TestMicroflowParameters_StillRefuseABareEntityType(t *testing.T) {
	entity := ast.QualifiedName{Name: "Thing"}
	vs := ValidateMicroflow(&ast.CreateMicroflowStmt{
		Name: ast.QualifiedName{Module: "U", Name: "MF"},
		Parameters: []ast.MicroflowParam{
			{Name: "T", Type: ast.DataType{Kind: ast.TypeEntity, EntityRef: &entity}},
		},
	})
	if firstViolationWithRule(vs, "MDL008") == nil {
		t.Error("a bare entity type on a parameter must still be reported")
	}
}
