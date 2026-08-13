// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

func mkRegex(containerID model.ID, name, pattern string) *model.RegularExpression {
	re := &model.RegularExpression{ContainerID: containerID, Name: name, Expression: pattern}
	re.ID = nextID("regex")
	return re
}

func TestCreateRegularExpression_Mock(t *testing.T) {
	mod := mkModule("Val")
	h := mkHierarchy(mod)

	var created *model.RegularExpression
	mb := &mock.MockBackend{
		IsConnectedFunc:            func() bool { return true },
		ListModulesFunc:            func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) { return nil, nil },
		CreateRegularExpressionFunc: func(re *model.RegularExpression) error {
			created = re
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateRegularExpression(ctx, &ast.CreateRegularExpressionStmt{
		Name:       ast.QualifiedName{Module: "Val", Name: "Email"},
		Expression: `^[^@]+@[^@]+$`,
	}))

	if created == nil {
		t.Fatal("CreateRegularExpression was not called")
	}
	if created.Expression != `^[^@]+@[^@]+$` {
		t.Errorf("Expression = %q — the pattern must reach the backend verbatim", created.Expression)
	}
	assertContainsStr(t, buf.String(), "Created regular expression: Val.Email")
}

func TestCreateRegularExpression_Mock_RequiresExpression(t *testing.T) {
	mod := mkModule("Val")
	mb := &mock.MockBackend{
		IsConnectedFunc:            func() bool { return true },
		ListModulesFunc:            func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) { return nil, nil },
		CreateRegularExpressionFunc: func(re *model.RegularExpression) error {
			t.Error("must not write a regular expression with no pattern")
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(mkHierarchy(mod)))
	if err := execCreateRegularExpression(ctx, &ast.CreateRegularExpressionStmt{
		Name: ast.QualifiedName{Module: "Val", Name: "Empty"},
	}); err == nil {
		t.Fatal("expected an error for a missing Expression")
	}
}

func TestCreateRegularExpression_Mock_OrModifyReusesID(t *testing.T) {
	mod := mkModule("Val")
	existing := mkRegex(mod.ID, "Email", `^a$`)
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	var updated *model.RegularExpression
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) {
			return []*model.RegularExpression{existing}, nil
		},
		UpdateRegularExpressionFunc: func(re *model.RegularExpression) error {
			updated = re
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateRegularExpression(ctx, &ast.CreateRegularExpressionStmt{
		Name:           ast.QualifiedName{Module: "Val", Name: "Email"},
		Expression:     `^b$`,
		CreateOrModify: true,
	}))

	if updated == nil {
		t.Fatal("UpdateRegularExpression was not called")
	}
	// Reusing the stored ID is what keeps a validation rule's by-name reference
	// pointing at the same document.
	if updated.ID != existing.ID {
		t.Errorf("ID = %q, want the existing %q", updated.ID, existing.ID)
	}
	if updated.Expression != `^b$` {
		t.Errorf("Expression = %q", updated.Expression)
	}
	assertContainsStr(t, buf.String(), "Modified regular expression: Val.Email")
}

func TestCreateRegularExpression_Mock_DuplicateWithoutOrModify(t *testing.T) {
	mod := mkModule("Val")
	existing := mkRegex(mod.ID, "Email", `^a$`)
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) {
			return []*model.RegularExpression{existing}, nil
		},
		CreateRegularExpressionFunc: func(re *model.RegularExpression) error {
			t.Error("CreateRegularExpression must not be called for a duplicate")
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	if err := execCreateRegularExpression(ctx, &ast.CreateRegularExpressionStmt{
		Name: ast.QualifiedName{Module: "Val", Name: "Email"}, Expression: `^b$`,
	}); err == nil {
		t.Fatal("expected an already-exists error")
	}
}

func TestDropRegularExpression_Mock(t *testing.T) {
	mod := mkModule("Val")
	existing := mkRegex(mod.ID, "Email", `^a$`)
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	var deleted string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) {
			return []*model.RegularExpression{existing}, nil
		},
		DeleteRegularExpressionFunc: func(id string) error { deleted = id; return nil },
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDropRegularExpression(ctx, &ast.DropRegularExpressionStmt{
		Name: ast.QualifiedName{Module: "Val", Name: "Email"},
	}))
	if deleted != string(existing.ID) {
		t.Errorf("deleted %q, want %q", deleted, existing.ID)
	}
	assertContainsStr(t, buf.String(), "Dropped regular expression: Val.Email")
}

