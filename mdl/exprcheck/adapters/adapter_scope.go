// SPDX-License-Identifier: Apache-2.0

package adapters

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/exprcheck"
)

// buildVarEntityScope walks a microflow body and records every variable
// known to hold an entity instance, mapping varName → entity QN.
//
// Sources covered:
//   - CreateObjectStmt (Variable ← EntityType)
//   - RetrieveStmt with $var = retrieve … from <Entity> (Variable ← EntityType)
//
// The map is best-effort. An empty entry means "unknown" and the caller
// should fall back to a slot path without entity.attr enrichment.
func buildVarEntityScope(body []ast.MicroflowStatement) map[string]string {
	scope := map[string]string{}
	var walk func([]ast.MicroflowStatement)
	walk = func(stmts []ast.MicroflowStatement) {
		for _, s := range stmts {
			switch n := s.(type) {
			case *ast.CreateObjectStmt:
				if n.Variable != "" {
					scope[n.Variable] = n.EntityType.String()
				}
			case *ast.RetrieveStmt:
				if n.Variable != "" && n.StartVariable == "" && n.Source.Name != "" {
					scope[n.Variable] = n.Source.String()
				}
			case *ast.IfStmt:
				walk(n.ThenBody)
				walk(n.ElseBody)
			case *ast.WhileStmt:
				walk(n.Body)
			case *ast.LoopStmt:
				walk(n.Body)
			}
		}
	}
	walk(body)
	return scope
}

// entityScope adapts a variable→entity map plus an association resolver to
// exprcheck.EntityScope, which is what lets `$Order/Customer/Name` be typed.
type entityScope struct {
	vars  map[string]string
	assoc associationResolver
}

// associationResolver is the association half of the model. exprcatalog.Reader
// satisfies it, and so does mdl/xpathrefs' Model — the two resolvers answer the
// same question and the signature is deliberately shared.
type associationResolver interface {
	AssociationTarget(assocQN, fromEntityQN string) (string, bool)
}

var _ exprcheck.EntityScope = entityScope{}

func (e entityScope) VariableEntity(name string) (string, bool) {
	qn, ok := e.vars[strings.TrimPrefix(name, "$")]
	return qn, ok && qn != ""
}

func (e entityScope) AssociationTarget(assocQN, fromEntityQN string) (string, bool) {
	if e.assoc == nil {
		return "", false
	}
	return e.assoc.AssociationTarget(assocQN, fromEntityQN)
}

// addParamEntities records the entity a parameter holds.
//
// buildVarEntityScope walks only the body, so it sees a variable a CREATE or
// RETRIEVE introduced and misses every parameter — and a microflow that takes
// its object as a parameter is the ordinary case, not an edge one.
//
// The visitor cannot tell `$P: Mod.Person` from an enumeration-typed parameter:
// a bare qualified name parses as TypeEnumeration with EnumRef set (see
// CLAUDE.md). Both spellings are recorded rather than guessed between — a name
// that turns out to be an enumeration simply resolves no attributes, so the
// wrong guess costs nothing.
func addParamEntities(scope map[string]string, params []ast.MicroflowParam) {
	for _, p := range params {
		if p.Name == "" {
			continue
		}
		switch {
		case p.Type.EntityRef != nil:
			scope[p.Name] = p.Type.EntityRef.String()
		case p.Type.Kind == ast.TypeEnumeration && p.Type.EnumRef != nil:
			scope[p.Name] = p.Type.EnumRef.String()
		}
	}
}
