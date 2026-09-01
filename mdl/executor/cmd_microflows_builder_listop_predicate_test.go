// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// predicateBuilder wires a flow builder over a Shop.Order entity carrying
// Status/Amount/Qty and an Order_Customer association, with $L typed as a list
// of it — the shape every case in issue #1002 was measured on.
func predicateBuilder(t *testing.T) *flowBuilder {
	t.Helper()
	mod := mkModule("Shop")
	order := &domainmodel.Entity{
		BaseElement: model.BaseElement{ID: nextID("ent")},
		ContainerID: nextID("dm"),
		Name:        "Order",
		Persistable: true,
		Attributes: []*domainmodel.Attribute{
			{Name: "Status"}, {Name: "Amount"}, {Name: "Qty"},
		},
	}
	dm := &domainmodel.DomainModel{
		BaseElement:  model.BaseElement{ID: nextID("dm")},
		ContainerID:  mod.ID,
		Entities:     []*domainmodel.Entity{order},
		Associations: []*domainmodel.Association{{Name: "Order_Customer"}},
	}
	mb := &mock.MockBackend{
		IsConnectedFunc:      func() bool { return true },
		ListModulesFunc:      func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		GetModuleByNameFunc:  func(name string) (*model.Module, error) { return mod, nil },
		ListDomainModelsFunc: func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:   func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
	}
	varTypes := map[string]string{"L": "List of Shop.Order"}
	return &flowBuilder{
		posX:         100,
		posY:         100,
		spacing:      HorizontalSpacing,
		backend:      mb,
		declaredVars: map[string]string{},
		varTypes:     varTypes,
		measurer:     &layoutMeasurer{varTypes: varTypes},
	}
}

// filterExpression builds a FILTER over $L with the given predicate and returns
// the expression the builder stored, plus any build errors.
//
// A nil source passes the condition as a bare tree, which is what the live
// `$R = FILTER($L, …)` syntax produces (visitor_microflow_statements.go). A
// non-empty source wraps it in the frozen SourceExpr the other visitor path
// builds (visitor_microflow_actions.go, buildSourceExpression) — the builder has
// to get both right, and only the second exercises the source-text rewrite.
func filterExpression(t *testing.T, fb *flowBuilder, source string, cond ast.Expression) (string, []string) {
	t.Helper()
	predicate := cond
	if source != "" {
		predicate = &ast.SourceExpr{Expression: cond, Source: source}
	}
	stmt := &ast.ListOperationStmt{
		Operation:      ast.ListOpFilter,
		InputVariable:  "L",
		OutputVariable: "R",
		Condition:      predicate,
	}
	oc := fb.buildFlowGraph([]ast.MicroflowStatement{stmt},
		&ast.MicroflowReturnType{Type: ast.DataType{Kind: ast.TypeBoolean}})
	var expr string
	for _, obj := range oc.Objects {
		act, ok := obj.(*microflows.ActionActivity)
		if !ok {
			continue
		}
		lo, ok := act.Action.(*microflows.ListOperationAction)
		if !ok {
			continue
		}
		switch op := lo.Operation.(type) {
		case *microflows.FilterOperation:
			expr = op.Expression
		case *microflows.FilterByAttributeOperation:
			expr = "BY-ATTRIBUTE " + op.Attribute + " " + op.Expression
		}
	}
	return expr, fb.GetErrors()
}

// bare builds `<name> <op> <literal>` the way the visitor does for a bare
// attribute predicate: an IdentifierExpr on the left. A literal wrapped in
// single quotes is a string, anything else an integer — enough to render the
// right-hand side the way the real parser would.
func bare(name, op, literal string) ast.Expression {
	lit := &ast.LiteralExpr{Value: literal, Kind: ast.LiteralString}
	switch {
	case strings.HasPrefix(literal, "'"):
		lit.Value = strings.Trim(literal, "'")
	case literal == "empty":
		lit.Kind = ast.LiteralEmpty
	default:
		lit.Kind = ast.LiteralInteger
	}
	return &ast.BinaryExpr{
		Left:     &ast.IdentifierExpr{Name: name},
		Operator: op,
		Right:    lit,
	}
}

