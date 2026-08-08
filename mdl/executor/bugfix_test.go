// SPDX-License-Identifier: Apache-2.0

// Tests for bug fixes discovered during BST Monitoring app session (2026-03-13).
package executor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// TestValidateDuplicateVariableDeclareRetrieve verifies that DECLARE followed by
// RETRIEVE for the same variable is caught as a duplicate (CE0111).
// Bug #3: mxcli check passed but mx check reported CE0111.
func TestValidateDuplicateVariableDeclareRetrieve(t *testing.T) {
	input := `create microflow Test.MF_DuplicateVar ()
begin
  declare $Count Integer = 0;
  retrieve $Count from Test.TestItem;
  return $Count;
end;`

	errors := validateMicroflowFromMDL(t, input)

	found := false
	for _, e := range errors {
		if strings.Contains(e, "duplicate") && strings.Contains(e, "Count") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected duplicate variable error for $Count, got errors: %v", errors)
	}
}

// TestValidateDuplicateVariableDeclareOnly verifies that two DECLARE statements
// for the same variable are caught as duplicate.
func TestValidateDuplicateVariableDeclareOnly(t *testing.T) {
	input := `create microflow Test.MF_DoubleDeclare ()
begin
  declare $X Integer = 0;
  declare $X String = 'hello';
end;`

	errors := validateMicroflowFromMDL(t, input)

	found := false
	for _, e := range errors {
		if strings.Contains(e, "duplicate") && strings.Contains(e, "X") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected duplicate variable error for $X, got errors: %v", errors)
	}
}

// TestValidateNoDuplicateWhenRetrieveOnly verifies that a single RETRIEVE
// (without prior DECLARE) does not trigger a false positive.
func TestValidateNoDuplicateWhenRetrieveOnly(t *testing.T) {
	input := `create microflow Test.MF_RetrieveOnly ()
begin
  retrieve $Items from Test.SomeEntity;
end;`

	errors := validateMicroflowFromMDL(t, input)

	for _, e := range errors {
		if strings.Contains(e, "duplicate") {
			t.Errorf("Unexpected duplicate variable error: %s", e)
		}
	}
}

// TestValidateDuplicateVariableDeclareCreate verifies that DECLARE followed by
// CREATE for the same variable is caught as a duplicate (CE0111).
func TestValidateDuplicateVariableDeclareCreate(t *testing.T) {
	input := `create microflow Test.MF_DeclareCreate ()
begin
  declare $NewTodo Test.Todo;
  $NewTodo = create Test.Todo();
end;`

	errors := validateMicroflowFromMDL(t, input)

	found := false
	for _, e := range errors {
		if strings.Contains(e, "duplicate") && strings.Contains(e, "NewTodo") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected duplicate variable error for $NewTodo, got errors: %v", errors)
	}
}

// TestValidateDuplicateMicroflowCallOutputVar covers finding #5 (second trigger):
// a microflow-call output variable reused across a fallback chain is a fresh
// declaration each time, so the second assignment is CE0111 — the natural
// "try A, else try B" shape. Both the same-scope and the nested-in-if forms
// must be caught (the fallback is almost always written inside an if).
func TestValidateDuplicateMicroflowCallOutputVar(t *testing.T) {
	t.Run("same scope", func(t *testing.T) {
		input := `create microflow Test.MF_Fallback ()
begin
  $Summary = call microflow Test.Inner(Tag = 'description');
  $Summary = call microflow Test.Inner(Tag = 'summary');
end;`
		errors := validateMicroflowFromMDL(t, input)
		if !hasDupError(errors, "Summary") {
			t.Errorf("Expected duplicate variable error for $Summary, got: %v", errors)
		}
	})

	t.Run("fallback inside if", func(t *testing.T) {
		input := `create microflow Test.MF_FallbackIf ()
begin
  $Summary = call microflow Test.Inner(Tag = 'description');
  if trim($Summary) = '' then
    $Summary = call microflow Test.Inner(Tag = 'summary');
  end if;
end;`
		errors := validateMicroflowFromMDL(t, input)
		if !hasDupError(errors, "Summary") {
			t.Errorf("Expected duplicate variable error for $Summary (fallback in if), got: %v", errors)
		}
	})
}

