// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCreateValidationRule_Regex(t *testing.T) {
	prog, errs := Build(`CREATE VALIDATION RULE FOR Shop.Product.Email
		REGEX Shop.EmailPattern
		FEEDBACK 'Enter a valid email address';`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateValidationRuleStmt)
	if !ok {
		t.Fatalf("expected CreateValidationRuleStmt, got %T", prog.Statements[0])
	}
	if got := stmt.Attribute.String(); got != "Shop.Product.Email" {
		t.Errorf("Attribute = %q", got)
	}
	if stmt.Kind != ast.ValidationRuleRegEx {
		t.Errorf("Kind = %q, want RegEx", stmt.Kind)
	}
	if got := stmt.RegularExpression.String(); got != "Shop.EmailPattern" {
		t.Errorf("RegularExpression = %q", got)
	}
	if stmt.Feedback != "Enter a valid email address" {
		t.Errorf("Feedback = %q", stmt.Feedback)
	}
}

// TestCreateValidationRule_RangeBounds is the case where reading the literals
// positionally is not enough: `from 1` and `to 100` both carry exactly one
// literal, and only the FROM/TO tokens say which bound it is. Getting this
// wrong would turn a minimum into a maximum silently.
func TestCreateValidationRule_RangeBounds(t *testing.T) {
	tests := []struct {
		name       string
		constraint string
		wantMin    string
		wantMax    string
		wantNoMin  bool
		wantNoMax  bool
	}{
		{name: "both bounds", constraint: "RANGE FROM 1 TO 100", wantMin: "1", wantMax: "100"},
		{name: "minimum only", constraint: "RANGE FROM 1", wantMin: "1", wantNoMax: true},
		{name: "maximum only", constraint: "RANGE TO 100", wantMax: "100", wantNoMin: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prog, errs := Build(`CREATE VALIDATION RULE FOR Shop.Booking.Guests ` +
				tt.constraint + ` FEEDBACK 'out of range';`)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			stmt := prog.Statements[0].(*ast.CreateValidationRuleStmt)
			if stmt.Kind != ast.ValidationRuleRange {
				t.Fatalf("Kind = %q, want Range", stmt.Kind)
			}
			if tt.wantNoMin && stmt.Min != nil {
				t.Errorf("Min = %q, want absent", *stmt.Min)
			}
			if tt.wantNoMax && stmt.Max != nil {
				t.Errorf("Max = %q, want absent", *stmt.Max)
			}
			if tt.wantMin != "" {
				if stmt.Min == nil {
					t.Fatalf("Min absent, want %q", tt.wantMin)
				}
				if *stmt.Min != tt.wantMin {
					t.Errorf("Min = %q, want %q", *stmt.Min, tt.wantMin)
				}
			}
			if tt.wantMax != "" {
				if stmt.Max == nil {
					t.Fatalf("Max absent, want %q", tt.wantMax)
				}
				if *stmt.Max != tt.wantMax {
					t.Errorf("Max = %q, want %q", *stmt.Max, tt.wantMax)
				}
			}
		})
	}
}

// TestCreateValidationRule_RejectsUnrepresentableForms covers the shapes the
// old grammar admitted and Mendix has no rule type for. They must not parse —
// the previous grammar accepted all of them and, having no visitor or handler,
// silently did nothing.
func TestCreateValidationRule_RejectsUnrepresentableForms(t *testing.T) {
	for _, input := range []string{
		// No EXPRESSION rule type exists in Mendix.
		`CREATE VALIDATION RULE FOR Shop.Product.Price EXPRESSION $Price > 0 FEEDBACK 'x';`,
		// Mendix stores a reference to a regex document, never an inline pattern.
		`CREATE VALIDATION RULE FOR Shop.Product.Email REGEX '^.+@.+$' FEEDBACK 'x';`,
		// Strict inequalities have no Mendix representation.
		`CREATE VALIDATION RULE FOR Shop.Booking.Guests RANGE > 1 FEEDBACK 'x';`,
		// FEEDBACK is not optional.
		`CREATE VALIDATION RULE FOR Shop.Product.Email REGEX Shop.EmailPattern;`,
	} {
		if _, errs := Build(input); len(errs) == 0 {
			t.Errorf("expected a parse error for: %s", input)
		}
	}
}