// TestFilterPredicateQualifiesBareAttributes is issue #1002. A bare attribute in
// a filter-by-expression predicate was stored verbatim, which mxbuild rejects
// with CE0117 because the predicate is evaluated per item against
// $currentObject. Measured identically on mxbuild 11.11.0 and 11.13.0, so the
// issue's "Mendix 11.13 regression" diagnosis does not hold.
func TestFilterPredicateQualifiesBareAttributes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		cond   ast.Expression
		want   string
	}{
		{
			name:   "bare attribute with a comparison operator",
			source: "Amount > 0",
			cond:   bare("Amount", ">", "0"),
			want:   "$currentObject/Amount > 0",
		},
		{
			name:   "bare attribute with !=",
			source: "Status != 'Pending'",
			cond:   bare("Status", "!=", "'Pending'"),
			want:   "$currentObject/Status != 'Pending'",
		},
		{
			// The literal spells an attribute name and must survive untouched.
			name:   "string literal is not rewritten",
			source: "Qty > 0 and Status != 'Amount'",
			cond: &ast.BinaryExpr{
				Left:     bare("Qty", ">", "0"),
				Operator: "and",
				Right:    bare("Status", "!=", "'Amount'"),
			},
			want: "$currentObject/Qty > 0 and $currentObject/Status != 'Amount'",
		},
		{
			name:   "an association carries its module qualifier",
			source: "Order_Customer != empty",
			cond:   bare("Order_Customer", "!=", "empty"),
			want:   "$currentObject/Shop.Order_Customer != empty",
		},
		{
			// The shape the live `$R = FILTER($L, …)` syntax produces.
			name:   "bare attribute as a bare tree, no frozen source",
			source: "",
			cond:   bare("Amount", ">", "0"),
			want:   "$currentObject/Amount > 0",
		},
		{
			name:   "an already-qualified path is left alone",
			source: "$currentObject/Amount > 0",
			cond: &ast.BinaryExpr{
				Left: &ast.AttributePathExpr{Variable: "currentObject", Path: []string{"Amount"},
					Segments: []ast.PathSegment{{Name: "Amount", Separator: "/"}}},
				Operator: ">",
				Right:    &ast.LiteralExpr{Value: "0", Kind: ast.LiteralInteger},
			},
			want: "$currentObject/Amount > 0",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, errs := filterExpression(t, predicateBuilder(t), tc.source, tc.cond)
			if len(errs) > 0 {
				t.Fatalf("unexpected build errors: %v", errs)
			}
			if got != tc.want {
				t.Errorf("stored expression = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestFilterPredicateRefusesUnknownMember covers the other half: a bare name the
// element entity does not have used to reach mxbuild as CE0117. Nothing can be
// qualified, so it is refused here instead.
func TestFilterPredicateRefusesUnknownMember(t *testing.T) {
	_, errs := filterExpression(t, predicateBuilder(t), "Nonexistent > 0", bare("Nonexistent", ">", "0"))
	if len(errs) == 0 {
		t.Fatal("expected a build error for a name that is not a member of Shop.Order")
	}
	joined := strings.Join(errs, "\n")
	for _, want := range []string{"Nonexistent", "Shop.Order", "CE0117"} {
		if !strings.Contains(joined, want) {
			t.Errorf("error %q does not mention %q", joined, want)
		}
	}
}

// TestFilterPredicateKeepsAttributeReroute is the bug #343 regression guard. The
// `attr = value` form must stay on Microflows$Filter (filter BY ATTRIBUTE),
// which takes a member name rather than an expression — qualifying it into an
// expression would undo that fix.
func TestFilterPredicateKeepsAttributeReroute(t *testing.T) {
	got, errs := filterExpression(t, predicateBuilder(t), "", bare("Status", "=", "'Pending'"))
	if len(errs) > 0 {
		t.Fatalf("unexpected build errors: %v", errs)
	}
	if !strings.HasPrefix(got, "BY-ATTRIBUTE ") {
		t.Fatalf("`Status = 'Pending'` built as %q, want the filter-by-attribute operation (bug #343)", got)
	}
	if !strings.Contains(got, "Shop.Order.Status") {
		t.Errorf("filter-by-attribute names %q, want the Shop.Order.Status member", got)
	}
}

// TestFilterPredicateUntouchedWithoutElementEntity: when the list's element type
// was never tracked nothing is proven either way, so the predicate is passed
// through rather than guessed at or refused.
func TestFilterPredicateUntouchedWithoutElementEntity(t *testing.T) {
	fb := predicateBuilder(t)
	fb.varTypes = map[string]string{} // $L's type is unknown
	fb.measurer = &layoutMeasurer{varTypes: fb.varTypes}
	got, errs := filterExpression(t, fb, "Amount > 0", bare("Amount", ">", "0"))
	if len(errs) > 0 {
		t.Fatalf("unexpected build errors: %v", errs)
	}
	if got != "Amount > 0" {
		t.Errorf("stored expression = %q, want it passed through unchanged", got)
	}
}

// TestQualifyNamesInSource pins the source-text rewrite on its own. The
// predicate reaches the builder as a frozen SourceExpr (buildSourceExpression),
// so this is what actually decides the stored bytes.
func TestQualifyNamesInSource(t *testing.T) {
	names := map[string]string{"Amount": "Amount", "Assoc": "Shop.Assoc"}
	tests := []struct{ in, want string }{
		{"Amount > 0", "$currentObject/Amount > 0"},
		{"(Amount > 0)", "($currentObject/Amount > 0)"},
		{"Amount > 0 and Amount < 9", "$currentObject/Amount > 0 and $currentObject/Amount < 9"},
		{"Assoc != empty", "$currentObject/Shop.Assoc != empty"},
		// Already anchored to a variable, or qualified by a module: untouched.
		{"$currentObject/Amount > 0", "$currentObject/Amount > 0"},
		{"$other/Amount > 0", "$other/Amount > 0"},
		{"Mod.Amount > 0", "Mod.Amount > 0"},
		// Inside a string literal, including a doubled-quote escape.
		{"Status != 'Amount'", "Status != 'Amount'"},
		{"Status != 'it''s Amount' and Amount > 0",
			"Status != 'it''s Amount' and $currentObject/Amount > 0"},
		// A name that is not in the map is not touched.
		{"Other > 0", "Other > 0"},
	}
	for _, tc := range tests {
		if got := qualifyNamesInSource(tc.in, names); got != tc.want {
			t.Errorf("qualifyNamesInSource(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