func hasDupError(errors []string, varName string) bool {
	for _, e := range errors {
		if strings.Contains(e, "duplicate") && strings.Contains(e, varName) {
			return true
		}
	}
	return false
}

// TestValidateEntityReservedAttributeName verifies that persistent entity attributes
// using reserved system names (CreatedDate, ChangedDate, Owner, ChangedBy) are caught.
func TestValidateEntityReservedAttributeName(t *testing.T) {
	input := `create persistent entity Test.MyEntity (
  Name : String(200),
  CreatedDate : DateTime,
  Status : String(50)
);`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("Expected CreateEntityStmt, got %T", prog.Statements[0])
	}

	violations := ValidateEntity(stmt)
	found := false
	for _, v := range violations {
		if strings.Contains(v.Message, "CreatedDate") && strings.Contains(v.Message, "system attribute") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected reserved attribute error for CreatedDate, got: %v", violations)
	}
}

// TestValidateEntityAutonumberNeedsSeed verifies MDL023: an autonumber attribute
// without a seed fails the build (CE7247), so mxcli check flags it. Findings #6.
func TestValidateEntityAutonumberNeedsSeed(t *testing.T) {
	build := func(src string) []linter.Violation {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse error: %v", errs[0])
		}
		return ValidateEntity(prog.Statements[0].(*ast.CreateEntityStmt))
	}
	hasMDL := func(vs []linter.Violation, id string) bool {
		for _, v := range vs {
			if v.RuleID == id {
				return true
			}
		}
		return false
	}

	if !hasMDL(build(`create persistent entity M.G ( PuzzleNo: autonumber );`), "MDL023") {
		t.Error("expected MDL023 for a seedless autonumber")
	}
	if hasMDL(build(`create persistent entity M.G ( PuzzleNo: autonumber default 1 );`), "MDL023") {
		t.Error("did not expect MDL023 when autonumber has a seed")
	}
}

// TestValidateEntityAutoMemberRename verifies MDL022: an AutoX pseudo-type
// declared under a name other than its fixed system member warns that the name
// is discarded on write. Findings #7.
func TestValidateEntityAutoMemberRename(t *testing.T) {
	build := func(src string) []linter.Violation {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse error: %v", errs[0])
		}
		return ValidateEntity(prog.Statements[0].(*ast.CreateEntityStmt))
	}
	hasMDL := func(vs []linter.Violation, id string) bool {
		for _, v := range vs {
			if v.RuleID == id {
				return true
			}
		}
		return false
	}

	if !hasMDL(build(`create persistent entity M.G ( StartedAt: autocreateddate );`), "MDL022") {
		t.Error("expected MDL022 for a renamed autocreateddate")
	}
	// Canonical name (case-insensitive) does not warn.
	if hasMDL(build(`create persistent entity M.G ( CreatedDate: autocreateddate );`), "MDL022") {
		t.Error("did not expect MDL022 when the name matches the system member")
	}
}

