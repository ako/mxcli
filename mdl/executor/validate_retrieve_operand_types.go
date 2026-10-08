// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// A retrieve constraint comparing an attribute with a variable of another type
// passed `check --references` and exec, then mxbuild reported CE0161 "Error(s)
// in XPath constraint." (mendixlabs/mxcli#1325). The member check
// (validateRetrieveMembers) resolved the attribute; nothing compared its type
// with the variable's.
//
// Measured on mxbuild 11.14.0, `[Attr = $P]` and `[Attr >= $P]` over every pair
// of String, Integer, Long, Decimal, Boolean, DateTime and two enumerations. A
// pair builds clean only when:
//
//	String attribute       String variable
//	Integer/Long/Decimal   Integer, Long or Decimal variable (any mix)
//	Boolean                Boolean
//	DateTime               DateTime, or String
//	Enumeration E          Enumeration E (the same one)
//
// Every other pair is CE0161. A String variable against a DateTime attribute
// builds — "the variable must be the attribute's type" would refuse it.
//
// Silence wherever a side cannot be typed: an attribute reached over a path, an
// inherited one, an attribute or variable of a kind not measured (Date,
// AutoNumber, Binary, HashedString), an object or list variable.

// validateRetrieveOperandTypes reports each database retrieve's comparison of
// an attribute with a variable whose type mxbuild rejects for it.
func validateRetrieveOperandTypes(ctx *ExecContext, params []ast.MicroflowParam, body []ast.MicroflowStatement, sc *scriptContext) []string {
	vars := map[string]ast.DataType{}
	for _, p := range params {
		vars[p.Name] = p.Type
	}
	var retrieves []*ast.RetrieveStmt
	walkFlowStatements(body, func(s ast.MicroflowStatement) {
		switch st := s.(type) {
		case *ast.DeclareStmt:
			vars[st.Variable] = st.Type
		case *ast.RetrieveStmt:
			if st.StartVariable == "" && st.Source.Module != "" && st.Where != nil {
				retrieves = append(retrieves, st)
			}
		}
	})

	var errs []string
	for _, r := range retrieves {
		entityQN := r.Source.String()
		walkXPathComparisons(r.Where, func(attr string, variable string) {
			vt, ok := vars[variable]
			if !ok {
				return
			}
			at, ok := retrieveAttributeType(ctx, sc, entityQN, attr)
			if !ok || xpathOperandTypesCompatible(at, vt) != operandIncompatible {
				return
			}
			hint := ""
			if at.Kind == ast.TypeEnumeration {
				hint = ". Compare with a variable of the enumeration type, or a string literal of the value key"
			}
			errs = append(errs, fmt.Sprintf(
				"retrieve from %s: the constraint compares %s (%s) with $%s (%s) — mxbuild rejects the constraint (CE0161 \"Error(s) in XPath constraint\")%s",
				entityQN, attr, xpathOperandTypeName(at), variable, xpathOperandTypeName(vt), hint))
		})
	}
	return errs
}

// walkXPathComparisons calls fn for each comparison of a bare attribute with a
// plain variable, either way round, through and/or, not() and parentheses.
func walkXPathComparisons(e ast.Expression, fn func(attr, variable string)) {
	switch x := e.(type) {
	case *ast.SourceExpr:
		walkXPathComparisons(x.Expression, fn)
	case *ast.ParenExpr:
		walkXPathComparisons(x.Inner, fn)
	case *ast.UnaryExpr:
		walkXPathComparisons(x.Operand, fn)
	case *ast.FunctionCallExpr:
		if strings.EqualFold(x.Name, "not") {
			for _, a := range x.Arguments {
				walkXPathComparisons(a, fn)
			}
		}
	case *ast.BinaryExpr:
		switch strings.ToLower(x.Operator) {
		case "and", "or":
			walkXPathComparisons(x.Left, fn)
			walkXPathComparisons(x.Right, fn)
		case "=", "!=", "<>", "<", ">", "<=", ">=":
			if id, ok := x.Left.(*ast.IdentifierExpr); ok {
				if v, ok := x.Right.(*ast.VariableExpr); ok {
					fn(id.Name, v.Name)
				}
			} else if v, ok := x.Left.(*ast.VariableExpr); ok {
				if id, ok := x.Right.(*ast.IdentifierExpr); ok {
					fn(id.Name, v.Name)
				}
			}
		}
	}
}

// retrieveAttributeType is the declared type of an attribute of the retrieved
// entity: from the script's declaration when the script creates the entity,
// else from the stored domain model. Inherited members are not resolved.
func retrieveAttributeType(ctx *ExecContext, sc *scriptContext, entityQN, attr string) (ast.DataType, bool) {
	if sc != nil {
		if decl := sc.entityDecls[entityQN]; decl != nil && !sc.alteredEntities[entityQN] {
			for _, a := range decl.Attributes {
				if a.Name == attr {
					return a.Type, true
				}
			}
			return ast.DataType{}, false
		}
	}
	t := inferAttributeTypeFromEntity(ctx, entityQN, attr)
	return t, t.Kind != ast.TypeUnknown
}

type operandCompat int

const (
	operandUnknown operandCompat = iota
	operandCompatible
	operandIncompatible
)

// xpathOperandClass groups the measured kinds by what they compare with.
func xpathOperandClass(k ast.DataTypeKind) string {
	switch k {
	case ast.TypeString:
		return "string"
	case ast.TypeInteger, ast.TypeLong, ast.TypeDecimal:
		return "number"
	case ast.TypeBoolean:
		return "boolean"
	case ast.TypeDateTime:
		return "datetime"
	case ast.TypeEnumeration:
		return "enumeration"
	}
	return ""
}

// xpathOperandTypesCompatible reports whether mxbuild accepts comparing an
// attribute of type attr with a variable of type v (the table above).
func xpathOperandTypesCompatible(attr, v ast.DataType) operandCompat {
	ac, vc := xpathOperandClass(attr.Kind), xpathOperandClass(v.Kind)
	if ac == "" || vc == "" {
		return operandUnknown
	}
	if ac == "enumeration" && vc == "enumeration" {
		a, b := qualifiedEnumRef(attr), qualifiedEnumRef(v)
		if a == "" || b == "" {
			return operandUnknown
		}
		if strings.EqualFold(a, b) {
			return operandCompatible
		}
		return operandIncompatible
	}
	if ac == vc || (ac == "datetime" && vc == "string") {
		return operandCompatible
	}
	return operandIncompatible
}

func qualifiedEnumRef(t ast.DataType) string {
	if t.EnumRef == nil || t.EnumRef.Module == "" {
		return ""
	}
	return t.EnumRef.String()
}

func xpathOperandTypeName(t ast.DataType) string {
	if t.Kind == ast.TypeEnumeration {
		if ref := qualifiedEnumRef(t); ref != "" {
			return "Enumeration " + ref
		}
	}
	return t.Kind.String()
}
