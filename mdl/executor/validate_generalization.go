// SPDX-License-Identifier: Apache-2.0

// Package executor - EXTENDS target validation for CREATE ENTITY.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// validateEntityGeneralization checks that an entity's EXTENDS target resolves.
//
// It never did, which is how `extends System.Thumbnail` survived long enough to
// become #972: mxcli reported "All references valid" for a name the modeler does
// not have. Two failure sizes, both measured on mxbuild 11.13.0:
//
//   - A qualified name that resolves to nothing is stored verbatim and reported
//     at build time as CE1613 "The selected entity … no longer exists".
//   - An UNQUALIFIED name is stored as a bare word, and Mendix cannot build an
//     entity identifier from it — the project does not load at all, so mx check
//     dies before validating anything. Same shape as the bare attribute name in
//     #973.
//
// Resolution is against the whole script plus the project, not the statements
// seen so far: a generalization is resolved lazily, so `create entity A extends
// B; create entity B;` is correct and executes correctly (see eagerDefRefs,
// which deliberately does not treat it as an ordering error).
func validateEntityGeneralization(ctx *ExecContext, s *ast.CreateEntityStmt, sc *scriptContext) error {
	gen := s.Generalization
	if gen == nil || gen.Name == "" {
		return nil
	}

	if err := checkGeneralizationQualified(s.Name, *gen); err != nil {
		return err
	}

	genQN := gen.String()
	if sc != nil && sc.entities[genQN] {
		return nil // created by this script, in any order
	}

	// The module has to exist before "the entity is not in it" means anything.
	if sc == nil || !sc.modules[gen.Module] {
		if _, err := findModule(ctx, gen.Module); err != nil {
			return mdlerrors.NewNotFoundMsg("module", gen.Module, fmt.Sprintf(
				"entity %s extends %s, but module %s does not exist — "+
					"the generalization is stored by name, so mxbuild reports it as "+
					"CE1613 \"The selected entity '%s' no longer exists\"",
				s.Name.String(), genQN, gen.Module, genQN))
		}
	}

	if buildEntityQualifiedNames(ctx)[genQN] {
		return nil
	}
	return mdlerrors.NewNotFoundMsg("entity", genQN, fmt.Sprintf(
		"entity %s extends %s, which does not exist — "+
			"the generalization is stored by name, so mxbuild reports it as "+
			"CE1613 \"The selected entity '%s' no longer exists\"",
		s.Name.String(), genQN, genQN))
}

// generalizationIsBare reports an EXTENDS target written without a module.
func generalizationIsBare(gen *ast.QualifiedName) bool {
	return gen != nil && gen.Name != "" && gen.Module == ""
}

const bareGeneralizationWhy = "MDL has no implicit module context, and a bare name is stored as-is, " +
	"which Mendix cannot load at all (\"An error occurred when trying to set the " +
	"'Generalization' property\") — mx check dies before validating anything"

// checkGeneralizationQualified rejects a bare EXTENDS target.
//
// Three callers share it, because this one case has to be caught everywhere:
// ValidateEntity (MDL069, so `mxcli check` reports it with no project at hand),
// the reference check, and the executor — `exec --no-check` skips the pre-flight
// and this is the case that writes a file Mendix cannot open.
func checkGeneralizationQualified(entity, gen ast.QualifiedName) error {
	if !generalizationIsBare(&gen) {
		return nil
	}
	return mdlerrors.NewValidationf(
		"entity %s extends %q, which is not a qualified name — %s. Write it as Module.%s",
		entity.String(), gen.Name, bareGeneralizationWhy, gen.Name)
}

// validateBareGeneralization is MDL069, the project-less half of the same rule.
// A bare EXTENDS target is a static property of the statement, so `mxcli check`
// reports it with no project to resolve against.
func validateBareGeneralization(stmt *ast.CreateEntityStmt) []linter.Violation {
	if !generalizationIsBare(stmt.Generalization) {
		return nil
	}
	name := stmt.Generalization.Name
	return []linter.Violation{{
		RuleID:   "MDL069",
		Severity: linter.SeverityError,
		Message: fmt.Sprintf("entity %s extends %q, which is not a qualified name — %s",
			stmt.Name.String(), name, bareGeneralizationWhy),
		Suggestion: fmt.Sprintf("Qualify it: `extends Module.%s`. If the parent is in this entity's own "+
			"module, that is still `extends %s.%s`.", name, stmt.Name.Module, name),
		Location: linter.Location{DocumentType: "entity", DocumentName: stmt.Name.String()},
	}}
}