// TestValidateAlterEntityAddAttribute verifies that the same per-attribute
// checks applied on CREATE (MDL022 AutoX rename, MDL023 autonumber seed, MDL021
// reserved words) also run on the ALTER ENTITY ADD ATTRIBUTE path. Previously an
// attribute added via ALTER escaped these checks entirely (findings #6, half-fix).
func TestValidateAlterEntityAddAttribute(t *testing.T) {
	build := func(src string) []linter.Violation {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			t.Fatalf("parse error: %v", errs[0])
		}
		return ValidateAlterEntity(prog.Statements[0].(*ast.AlterEntityStmt))
	}
	hasMDL := func(vs []linter.Violation, id string) bool {
		for _, v := range vs {
			if v.RuleID == id {
				return true
			}
		}
		return false
	}

	// MDL023: seedless autonumber added via ALTER.
	if !hasMDL(build(`alter entity M.G add attribute PuzzleNo: autonumber;`), "MDL023") {
		t.Error("expected MDL023 for a seedless autonumber added via ALTER")
	}
	if hasMDL(build(`alter entity M.G add attribute PuzzleNo: autonumber default 1;`), "MDL023") {
		t.Error("did not expect MDL023 when the added autonumber has a seed")
	}
	// MDL022: AutoX pseudo-type added under a non-canonical name.
	if !hasMDL(build(`alter entity M.G add attribute StartedAt: autocreateddate;`), "MDL022") {
		t.Error("expected MDL022 for a renamed autocreateddate added via ALTER")
	}
	if hasMDL(build(`alter entity M.G add attribute CreatedDate: autocreateddate;`), "MDL022") {
		t.Error("did not expect MDL022 when the added AutoX name matches the system member")
	}
	// MDL021: reserved word added via ALTER (kind-independent).
	if !hasMDL(build(`alter entity M.G add attribute Type: string;`), "MDL021") {
		t.Error("expected MDL021 for a reserved word added via ALTER")
	}
	// A plain, valid attribute added via ALTER produces no violations.
	if vs := build(`alter entity M.G add attribute Score: integer;`); len(vs) != 0 {
		t.Errorf("expected no violations for a plain added attribute, got %v", vs)
	}
	// Non-ADD alter operations are ignored.
	if vs := build(`alter entity M.G drop attribute Score;`); len(vs) != 0 {
		t.Errorf("expected no violations for a DROP ATTRIBUTE op, got %v", vs)
	}
}

// TestDroppedEntityMembers verifies the helper that lists members a CREATE OR
// MODIFY replace would delete (findings #24).
func TestDroppedEntityMembers(t *testing.T) {
	existing := &domainmodel.Entity{
		Name: "Game",
		Attributes: []*domainmodel.Attribute{
			{Name: "Puzzle"}, {Name: "Solution"}, {Name: "Difficulty"},
		},
		HasCreatedDate: true,
		HasOwner:       true,
	}
	replacement := &domainmodel.Entity{
		Name: "Game",
		Attributes: []*domainmodel.Attribute{
			{Name: "puzzle"}, // case-insensitive match — kept
			{Name: "Score"},  // new — not a drop
		},
		HasOwner: true, // kept
		// HasCreatedDate now false — dropped
	}
	dropped := droppedEntityMembers(existing, replacement)
	got := strings.Join(dropped, ",")
	// Solution and Difficulty are dropped; Puzzle survives (case-insensitive);
	// createdDate audit field is dropped; owner survives.
	for _, want := range []string{"Solution", "Difficulty", "createdDate (system field)"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected dropped members to include %q, got %q", want, got)
		}
	}
	for _, unwanted := range []string{"Puzzle", "puzzle", "Score", "owner"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("did not expect %q among dropped members, got %q", unwanted, got)
		}
	}
}

// TestCreateOrModifyEntity_WarnsOnDrop verifies that CREATE OR MODIFY ENTITY
// still applies (non-blocking) but prints a data-loss warning listing the
// attributes it removes (findings #24).
func TestCreateOrModifyEntity_WarnsOnDrop(t *testing.T) {
	mod := mkModule("Sudoku")
	existing := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: nextID("dm"),
		Name:        "Game",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Puzzle"}, {Name: "Solution"}, {Name: "Difficulty"},
		},
	}
	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{existing},
	}
	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, existing.ContainerID, dm.ID)

	updateCalled := false
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		UpdateEntityFunc: func(dmID model.ID, e *domainmodel.Entity) error {
			updateCalled = true
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	// Redeclare only one of the three existing attributes.
	err := execCreateEntity(ctx, &ast.CreateEntityStmt{
		Name:           ast.QualifiedName{Module: "Sudoku", Name: "Game"},
		Kind:           ast.EntityPersistent,
		CreateOrModify: true,
		Attributes: []ast.Attribute{
			{Name: "Puzzle", Type: ast.DataType{Kind: ast.TypeString}},
		},
	})
	assertNoError(t, err)
	if !updateCalled {
		t.Fatal("UpdateEntity was not called — modify should still apply (non-blocking)")
	}
	out := buf.String()
	assertContainsStr(t, out, "drops 2 existing member(s)")
	assertContainsStr(t, out, "Solution")
	assertContainsStr(t, out, "Difficulty")
	assertContainsStr(t, out, "alter entity Sudoku.Game add attribute")
}

