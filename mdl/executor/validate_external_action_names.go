// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// externalActionNameRule is the rule ID for "`call external action` names an action
// the consumed service's contract does not have".
const externalActionNameRule = "MDL111"

// `call external action M.Client.Action` must name an action of the consumed
// service's contract, exactly. Measured on MxBuild 11.16.1 against a contract with
// one action, DoThing:
//
//	DoThing                  clean
//	NotThere                 CE7251 "Service 'M.Client' does not contain action 'NotThere'."
//	dothing                  CE7251   (action names are case-sensitive)
//
// with CE0117 on the output variable either way. check and exec accepted both: the
// builder matches the name case-insensitively to type the result, and writes it as
// typed.
//
// The contract is the one stored on the project's consumed service. A service the
// script creates, alters or drops is not judged (its contract is not the stored
// one), nor is one with no stored contract, nor anything without a project.
func ValidateExternalActionNames(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil || open == nil {
		return nil
	}
	changed := map[string]bool{}
	var calls []struct {
		flow ast.QualifiedName
		stmt *ast.CallExternalActionStmt
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateODataClientStmt:
			changed[strings.ToLower(s.Name.String())] = true
		case *ast.AlterODataClientStmt:
			changed[strings.ToLower(s.Name.String())] = true
		case *ast.DropODataClientStmt:
			changed[strings.ToLower(s.Name.String())] = true
		case *ast.CreateMicroflowStmt:
			name := s.Name
			forEachMicroflowStatement(s.Body, func(ms ast.MicroflowStatement) {
				if c, ok := ms.(*ast.CallExternalActionStmt); ok && c.ActionName != "" {
					calls = append(calls, struct {
						flow ast.QualifiedName
						stmt *ast.CallExternalActionStmt
					}{name, c})
				}
			})
		}
	}
	if len(calls) == 0 {
		return nil
	}
	contracts := projectConsumedContracts(open)
	if len(contracts) == 0 {
		return nil
	}

	var out []linter.Violation
	for _, c := range calls {
		svc := qualifyIn(c.flow.Module, c.stmt.ServiceName.String())
		key := strings.ToLower(svc)
		if changed[key] {
			continue
		}
		actions, ok := contracts[key]
		if !ok {
			continue
		}
		if actions[c.stmt.ActionName] {
			continue
		}
		names := make([]string, 0, len(actions))
		for a := range actions {
			names = append(names, a)
		}
		sort.Strings(names)
		fix := fmt.Sprintf("%s has no actions.", svc)
		if len(names) > 0 {
			fix = fmt.Sprintf("Use one of %s's actions, spelled exactly (names are case-sensitive): %s.", svc, strings.Join(names, ", "))
		}
		out = append(out, linter.Violation{
			RuleID:   externalActionNameRule,
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("microflow %s: `call external action %s.%s` names an action the service's contract does not have — "+
				"mxbuild reports CE7251 \"Service '%s' does not contain action '%s'\"",
				c.flow, svc, c.stmt.ActionName, svc, c.stmt.ActionName),
			Location:   linter.Location{Module: c.flow.Module, DocumentType: "microflow", DocumentName: c.flow.Name},
			Suggestion: fix,
		})
	}
	return out
}

// projectConsumedContracts is the action names of each project consumed service
// that stores a contract, keyed by lower-cased qualified name. A service whose
// contract does not parse is left out, so it is not judged.
func projectConsumedContracts(open func() backend.FullBackend) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	b := open()
	if b == nil {
		return out
	}
	modules, err := b.ListModules()
	if err != nil {
		return out
	}
	names := map[string]string{}
	for _, m := range modules {
		if m != nil {
			names[string(m.ID)] = m.Name
		}
	}
	services, err := b.ListConsumedODataServices()
	if err != nil {
		return out
	}
	for _, svc := range services {
		if svc == nil || svc.Metadata == "" || names[string(svc.ContainerID)] == "" {
			continue
		}
		doc, err := types.ParseEdmx(svc.Metadata)
		if err != nil || doc == nil {
			continue
		}
		actions := map[string]bool{}
		for _, a := range doc.Actions {
			if a != nil && a.Name != "" {
				actions[a.Name] = true
			}
		}
		out[strings.ToLower(names[string(svc.ContainerID)]+"."+svc.Name)] = actions
	}
	return out
}
