// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// stripXPathBrackets removes the outer [ and ] from a Mendix XPath constraint string.
// Returns the inner expression ready for parsing.
func stripXPathBrackets(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
		return s[1 : len(s)-1]
	}
	return s
}

// robustExprToStarlark converts a RobustExpr AST node to a Starlark struct tree.
// Each node is a struct with a "kind" field and type-specific fields.
// Returns a struct with kind="null" for a nil node.
func robustExprToStarlark(expr exprcheck.RobustExpr) starlark.Value {
	if expr == nil {
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind": starlark.String("null"),
		})
	}

	switch n := expr.(type) {
	case *exprcheck.BinExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("bin"),
			"op":    starlark.String(n.Op),
			"left":  robustExprToStarlark(n.L),
			"right": robustExprToStarlark(n.R),
		})
	case *exprcheck.UnaryExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":    starlark.String("unary"),
			"op":      starlark.String(n.Op),
			"operand": robustExprToStarlark(n.Operand),
		})
	case *exprcheck.CallExpr:
		args := make([]starlark.Value, len(n.Args))
		for i, a := range n.Args {
			args[i] = robustExprToStarlark(a)
		}
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind": starlark.String("call"),
			"name": starlark.String(n.Name),
			"args": starlark.NewList(args),
		})
	case *exprcheck.StringLit:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("string"),
			"value": starlark.String(n.Value),
		})
	case *exprcheck.NumberLit:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("number"),
			"value": starlark.String(n.Value),
		})
	case *exprcheck.BoolLit:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("bool"),
			"value": starlark.Bool(n.Value),
		})
	case *exprcheck.EmptyExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind": starlark.String("empty"),
		})
	case *exprcheck.VariableExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind": starlark.String("variable"),
			"name": starlark.String(n.Name),
		})
	case *exprcheck.AttributePathExpr:
		pathParts := make([]starlark.Value, len(n.Path))
		for i, p := range n.Path {
			pathParts[i] = starlark.String(p)
		}
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":     starlark.String("attr_path"),
			"variable": starlark.String(n.Variable),
			"path":     starlark.NewList(pathParts),
		})
	case *exprcheck.QNameExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":   starlark.String("qname"),
			"module": starlark.String(n.Module),
			"name":   starlark.String(n.Name),
			"sub":    starlark.String(n.Sub),
		})
	case *exprcheck.ParenExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("paren"),
			"inner": robustExprToStarlark(n.Inner),
		})
	case *exprcheck.IfThenElseExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("if"),
			"cond":  robustExprToStarlark(n.Cond),
			"then":  robustExprToStarlark(n.Then),
			"else_": robustExprToStarlark(n.Else),
		})
	case *exprcheck.ConstantRef:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("constant"),
			"qname": starlark.String(n.QName),
		})
	case *exprcheck.TokenExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":  starlark.String("token"),
			"token": starlark.String(n.Token),
			"arg":   starlark.String(n.Arg),
		})
	case *exprcheck.RecoveredExpr:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind":   starlark.String("recovered"),
			"source": starlark.String(n.SourceFragment),
			"reason": starlark.String(n.Reason),
		})
	default:
		return starlarkstruct.FromStringDict(starlark.String("expr"), starlark.StringDict{
			"kind": starlark.String("unknown"),
		})
	}
}

// xpathExpressionEntryToStarlark converts an XPathExpressionEntry to a Starlark struct.
func xpathExpressionEntryToStarlark(e XPathExpressionEntry) starlark.Value {
	return starlarkstruct.FromStringDict(starlark.String("xpath_expression"), starlark.StringDict{
		"id":                      starlark.String(e.ID),
		"document_type":           starlark.String(e.DocumentType),
		"document_id":             starlark.String(e.DocumentID),
		"document_qualified_name": starlark.String(e.DocumentQualifiedName),
		"component_type":          starlark.String(e.ComponentType),
		"component_id":            starlark.String(e.ComponentID),
		"component_name":          starlark.String(e.ComponentName),
		"xpath_expression":        starlark.String(e.XPathExpression),
		"target_entity":           starlark.String(e.TargetEntity),
		"referenced_entities":     starlark.String(e.ReferencedEntities),
		"is_parameterized":        starlark.Bool(e.IsParameterized),
		"usage_type":              starlark.String(e.UsageType),
		"module_name":             starlark.String(e.ModuleName),
	})
}