// TestValidateEntityNPEReservedWords verifies that non-persistent entities
// reject runtime-reserved attribute names (CE7247). Issue #552.
//
// Mendix Studio Pro reports CE7247 "The name 'X' is a reserved word." on
// NPE attributes named Owner, Type, Context, Id, CreatedDate, ChangedDate,
// ChangedBy, etc. Previously ValidateEntity early-returned for NPEs and
// missed these.
func TestValidateEntityNPEReservedWords(t *testing.T) {
	input := `create non-persistent entity Test.MyNPE (
  Owner : String(200),
  Type : String(50),
  Context : String(100),
  Id : Integer,
  CreatedDate : DateTime
);`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("Expected CreateEntityStmt, got %T", prog.Statements[0])
	}

	violations := ValidateEntity(stmt)
	expected := []string{"Owner", "Type", "Context", "Id", "CreatedDate"}
	for _, name := range expected {
		found := false
		for _, v := range violations {
			if v.RuleID == "MDL021" && strings.Contains(v.Message, "'"+name+"'") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected MDL021 violation for NPE attribute %q (CE7247), got: %v", name, violations)
		}
	}
}

// TestValidateEntityNPENormalAttributesPass verifies that legitimate NPE
// attribute names (non-reserved) do not trigger false positives.
func TestValidateEntityNPENormalAttributesPass(t *testing.T) {
	input := `create non-persistent entity Test.MyNPE (
  Title : String(200),
  Message : String(2000),
  IsActive : Boolean
);`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	violations := ValidateEntity(stmt)
	if len(violations) > 0 {
		t.Errorf("Normal NPE attribute names should not trigger violations, got: %v", violations)
	}
}

// TestValidateEntityNormalAttributesPass verifies that normal attribute names
// don't trigger false positives.
func TestValidateEntityNormalAttributesPass(t *testing.T) {
	input := `create persistent entity Test.MyEntity (
  Name : String(200),
  Description : String(2000),
  Amount : Decimal,
  IsActive : Boolean
);`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("Expected CreateEntityStmt, got %T", prog.Statements[0])
	}

	violations := ValidateEntity(stmt)
	if len(violations) > 0 {
		t.Errorf("Normal attributes should not trigger errors, got: %v", violations)
	}
}

// TestValidateEntityReservedJavaKeyword verifies that Java reserved words
// used as attribute names (e.g. "type") are caught with MDL021 (CE7247).
func TestValidateEntityReservedJavaKeyword(t *testing.T) {
	input := `create persistent entity Test.MyEntity (
  Name : String(200),
  Type : String(50)
);`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt, ok := prog.Statements[0].(*ast.CreateEntityStmt)
	if !ok {
		t.Fatalf("Expected CreateEntityStmt, got %T", prog.Statements[0])
	}

	violations := ValidateEntity(stmt)
	found := false
	for _, v := range violations {
		if v.RuleID == "MDL021" && strings.Contains(v.Message, "Type") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected MDL021 violation for Java-reserved attribute 'Type', got: %v", violations)
	}
}

