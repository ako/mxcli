// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// An association is stored in ONE module's domain model, and Mendix's storage
// shape decides which: the FROM entity's. Both stored forms say so —
// `DomainModels$Association` holds two element IDs (both local to the unit), and
// `DomainModels$CrossAssociation` holds a local `ParentPointer` plus a BY_NAME
// `ChildRef`. There is no by-name PARENT in either. A remote FROM entity is
// therefore not a feature waiting to be written; it is unrepresentable.
//
// Written anyway, the association's ParentPointer is an element ID belonging to
// a different unit, and the project stops LOADING — so this is the same failure
// class as ValidateDatabaseConnection, not a build error:
//
//	KeyNotFoundException: The given key '4a25f08b-…' was not present in the
//	dictionary
//	  at StreamingBsonUnitReader.ResolvePostponedProperties()
//
// Neither `mxcli check` nor `mxcli exec` saw it (check passed, exec reported
// "Created association"), and mxbuild reports it as a stack trace naming no
// document — the reporter located the culprit by scanning every .mxunit for the
// raw GUID bytes. Measured on 11.13.0, both directions, with the supported
// shape as the control.
//
// It is NOT specific to System. `from Administration.Account to
// MyFirstModule.Department` fails identically; System.User is simply the
// remote parent people reach for first. A guard keyed on the System module
// would pass the general case straight through.
//
// Refusing is the whole fix — there is nothing to write instead. The three
// remedies are the author's to choose between, so the message names all three
// rather than guessing: reverse the direction, declare the association in the
// FROM entity's own module, or model the relationship as a join entity (which
// is what the reporter ended up doing, and what a many-to-many between two
// modules wants anyway).
const associationParentModuleRule = "MDL070"

// ValidateAssociationModules reports an association whose FROM entity lives in a
// module other than the one the association is declared in.
//
// The comparison is case-insensitive because module lookup is: `m.A` and `M.A`
// name one module, and refusing that pair would be a false positive on a
// statement that writes a perfectly good association.
func ValidateAssociationModules(stmt *ast.CreateAssociationStmt) []linter.Violation {
	if stmt == nil {
		return nil
	}
	// An unqualified endpoint means "this association's module" (the executor
	// defaults it that way), so it cannot be remote.
	//
	// An unqualified association NAME has no module to compare against. Skipping
	// it is safe rather than a hole: execCreateAssociation refuses it first, with
	// "module name is required: objects must be created within a module".
	// Measured — `create association User_Department from System.User to
	// MyFirstModule.Department` reaches that refusal and the project stays at 0
	// errors. (`check` does pass it, which is a check/exec divergence worth
	// closing on its own; it errs in the safe direction.)
	if stmt.Name.Module == "" || stmt.Parent.Module == "" {
		return nil
	}
	if strings.EqualFold(stmt.Name.Module, stmt.Parent.Module) {
		return nil
	}

	return []linter.Violation{{
		RuleID:   associationParentModuleRule,
		Severity: linter.SeverityError,
		Message: fmt.Sprintf(
			"association %s is declared in module %s but its FROM entity %s lives in %s — "+
				"Mendix stores an association in the module of its FROM entity, so this writes a "+
				"dangling pointer and the project can no longer be opened (KeyNotFoundException "+
				"while loading, not a build error)",
			stmt.Name.String(), stmt.Name.Module, stmt.Parent.String(), stmt.Parent.Module),
		Location: linter.Location{
			DocumentType: "association",
			DocumentName: stmt.Name.String(),
		},
		Suggestion: fmt.Sprintf(
			"Reverse it — `from %s to %s` — or declare it in %s, or model the relationship as a "+
				"join entity in %s with a reference to each side (the usual answer when the FROM "+
				"entity is in System or a Marketplace module, which must not be written to).",
			stmt.Child.String(), stmt.Parent.String(), stmt.Parent.Module, stmt.Name.Module),
	}}
}
