// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// inheritanceSplitRule is the rule ID for "a `split type` branch the build rejects".
const inheritanceSplitRule = "MDL109"

// A `split type $V` must have a branch for $V's own entity, may only branch on that
// entity or a specialization of it, and may name each once. Measured on MxBuild
// 11.16.1, one project per case (Animal <- Dog <- Puppy, Animal <- Cat):
//
//	every type + (empty)                              clean
//	a subtype left out (Cat, or Puppy)                clean   (only the base is required)
//	the subject's entity left out                     CE0090 "The 'M.Animal' value should be configured for an outgoing flow."
//	a branch on an unrelated entity                   CE0774 "Value is not entity 'M.Animal' or a specialization."
//	a branch on the subject's generalization          CE0774  (subject Dog, branch Animal)
//	a branch on a sibling (subject Dog, branch Cat)   CE0774
//	the same entity twice                             CE0771 "Value is already handled by another sequence flow."
//	no (empty) branch                                 clean   (the builder always adds it)
//
// check and exec accepted every rejected case. Older records (11.13.0) said every
// subtype needs a branch; on 11.16.1 only the subject's own entity does, so a
// missing subtype is not reported.
//
// CE0089 ("The '(empty)' value should be configured"), which this was started from,
// is not reachable from MDL: the builder emits the (empty) flow unconditionally.
//
// The subject's entity comes from the flow's parameters and the objects it
// retrieves (`first`) or creates; a subject typed any other way is not judged. A
// branch entity's generalizations come from the script, then the project; one
// whose chain cannot be read completely is not judged for CE0774 (an incomplete
// chain can say yes, never no).
func ValidateInheritanceSplits(prog *ast.Program, open func() backend.FullBackend) []linter.Violation {
	if prog == nil {
		return nil
	}
	scriptGen := map[string]string{} // lower-cased entity QN -> its generalization QN, as written
	scriptEntities := map[string]bool{}
	for _, stmt := range prog.Statements {
		if e, ok := stmt.(*ast.CreateEntityStmt); ok {
			key := strings.ToLower(e.Name.String())
			scriptEntities[key] = true
			if e.Generalization != nil {
				scriptGen[key] = qualifyIn(e.Name.Module, e.Generalization.String())
			}
		}
	}

	var lookup entityLookupBackend
	opened := false
	projectBackend := func() entityLookupBackend {
		if !opened {
			opened = true
			if open != nil {
				if b := open(); b != nil {
					if lb, ok := b.(entityLookupBackend); ok {
						lookup = lb
					}
				}
			}
		}
		return lookup
	}
	// chain is entity followed by its generalizations, nearest first, and whether
	// the walk reached the root.
	chain := func(entity string) ([]string, bool) {
		var out []string
		seen := map[string]bool{}
		for cur := entity; cur != ""; {
			key := strings.ToLower(cur)
			if seen[key] {
				return out, false
			}
			seen[key] = true
			out = append(out, cur)
			if scriptEntities[key] {
				cur = scriptGen[key]
				continue
			}
			rest, complete := generalizationChainFor(projectBackend(), cur)
			if len(rest) > 1 {
				out = append(out, rest[1:]...)
			}
			return out, complete
		}
		return out, true
	}

	var out []linter.Violation
	judge := func(kind string, name ast.QualifiedName, params []ast.MicroflowParam, body []ast.MicroflowStatement) {
		vars := splitSubjectEntities(params, body)
		forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
			stmt, ok := s.(*ast.InheritanceSplitStmt)
			if !ok {
				return
			}
			base := vars[stmt.Variable]
			if base == "" {
				return
			}
			base = qualifyIn(name.Module, base)
			loc := linter.Location{Module: name.Module, DocumentType: kind, DocumentName: name.Name}
			report := func(code, msg, fix string) {
				out = append(out, linter.Violation{
					RuleID: inheritanceSplitRule, Severity: linter.SeverityError, Location: loc,
					Message:    fmt.Sprintf("%s %s: `split type $%s` %s — mxbuild reports %s", kind, name, stmt.Variable, msg, code),
					Suggestion: fix,
				})
			}
			seen := map[string]bool{}
			hasBase := false
			for _, c := range stmt.Cases {
				entity := qualifyIn(name.Module, c.Entity.String())
				key := strings.ToLower(entity)
				if seen[key] {
					report("CE0771", fmt.Sprintf("has two branches for %s", entity),
						"Keep one branch per entity: the build routes each type to exactly one flow.")
					continue
				}
				seen[key] = true
				if strings.EqualFold(entity, base) {
					hasBase = true
					continue
				}
				ch, complete := chain(entity)
				related := false
				for _, e := range ch {
					if strings.EqualFold(e, base) {
						related = true
						break
					}
				}
				if !related && complete {
					report("CE0774", fmt.Sprintf("branches on %s, which is not %s or a specialization of it", entity, base),
						fmt.Sprintf("Branch only on %s or an entity that extends it; a generalization or a sibling can never be the object's type.", base))
				}
			}
			if !hasBase {
				report("CE0090", fmt.Sprintf("has no branch for %s, the subject's own entity", base),
					fmt.Sprintf("Add `when %s then …`: it is taken for an object of exactly that type, and the build requires it. "+
						"The `(empty)` branch is the null object and does not count.", base))
			}
		})
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateMicroflowStmt:
			judge("microflow", s.Name, s.Parameters, s.Body)
		case *ast.CreateNanoflowStmt:
			judge("nanoflow", s.Name, s.Parameters, s.Body)
		}
	}
	return out
}

// splitSubjectEntities is the entity each object variable of a flow holds: its
// entity-typed parameters, and the objects it retrieves with `first` or creates.
func splitSubjectEntities(params []ast.MicroflowParam, body []ast.MicroflowStatement) map[string]string {
	vars := map[string]string{}
	for _, p := range params {
		if p.Type.Kind == ast.TypeEntity && p.Type.EntityRef != nil {
			vars[p.Name] = p.Type.EntityRef.String()
		}
	}
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		switch st := s.(type) {
		case *ast.RetrieveStmt:
			if st.First && st.Variable != "" && st.StartVariable == "" && st.Source.Name != "" {
				vars[st.Variable] = st.Source.String()
			}
		case *ast.CreateObjectStmt:
			if st.Variable != "" && st.EntityType.Name != "" {
				vars[st.Variable] = st.EntityType.String()
			}
		}
	})
	return vars
}
