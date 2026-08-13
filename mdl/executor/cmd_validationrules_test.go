// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// validationRuleCtx wires a Shop module with one Product entity (Email, Guests)
// and one regular expression, Shop.EmailPattern.
func validationRuleCtx(t *testing.T, existing ...*domainmodel.ValidationRule) (*ExecContext, *domainmodel.Entity) {
	t.Helper()

	mod := mkModule("Shop")
	email := &domainmodel.Attribute{Name: "Email"}
	email.ID = nextID("attr")
	guests := &domainmodel.Attribute{Name: "Guests"}
	guests.ID = nextID("attr")

	product := &domainmodel.Entity{
		Name:            "Product",
		Attributes:      []*domainmodel.Attribute{email, guests},
		ValidationRules: existing,
	}
	product.ID = nextID("entity")

	dm := &domainmodel.DomainModel{
		BaseElement: model.BaseElement{ID: nextID("dm")},
		ContainerID: mod.ID,
		Entities:    []*domainmodel.Entity{product},
	}
	product.ContainerID = dm.ID

	h := mkHierarchy(mod)
	withContainer(h, dm.ID, mod.ID)
	withContainer(h, product.ContainerID, dm.ID)

	rx := &model.RegularExpression{ContainerID: mod.ID, Name: "EmailPattern", Expression: `^[^@]+@[^@]+$`}
	rx.ID = nextID("regex")

	mb := &mock.MockBackend{
		IsConnectedFunc:            func() bool { return true },
		ListModulesFunc:            func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListDomainModelsFunc:       func() ([]*domainmodel.DomainModel, error) { return []*domainmodel.DomainModel{dm}, nil },
		GetDomainModelFunc:         func(id model.ID) (*domainmodel.DomainModel, error) { return dm, nil },
		ListRegularExpressionsFunc: func() ([]*model.RegularExpression, error) { return []*model.RegularExpression{rx}, nil },
		UpdateEntityFunc:           func(dmID model.ID, e *domainmodel.Entity) error { return nil },
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	return ctx, product
}

func TestCreateValidationRule_Regex_Mock(t *testing.T) {
	ctx, product := validationRuleCtx(t)

	assertNoError(t, execCreateValidationRule(ctx, &ast.CreateValidationRuleStmt{
		Attribute:         ast.QualifiedName{Module: "Shop", Name: "Product.Email"},
		Kind:              ast.ValidationRuleRegEx,
		RegularExpression: ast.QualifiedName{Module: "Shop", Name: "EmailPattern"},
		Feedback:          "Enter a valid email address",
	}))

	if len(product.ValidationRules) != 1 {
		t.Fatalf("got %d rules, want 1", len(product.ValidationRules))
	}
	vr := product.ValidationRules[0]
	if vr.Type != "RegEx" {
		t.Errorf("Type = %q, want RegEx", vr.Type)
	}
	if string(vr.AttributeID) != "Shop.Product.Email" {
		t.Errorf("AttributeID = %q", vr.AttributeID)
	}
	info, ok := vr.Rule.(*domainmodel.RegexValidationRuleInfo)
	if !ok {
		t.Fatalf("Rule = %T, want *RegexValidationRuleInfo", vr.Rule)
	}
	if info.RegularExpressionQualifiedName != "Shop.EmailPattern" {
		t.Errorf("reference = %q", info.RegularExpressionQualifiedName)
	}
	if vr.ErrorMessage == nil || vr.ErrorMessage.Translations["en_US"] != "Enter a valid email address" {
		t.Errorf("feedback not carried: %+v", vr.ErrorMessage)
	}
}

// TestCreateValidationRule_Regex_UnknownPattern: the reference is stored by
// name, so a typo would produce a rule that loads and constrains nothing.
func TestCreateValidationRule_Regex_UnknownPattern(t *testing.T) {
	ctx, product := validationRuleCtx(t)

	err := execCreateValidationRule(ctx, &ast.CreateValidationRuleStmt{
		Attribute:         ast.QualifiedName{Module: "Shop", Name: "Product.Email"},
		Kind:              ast.ValidationRuleRegEx,
		RegularExpression: ast.QualifiedName{Module: "Shop", Name: "EmialPattern"},
		Feedback:          "x",
	})
	if err == nil {
		t.Fatal("expected an error for an unknown regular expression")
	}
	if len(product.ValidationRules) != 0 {
		t.Errorf("a rule was added despite the error: %+v", product.ValidationRules)
	}
}

func TestCreateValidationRule_Range_Mock(t *testing.T) {
	ctx, product := validationRuleCtx(t)
	lo, hi := "1", "100"

	assertNoError(t, execCreateValidationRule(ctx, &ast.CreateValidationRuleStmt{
		Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Guests"},
		Kind:      ast.ValidationRuleRange,
		Min:       &lo,
		Max:       &hi,
		Feedback:  "Between 1 and 100",
	}))

	if len(product.ValidationRules) != 1 {
		t.Fatalf("got %d rules, want 1", len(product.ValidationRules))
	}
	info, ok := product.ValidationRules[0].Rule.(*domainmodel.RangeValidationRuleInfo)
	if !ok {
		t.Fatalf("Rule = %T, want *RangeValidationRuleInfo", product.ValidationRules[0].Rule)
	}
	if !info.UseMinValue || !info.UseMaxValue {
		t.Errorf("bound flags = min:%v max:%v, want both true", info.UseMinValue, info.UseMaxValue)
	}
	if info.MinValue == nil || *info.MinValue != "1" || info.MaxValue == nil || *info.MaxValue != "100" {
		t.Errorf("bounds not carried: %+v", info)
	}
}

// TestCreateValidationRule_ReplacesSameTypeOnly: an attribute may carry several
// rules of different types at once (a Required and a RegEx), so re-creating one
// must displace only its own type — not clear the attribute's other rules.
func TestCreateValidationRule_ReplacesSameTypeOnly(t *testing.T) {
	required := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Email", Type: "Required"}
	oldRegex := &domainmodel.ValidationRule{
		AttributeID: "Shop.Product.Email",
		Type:        "RegEx",
		Rule:        &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: "Shop.OldPattern"},
	}
	otherAttr := &domainmodel.ValidationRule{AttributeID: "Shop.Product.Guests", Type: "RegEx"}
	ctx, product := validationRuleCtx(t, required, oldRegex, otherAttr)

	assertNoError(t, execCreateValidationRule(ctx, &ast.CreateValidationRuleStmt{
		Attribute:         ast.QualifiedName{Module: "Shop", Name: "Product.Email"},
		Kind:              ast.ValidationRuleRegEx,
		RegularExpression: ast.QualifiedName{Module: "Shop", Name: "EmailPattern"},
		Feedback:          "x",
	}))

	var types []string
	for _, vr := range product.ValidationRules {
		types = append(types, string(vr.AttributeID)+":"+vr.Type)
	}
	if len(product.ValidationRules) != 3 {
		t.Fatalf("got %d rules %v, want 3 (Required kept, other attribute kept, RegEx replaced)", len(product.ValidationRules), types)
	}
	for _, vr := range product.ValidationRules {
		if vr.Type != "RegEx" || string(vr.AttributeID) != "Shop.Product.Email" {
			continue
		}
		info, ok := vr.Rule.(*domainmodel.RegexValidationRuleInfo)
		if !ok || info.RegularExpressionQualifiedName != "Shop.EmailPattern" {
			t.Errorf("the RegEx rule was not replaced: %+v", vr.Rule)
		}
	}
}