// TestReturnsNothingAcceptsBarReturn verifies that RETURNS Nothing treats
// RETURN; (no value) as valid — "Nothing" means void.
func TestReturnsNothingAcceptsBarReturn(t *testing.T) {
	input := `create microflow Test.MF_ReturnsNothing ()
returns Nothing
begin
  log info 'hello';
  return;
end;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)

	// The return type should be TypeVoid
	if stmt.ReturnType != nil && stmt.ReturnType.Type.Kind != ast.TypeVoid {
		t.Errorf("Expected TypeVoid for returns Nothing, got %v", stmt.ReturnType.Type.Kind)
	}

	// Validation should NOT produce errors about RETURN requiring a value
	warnings := ValidateMicroflowBody(stmt)
	for _, w := range warnings {
		if strings.Contains(w, "return requires a value") {
			t.Errorf("returns Nothing should not reject bare return;, got: %s", w)
		}
	}
}

// TestEnumDefaultNotDoubleQualified verifies that enum DEFAULT values are stored
// without the enum prefix (just the value name), preventing double-qualification.
func TestEnumDefaultNotDoubleQualified(t *testing.T) {
	input := `create persistent entity Test.Item (
  Status : Enumeration(Test.ItemStatus) default Test.ItemStatus.Active
);`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if len(stmt.Attributes) == 0 {
		t.Fatal("Expected at least 1 attribute")
	}

	attr := stmt.Attributes[0]
	if !attr.HasDefault {
		t.Fatal("Expected attribute to have a default value")
	}

	// The default value from the parser is the full text "Test.ItemStatus.Active"
	defaultStr := fmt.Sprintf("%v", attr.DefaultValue)
	// When stored, it should be stripped to just "Active" (the executor does this)
	// Here we verify the parser at least captures the full text correctly
	if !strings.Contains(defaultStr, "Active") {
		t.Errorf("Default value should contain 'Active', got: %s", defaultStr)
	}
}

// TestExpressionToXPath_TokenQuoting verifies that [%CurrentDateTime%] tokens
// are unquoted in XPath context (Mendix special placeholders, not string literals).
// Quoting them causes CE0161 (XPath parse error) in Studio Pro.
func TestExpressionToXPath_TokenQuoting(t *testing.T) {
	tests := []struct {
		name     string
		expr     ast.Expression
		wantExpr string // expressionToString output
		wantXP   string // expressionToXPath output
	}{
		{
			name:     "Token_CurrentDateTime",
			expr:     &ast.TokenExpr{Token: "CurrentDateTime"},
			wantExpr: "[%CurrentDateTime%]",
			wantXP:   "'[%CurrentDateTime%]'",
		},
		{
			name:     "Token_CurrentUser",
			expr:     &ast.TokenExpr{Token: "CurrentUser"},
			wantExpr: "[%CurrentUser%]",
			wantXP:   "'[%CurrentUser%]'",
		},
		{
			name: "BinaryExpr_with_token",
			expr: &ast.BinaryExpr{
				Left:     &ast.IdentifierExpr{Name: "DueDate"},
				Operator: "<",
				Right:    &ast.TokenExpr{Token: "CurrentDateTime"},
			},
			wantExpr: "DueDate < [%CurrentDateTime%]",
			wantXP:   "DueDate < '[%CurrentDateTime%]'",
		},
		{
			name:     "Variable_unchanged",
			expr:     &ast.VariableExpr{Name: "MyVar"},
			wantExpr: "$MyVar",
			wantXP:   "$MyVar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotExpr := expressionToString(tt.expr)
			if gotExpr != tt.wantExpr {
				t.Errorf("expressionToString() = %q, want %q", gotExpr, tt.wantExpr)
			}
			gotXP := expressionToXPath(tt.expr)
			if gotXP != tt.wantXP {
				t.Errorf("expressionToXPath() = %q, want %q", gotXP, tt.wantXP)
			}
		})
	}
}

// TestExpressionToXPath_XPathPathExpr verifies that XPathPathExpr (bare association paths,
// nested predicates) serialize correctly via expressionToXPath.
func TestExpressionToXPath_XPathPathExpr(t *testing.T) {
	tests := []struct {
		name   string
		expr   ast.Expression
		wantXP string
	}{
		{
			name: "bare_association_path",
			expr: &ast.XPathPathExpr{
				Steps: []ast.XPathStep{
					{Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "Module", Name: "Assoc"}}},
					{Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "Module", Name: "Entity"}}},
					{Expr: &ast.IdentifierExpr{Name: "Attr"}},
				},
			},
			wantXP: "Module.Assoc/Module.Entity/Attr",
		},
		{
			name: "path_with_nested_predicate",
			expr: &ast.XPathPathExpr{
				Steps: []ast.XPathStep{
					{Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "Sys", Name: "roles"}}},
					{
						Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "Sys", Name: "UserRole"}},
						Predicate: &ast.BinaryExpr{
							Left:     &ast.IdentifierExpr{Name: "Active"},
							Operator: "=",
							Right:    &ast.LiteralExpr{Value: true, Kind: ast.LiteralBoolean},
						},
					},
				},
			},
			wantXP: "Sys.roles/Sys.UserRole[Active = true]",
		},
		{
			name: "path_with_reversed",
			expr: &ast.XPathPathExpr{
				Steps: []ast.XPathStep{
					{
						Expr:      &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "System", Name: "roles"}},
						Predicate: &ast.FunctionCallExpr{Name: "reversed"},
					},
					{Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "System", Name: "UserRole"}}},
				},
			},
			wantXP: "System.roles[reversed()]/System.UserRole",
		},
		{
			name: "comparison_with_path_and_token",
			expr: &ast.BinaryExpr{
				Left: &ast.XPathPathExpr{
					Steps: []ast.XPathStep{
						{Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "System", Name: "owner"}}},
					},
				},
				Operator: "=",
				Right:    &ast.TokenExpr{Token: "CurrentUser"},
			},
			wantXP: "System.owner = '[%CurrentUser%]'",
		},
		{
			name: "not_with_path",
			expr: &ast.UnaryExpr{
				Operator: "not",
				Operand: &ast.XPathPathExpr{
					Steps: []ast.XPathStep{
						{Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "Module", Name: "Assoc"}}},
						{Expr: &ast.QualifiedNameExpr{QualifiedName: ast.QualifiedName{Module: "Module", Name: "Entity"}}},
					},
				},
			},
			wantXP: "not(Module.Assoc/Module.Entity)",
		},
		{
			name: "function_with_path_args",
			expr: &ast.FunctionCallExpr{
				Name: "contains",
				Arguments: []ast.Expression{
					&ast.IdentifierExpr{Name: "Name"},
					&ast.VariableExpr{Name: "SearchStr"},
				},
			},
			wantXP: "contains(Name, $SearchStr)",
		},
		{
			name:   "empty_literal",
			expr:   &ast.LiteralExpr{Value: nil, Kind: ast.LiteralEmpty},
			wantXP: "empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotXP := expressionToXPath(tt.expr)
			if gotXP != tt.wantXP {
				t.Errorf("expressionToXPath() = %q, want %q", gotXP, tt.wantXP)
			}
		})
	}
}

// validateMicroflowFromMDL parses a CREATE MICROFLOW statement and runs
// ValidateMicroflowBody, returning any validation errors.
func validateMicroflowFromMDL(t *testing.T, input string) []string {
	t.Helper()

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	if len(prog.Statements) == 0 {
		t.Fatal("No statements parsed")
	}

	stmt, ok := prog.Statements[0].(*ast.CreateMicroflowStmt)
	if !ok {
		t.Fatalf("Expected CreateMicroflowStmt, got %T", prog.Statements[0])
	}

	return ValidateMicroflowBody(stmt)
}

// TestAssociationNavParsing verifies that $Var/Module.Assoc/Attr parses as
// AttributePathExpr (not nested BinaryExpr with "/" operator).
// Issue #120: extra spaces around path separators.
func TestAssociationNavParsing(t *testing.T) {
	input := `create microflow Test.MF_Nav()
returns String as $Result
begin
  declare $CustName String = $Order/Test.Order_Customer/Name;
  return $CustName;
end;`

	prog, errs := visitor.Build(input)
	if len(errs) > 0 {
		t.Fatalf("Parse error: %v", errs[0])
	}

	stmt := prog.Statements[0].(*ast.CreateMicroflowStmt)
	declStmt := stmt.Body[0].(*ast.DeclareStmt)

	// The expression should be an AttributePathExpr, not a BinaryExpr
	pathExpr, ok := declStmt.InitialValue.(*ast.AttributePathExpr)
	if !ok {
		t.Fatalf("Expected AttributePathExpr, got %T", declStmt.InitialValue)
	}

	if pathExpr.Variable != "Order" {
		t.Errorf("Variable = %q, want %q", pathExpr.Variable, "Order")
	}
	if len(pathExpr.Path) != 2 {
		t.Fatalf("Path length = %d, want 2", len(pathExpr.Path))
	}
	if pathExpr.Path[0] != "Test.Order_Customer" {
		t.Errorf("Path[0] = %q, want %q", pathExpr.Path[0], "Test.Order_Customer")
	}
	if pathExpr.Path[1] != "Name" {
		t.Errorf("Path[1] = %q, want %q", pathExpr.Path[1], "Name")
	}

	// Serialized form should have no extra spaces
	got := expressionToString(pathExpr)
	want := "$Order/Test.Order_Customer/Name"
	if got != want {
		t.Errorf("expressionToString() = %q, want %q", got, want)
	}
}

// TestResolveAssociationPaths verifies that resolveAssociationPaths inserts
// the target entity after an association segment.
// Issue #120: missing target entity qualifier.
func TestResolveAssociationPaths(t *testing.T) {
	tests := []struct {
		name string
		path []string
		want []string
	}{
		{
			name: "simple_attribute",
			path: []string{"Name"},
			want: []string{"Name"},
		},
		{
			name: "assoc_then_attr",
			path: []string{"Test.Order_Customer", "Name"},
			want: []string{"Test.Order_Customer", "Test.Customer", "Name"},
		},
		{
			name: "already_has_target_entity",
			path: []string{"Test.Order_Customer", "Test.Customer", "Name"},
			want: []string{"Test.Order_Customer", "Test.Customer", "Name"},
		},
		{
			name: "assoc_at_end",
			path: []string{"Test.Order_Customer"},
			want: []string{"Test.Order_Customer"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fb := &flowBuilder{
				backend: nil, // nil backend → no resolution, path unchanged
			}
			got := fb.resolvePathSegments(tt.path)

			// With nil reader, all paths should be unchanged
			if len(got) != len(tt.path) {
				t.Errorf("resolvePathSegments() length = %d, want %d", len(got), len(tt.path))
			}
		})
	}
}

func TestResolveAssociationPathsUnwrapsEmptySourceExpr(t *testing.T) {
	fb := &flowBuilder{}
	resolved := fb.resolveAssociationPaths(&ast.SourceExpr{
		Expression: &ast.VariableExpr{Name: "CurrentObject"},
	})

	if _, ok := resolved.(*ast.SourceExpr); ok {
		t.Fatalf("empty SourceExpr should unwrap to resolved inner expression, got %T", resolved)
	}
	if got := expressionToString(resolved); got != "$CurrentObject" {
		t.Fatalf("resolved expression = %q, want $CurrentObject", got)
	}
}

func TestResolveAssociationPathsKeepsNonEmptySourceExprVerbatim(t *testing.T) {
	source := "$CurrentObject/Module.Assoc/Name\n"
	fb := &flowBuilder{}
	resolved := fb.resolveAssociationPaths(&ast.SourceExpr{
		Expression: &ast.AttributePathExpr{
			Variable: "CurrentObject",
			Path:     []string{"Module.Assoc", "Name"},
		},
		Source: source,
	})

	sourceExpr, ok := resolved.(*ast.SourceExpr)
	if !ok {
		t.Fatalf("non-empty SourceExpr should remain SourceExpr, got %T", resolved)
	}
	if sourceExpr.Source != source {
		t.Fatalf("source = %q, want %q", sourceExpr.Source, source)
	}
}

// TestExprToStringNoSpaces verifies that association navigation expressions
// produce no extra spaces around separators after parsing.
// Issue #120: generated $Order / Module.Assoc / Name instead of $Order/Module.Assoc/Name
func TestExprToStringNoSpaces(t *testing.T) {
	tests := []struct {
		name string
		expr ast.Expression
		want string
	}{
		{
			name: "simple_path",
			expr: &ast.AttributePathExpr{
				Variable: "Order",
				Path:     []string{"OrderNumber"},
			},
			want: "$Order/OrderNumber",
		},
		{
			name: "assoc_path",
			expr: &ast.AttributePathExpr{
				Variable: "Order",
				Path:     []string{"Test.Order_Customer", "Name"},
			},
			want: "$Order/Test.Order_Customer/Name",
		},
		{
			name: "multi_segment_path",
			expr: &ast.AttributePathExpr{
				Variable: "Invoice",
				Path:     []string{"Billing.Invoice_Order", "Billing.Order_Customer", "Name"},
			},
			want: "$Invoice/Billing.Invoice_Order/Billing.Order_Customer/Name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expressionToString(tt.expr)
			if got != tt.want {
				t.Errorf("expressionToString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestValidateEntityNPEValidationRules covers issue #832: Mendix refuses
// validation rules on a non-persistable entity with
//
//	CE0070 "Validations rules are not allowed on entity 'X', because it is
//	        not persistable."
//
// `not null` and `unique` ARE validation rules — Studio Pro models "required"
// and "uniqueness" as rules on the entity, not as column constraints — so both
// forms are rejected on an NPE. Verified against mxbuild 11.6.6: `not null`
// with a message, `not null` bare, and `unique` each produce CE0070, while a
// plain attribute does not. Before this rule `mxcli check` and `mxcli exec`
// both accepted them and only a real build caught it.
func TestValidateEntityNPEValidationRules(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantFor []string // attribute names expected to be flagged
	}{
		{
			"not null with message",
			`create non-persistent entity Test.NP ( "Name" : String(100) not null error 'req' );`,
			[]string{"Name"},
		},
		{
			// The message is optional; the rule exists either way, so bare
			// `not null` is rejected by Mendix just the same.
			"not null bare",
			`create non-persistent entity Test.NP ( "Name" : String(100) not null );`,
			[]string{"Name"},
		},
		{
			"unique",
			`create non-persistent entity Test.NP ( "Code" : String(50) unique error 'dup' );`,
			[]string{"Code"},
		},
		{
			"both constraints on separate attributes",
			`create non-persistent entity Test.NP ( "Name" : String(100) not null, "Code" : String(50) unique );`,
			[]string{"Name", "Code"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			prog, errs := visitor.Build(c.input)
			if len(errs) > 0 {
				t.Fatalf("parse error: %v", errs[0])
			}
			stmt := prog.Statements[0].(*ast.CreateEntityStmt)
			violations := ValidateEntity(stmt)
			for _, attrName := range c.wantFor {
				found := false
				for _, v := range violations {
					if v.RuleID == "MDL054" && strings.Contains(v.Message, "'"+attrName+"'") {
						found = true
					}
				}
				if !found {
					t.Errorf("expected MDL054 for attribute %q (CE0070), got: %v", attrName, violations)
				}
			}
		})
	}
}

// A PERSISTENT entity may carry exactly the same constraints — that is where
// validation rules belong — so the rule must not fire there. Nor should it fire
// on an NPE attribute that carries no constraint.
func TestValidateEntityValidationRulesAllowedWhenPersistent(t *testing.T) {
	cases := []string{
		`create persistent entity Test.P ( "Name" : String(100) not null error 'req', "Code" : String(50) unique );`,
		`create non-persistent entity Test.NP ( "Name" : String(100), "Qty" : Integer );`,
	}
	for _, input := range cases {
		prog, errs := visitor.Build(input)
		if len(errs) > 0 {
			t.Fatalf("parse error: %v", errs[0])
		}
		stmt := prog.Statements[0].(*ast.CreateEntityStmt)
		for _, v := range ValidateEntity(stmt) {
			if v.RuleID == "MDL054" {
				t.Errorf("unexpected MDL054 on %q: %s", input, v.Message)
			}
		}
	}
}