func TestShowRegularExpressions_Mock(t *testing.T) {
	mod := mkModule("Val")
	r1 := mkRegex(mod.ID, "Email", `^[^@]+@[^@]+$`)
	r2 := mkRegex(mod.ID, "Digits", `^\d+$`)
	h := mkHierarchy(mod)
	withContainer(h, r1.ContainerID, mod.ID)
	withContainer(h, r2.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) {
			return []*model.RegularExpression{r1, r2}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execShowRegularExpressions(ctx, &ast.ShowRegularExpressionsStmt{}))

	out := buf.String()
	assertContainsStr(t, out, "Val.Email")
	assertContainsStr(t, out, "Val.Digits")
	assertContainsStr(t, out, "(2 regular expression(s))")
}

// TestDescribeRegularExpression_Mock_OutputParses is the round-trip proof:
// a pattern is full of characters that a formatter can mangle, so assert the
// output re-parses to the same pattern rather than matching a string.
func TestDescribeRegularExpression_Mock_OutputParses(t *testing.T) {
	mod := mkModule("Val")
	for _, pattern := range []string{
		`^[^@]+@[^@]+$`,
		`\w+((-|\+|\.)\w+)*@\w+([\.-]?\w+)*(\.\w{2,})+`,
		`.*(?<!/)$`,
		`^it's$`, // a quote must survive as a doubled quote in the emitted MDL
		`^[a-zA-Z0-9_-]+|$`,
	} {
		t.Run(pattern, func(t *testing.T) {
			re := mkRegex(mod.ID, "R", pattern)
			h := mkHierarchy(mod)
			withContainer(h, re.ContainerID, mod.ID)
			mb := &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) {
					return []*model.RegularExpression{re}, nil
				},
			}
			ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
			assertNoError(t, execDescribeRegularExpression(ctx, &ast.DescribeRegularExpressionStmt{
				Name: ast.QualifiedName{Module: "Val", Name: "R"},
			}))

			prog, errs := visitor.Build(buf.String())
			if len(errs) > 0 {
				t.Fatalf("describe output does not parse: %v\n%s", errs, buf.String())
			}
			stmt, ok := prog.Statements[0].(*ast.CreateRegularExpressionStmt)
			if !ok {
				t.Fatalf("statement 0 = %T", prog.Statements[0])
			}
			if stmt.Expression != pattern {
				t.Errorf("round-tripped pattern = %q, want %q", stmt.Expression, pattern)
			}
		})
	}
}

// TestDescribeRegularExpression_Mock_FlagsDotNetOnlySyntax: Mendix validates
// with .NET's engine, which accepts lookaround that Go's RE2 does not. The note
// must say "not verifiable", not "invalid" — a real Mendix module ships one.
func TestDescribeRegularExpression_Mock_FlagsDotNetOnlySyntax(t *testing.T) {
	mod := mkModule("Val")
	re := mkRegex(mod.ID, "R", `.*(?<!/)$`)
	h := mkHierarchy(mod)
	withContainer(h, re.ContainerID, mod.ID)
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) {
			return []*model.RegularExpression{re}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDescribeRegularExpression(ctx, &ast.DescribeRegularExpressionStmt{
		Name: ast.QualifiedName{Module: "Val", Name: "R"},
	}))
	out := buf.String()
	if !strings.Contains(out, "not verifiable") {
		t.Errorf("expected a not-verifiable note for .NET-only syntax:\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "invalid") {
		t.Errorf("must not call a legal Mendix pattern invalid:\n%s", out)
	}
}

func TestRegexCompilesInGo(t *testing.T) {
	if !regexCompilesInGo(`^\d+$`) {
		t.Error("a plain pattern should compile")
	}
	// Lookbehind is .NET-only; Go's RE2 rejects it. This is the Email Connector's
	// real pattern, so "does not compile in Go" must never mean "invalid".
	if regexCompilesInGo(`.*(?<!/)$`) {
		t.Error("Go unexpectedly compiled a lookbehind — the describe note's premise needs rechecking")
	}
}
