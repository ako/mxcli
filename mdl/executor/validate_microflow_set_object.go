// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// setObjectRule is the rule ID for "set on an object variable".
const setObjectRule = "MDL-SET01"

// checkSetOnObjectVariable flags `set $X = …` where $X is a single OBJECT.
// Mendix has no action that reassigns an object variable: Change variable
// takes only a primitive, and mxbuild answers CE7247 "Variable 'X' does not
// have a primitive type" (mendixlabs/mxcli#1323, measured on 11.14.0).
//
// This is plain check's half, which runs without a project, so it judges only
// the object producers it can see in the text: an entity parameter, a create
// object, a database `retrieve … first`, a loop iterator, a cast, a head. An
// association retrieve is an object or a list by the association's type and
// direction, which needs the project — check --references and exec decide that
// one (flowBuilder.refuseSetOnObject), and this rule leaves it alone rather
// than guess.
func (v *microflowValidator) checkSetOnObjectVariable(params []ast.MicroflowParam, body []ast.MicroflowStatement) {
	// objects maps each variable currently known to hold one object to its entity.
	objects := map[string]string{}
	// isParam marks the object parameters: mxbuild words their CE7247
	// differently ("Parameter 'A' cannot be changed."), measured on 11.14.0.
	isParam := map[string]bool{}
	for _, p := range params {
		if p.Type.EntityRef != nil && p.Type.Kind != ast.TypeListOf {
			objects[p.Name] = p.Type.EntityRef.String()
			isParam[p.Name] = true
		}
	}
	lists := map[string]string{}
	for _, p := range params {
		if p.Type.EntityRef != nil && p.Type.Kind == ast.TypeListOf {
			lists[p.Name] = p.Type.EntityRef.String()
		}
	}

	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		if set, ok := s.(*ast.MfSetStmt); ok && !strings.Contains(set.Target, "/") {
			name := strings.TrimPrefix(set.Target, "$")
			if entity, ok := objects[name]; ok {
				rejection := fmt.Sprintf("CE7247 \"Variable '%s' does not have a primitive type\"", name)
				if isParam[name] {
					rejection = fmt.Sprintf("CE7247 \"Parameter '%s' cannot be changed\"", name)
				}
				v.addViolation(setObjectRule, linter.SeverityError,
					fmt.Sprintf("cannot set object variable '$%s' (%s): Mendix has no action that reassigns an "+
						"object variable — Change variable takes only a primitive, and mxbuild rejects it with "+
						"%s.", name, entity, rejection),
					fmt.Sprintf("Return the new object from a sub-microflow instead (recursion for a chain walk), "+
						"retrieve it into a new variable, or change the object's members with `change $%s (…)`.", name))
			}
		}

		// Rebinding first: a name a statement produces is whatever that
		// statement makes it, and nothing it was before.
		for _, p := range statementProducedVars(s) {
			delete(objects, p.name)
			delete(lists, p.name)
			delete(isParam, p.name)
		}
		switch st := s.(type) {
		case *ast.CreateObjectStmt:
			if st.Variable != "" && st.EntityType.Module != "" {
				objects[st.Variable] = st.EntityType.String()
			}
		case *ast.RetrieveStmt:
			if st.Variable != "" && st.StartVariable == "" && st.Source.Module != "" {
				if st.First {
					objects[st.Variable] = st.Source.String()
				} else {
					lists[st.Variable] = st.Source.String()
				}
			}
		case *ast.CreateListStmt:
			if st.Variable != "" && st.EntityType.Module != "" {
				lists[st.Variable] = st.EntityType.String()
			}
		case *ast.LoopStmt:
			if entity, ok := lists[st.ListVariable]; ok && st.LoopVariable != "" {
				objects[st.LoopVariable] = entity
			}
		case *ast.ListOperationStmt:
			// head is list-only; find is also the String function, so it is left out.
			if entity, ok := lists[st.InputVariable]; ok && st.OutputVariable != "" {
				switch st.Operation {
				case ast.ListOpHead:
					objects[st.OutputVariable] = entity
				case ast.ListOpFilter, ast.ListOpSort, ast.ListOpTail:
					lists[st.OutputVariable] = entity
				}
			}
		}
	})
}
