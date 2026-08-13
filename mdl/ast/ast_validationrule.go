// SPDX-License-Identifier: Apache-2.0

package ast

// ValidationRuleKind is the constraint a CREATE VALIDATION RULE statement carries.
type ValidationRuleKind string

const (
	// ValidationRuleRegEx matches the attribute against a named regular
	// expression document. Mendix stores the reference by qualified name, not
	// the pattern itself, which is why RegularExpression below is a name.
	ValidationRuleRegEx ValidationRuleKind = "RegEx"
	// ValidationRuleRange bounds a numeric or date attribute.
	ValidationRuleRange ValidationRuleKind = "Range"
)

// CreateValidationRuleStmt represents:
//
//	CREATE VALIDATION RULE FOR Module.Entity.Attribute
//	    REGEX Module.PatternName
//	    FEEDBACK 'message';
//
//	CREATE VALIDATION RULE FOR Module.Entity.Attribute
//	    RANGE FROM 1 TO 100
//	    FEEDBACK 'message';
//
// Mendix validation rules are anonymous and entity-scoped, so the statement
// names the attribute it constrains rather than the rule.
type CreateValidationRuleStmt struct {
	// Attribute is the fully qualified target, Module.Entity.Attribute.
	Attribute QualifiedName
	Kind      ValidationRuleKind
	// Feedback is the message shown when the rule fails (required).
	Feedback string

	// RegularExpression is the qualified name of the pattern document, for
	// ValidationRuleRegEx.
	RegularExpression QualifiedName

	// Min and Max are the inclusive bounds for ValidationRuleRange, as written.
	// A nil bound is absent: from-only and to-only ranges are both legal, and
	// which are set decides Mendix's TypeOfRange.
	Min *string
	Max *string
}

func (s *CreateValidationRuleStmt) isStatement() {}