func TestCreateValidationRule_Rejects(t *testing.T) {
	tests := []struct {
		name string
		stmt *ast.CreateValidationRuleStmt
	}{
		{"unqualified attribute", &ast.CreateValidationRuleStmt{
			Attribute: ast.QualifiedName{Module: "Shop", Name: "Product"},
			Kind:      ast.ValidationRuleRegEx,
			Feedback:  "x",
		}},
		{"no feedback", &ast.CreateValidationRuleStmt{
			Attribute:         ast.QualifiedName{Module: "Shop", Name: "Product.Email"},
			Kind:              ast.ValidationRuleRegEx,
			RegularExpression: ast.QualifiedName{Module: "Shop", Name: "EmailPattern"},
		}},
		{"unknown attribute", &ast.CreateValidationRuleStmt{
			Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Nope"},
			Kind:      ast.ValidationRuleRange,
			Feedback:  "x",
		}},
		{"range with no bounds", &ast.CreateValidationRuleStmt{
			Attribute: ast.QualifiedName{Module: "Shop", Name: "Product.Guests"},
			Kind:      ast.ValidationRuleRange,
			Feedback:  "x",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, product := validationRuleCtx(t)
			if err := execCreateValidationRule(ctx, tt.stmt); err == nil {
				t.Error("expected an error")
			}
			if len(product.ValidationRules) != 0 {
				t.Errorf("a rule was added despite the error")
			}
		})
	}
}

// TestDescribeValidationConstraint covers the DESCRIBE side of the roundtrip:
// each authorable shape renders back to the syntax that produced it, and a
// shape MDL cannot author is reported as such rather than rendered wrong.
func TestDescribeValidationConstraint(t *testing.T) {
	lo, hi := "1", "100"
	tests := []struct {
		name string
		rule *domainmodel.ValidationRule
		want string
		ok   bool
	}{
		{
			name: "regex",
			rule: &domainmodel.ValidationRule{Type: "RegEx",
				Rule: &domainmodel.RegexValidationRuleInfo{RegularExpressionQualifiedName: "Shop.EmailPattern"}},
			want: "regex Shop.EmailPattern", ok: true,
		},
		{
			name: "range both",
			rule: &domainmodel.ValidationRule{Type: "Range",
				Rule: &domainmodel.RangeValidationRuleInfo{MinValue: &lo, MaxValue: &hi}},
			want: "range from 1 to 100", ok: true,
		},
		{
			name: "range from",
			rule: &domainmodel.ValidationRule{Type: "Range",
				Rule: &domainmodel.RangeValidationRuleInfo{MinValue: &lo}},
			want: "range from 1", ok: true,
		},
		{
			name: "range to",
			rule: &domainmodel.ValidationRule{Type: "Range",
				Rule: &domainmodel.RangeValidationRuleInfo{MaxValue: &hi}},
			want: "range to 100", ok: true,
		},
		{
			name: "attribute-bounded range is not authorable",
			rule: &domainmodel.ValidationRule{Type: "Range",
				Rule: &domainmodel.RangeValidationRuleInfo{MinAttributeQualifiedName: "Shop.Booking.StartDate"}},
		},
		{
			name: "regex with no reference",
			rule: &domainmodel.ValidationRule{Type: "RegEx", Rule: &domainmodel.RegexValidationRuleInfo{}},
		},
		{
			name: "payload dropped by the reader",
			rule: &domainmodel.ValidationRule{Type: "RegEx"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := describeValidationConstraint(tt.rule)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tt.ok, got)
			}
			if ok && got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCreateValidationRule_DescribeRoundTrip: what DESCRIBE emits must parse
// back into the statement it came from.
func TestCreateValidationRule_DescribeRoundTrip(t *testing.T) {
	rendered := "create validation rule for Shop.Customer.Email\n    regex Shop.EmailPattern\n    feedback 'it''s required';"

	prog, errs := visitor.Build(rendered)
	if len(errs) > 0 {
		t.Fatalf("DESCRIBE output does not parse: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateValidationRuleStmt)
	if !ok {
		t.Fatalf("got %T", prog.Statements[0])
	}
	if stmt.Attribute.String() != "Shop.Customer.Email" || stmt.RegularExpression.String() != "Shop.EmailPattern" {
		t.Errorf("roundtrip lost the target or reference: %+v", stmt)
	}
	if stmt.Feedback != "it's required" {
		t.Errorf("Feedback = %q — a doubled quote must decode back to one", stmt.Feedback)
	}
}

// TestCreateValidationRule_EndToEnd runs the parsed statement through the
// registry, so grammar, visitor, registration and handler are all exercised
// together — the layer where "parses but silently does nothing" used to live.
func TestCreateValidationRule_EndToEnd(t *testing.T) {
	ctx, product := validationRuleCtx(t)

	prog, errs := visitor.Build(`CREATE VALIDATION RULE FOR Shop.Product.Email
		REGEX Shop.EmailPattern
		FEEDBACK 'Enter a valid email address';`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}

	for _, stmt := range prog.Statements {
		if err := NewRegistry().Dispatch(ctx, stmt); err != nil {
			t.Fatalf("execute: %v", err)
		}
	}

	if len(product.ValidationRules) != 1 {
		t.Fatalf("statement parsed but wrote nothing — got %d rules", len(product.ValidationRules))
	}
}
