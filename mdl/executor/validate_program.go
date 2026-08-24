// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateProgram runs every semantic check mxcli can make from the parsed
// script alone, plus the few that additionally consult a project when one is
// given. It is the single definition of "what `mxcli check` checks".
//
// It exists as one function because it has two callers that must not diverge:
// `mxcli check`, which reports the violations, and `mxcli exec`, which refuses
// to apply a script whose violations include an error. Those were previously
// unconnected — `check` held this list inline and `exec` ran none of it — so a
// script `check` rejected was applied by `exec` anyway (mxcli-banking findings,
// slice 2: "a page with an invalid widget property was written to the model").
// Keeping the list in one place is what makes "check before exec" enforceable
// rather than a convention.
//
// projectPath may be empty; the checks that need a project skip themselves.
// Parse errors are the caller's business — this operates on a built program.
func ValidateProgram(prog *ast.Program, projectPath string) []linter.Violation {
	// Statement-level checks that need no project connection.
	var violations []linter.Violation
	securityEnabled := programEnablesSecurity(prog)
	for _, stmt := range prog.Statements {
		// Check enumeration values for reserved words
		if enumStmt, ok := stmt.(*ast.CreateEnumerationStmt); ok {
			violations = append(violations, ValidateEnumeration(enumStmt)...)
		}
		// Check entity attributes for reserved system names
		if entityStmt, ok := stmt.(*ast.CreateEntityStmt); ok {
			violations = append(violations, ValidateEntity(entityStmt)...)
		}
		// Apply the same per-attribute checks to ALTER ENTITY ADD ATTRIBUTE
		if alterStmt, ok := stmt.(*ast.AlterEntityStmt); ok {
			violations = append(violations, ValidateAlterEntity(alterStmt)...)
		}
		// An association carries the same pair of contradictory guards (MDL067).
		if assocStmt, ok := stmt.(*ast.CreateAssociationStmt); ok {
			violations = append(violations, validateIdempotencyGuard(
				assocStmt.CreateOrModify, assocStmt.IfNotExists, "association", assocStmt.Name.String())...)
		}
		// A user role with no System module role cannot sign in (CE0156) — but
		// only once security is on, which the script may say itself.
		if roleStmt, ok := stmt.(*ast.CreateUserRoleStmt); ok {
			violations = append(violations, ValidateUserRoleSystemModuleRole(roleStmt, securityEnabled)...)
		}
		// A page with parameters and a Url must name each parameter in it (CE5601).
		if pageStmt, ok := stmt.(*ast.CreatePageStmtV3); ok {
			violations = append(violations, ValidatePageURLParameters(pageStmt)...)
		}
		// Check microflow body for common issues
		if mfStmt, ok := stmt.(*ast.CreateMicroflowStmt); ok {
			violations = append(violations, ValidateMicroflow(mfStmt)...)
		}
		// Check workflow for constructs MxBuild rejects (missing page,
		// single-outcome-with-activities, invalid decision outcome names)
		if wfStmt, ok := stmt.(*ast.CreateWorkflowStmt); ok {
			violations = append(violations, ValidateWorkflow(wfStmt)...)
		}
		// Check GRANT for member rights Mendix cannot store
		if grantStmt, ok := stmt.(*ast.GrantEntityAccessStmt); ok {
			violations = append(violations, ValidateGrantEntityAccess(grantStmt)...)
		}
		// Check typed ALTER SETTINGS / CREATE CONFIGURATION property values
		if setStmt, ok := stmt.(*ast.AlterSettingsStmt); ok {
			violations = append(violations, ValidateSettings(setStmt)...)
		}
		if cfgStmt, ok := stmt.(*ast.CreateConfigurationStmt); ok {
			violations = append(violations, ValidateCreateConfiguration(cfgStmt)...)
		}
		// Check database connection credentials: a literal where Mendix
		// stores a constant reference writes an UNOPENABLE project.
		if dbStmt, ok := stmt.(*ast.CreateDatabaseConnectionStmt); ok {
			violations = append(violations, ValidateDatabaseConnection(dbStmt)...)
		}
		// Check view entity OQL
		if viewStmt, ok := stmt.(*ast.CreateViewEntityStmt); ok {
			if viewStmt.Query.RawQuery != "" {
				violations = append(violations, ValidateOQLSyntax(viewStmt.Query.RawQuery)...)
				violations = append(violations, ValidateOQLTypes(viewStmt.Query.RawQuery, viewStmt.Attributes)...)
			}
		}
	}

	// Check for intra-script duplicate definitions (CREATE X … CREATE X without DROP)
	violations = append(violations, CheckScriptDuplicates(prog)...)

	// Validate design properties against the project's theme registry
	// (themesource design-properties.json) — flags unknown keys and invalid
	// option values, listing the allowed values. Only runs with --project.
	violations = append(violations, ValidateDesignProperties(prog, projectPath)...)

	// Validate pluggable widget properties against widget definitions —
	// catches typos in property keys before MxBuild does. Uses built-in
	// definitions alone when no project is given; with --project, also
	// loads project-installed .def.json files for full coverage.
	violations = append(violations, ValidateWidgetProperties(prog, projectPath)...)

	// Warn (MPR010) when an edit/new form (a parameter-bound DataView) is not
	// wrapped in a layout grid — its label/input widths only render correctly
	// inside a layoutgrid. Same rule as the MPR010 lint rule, surfaced at
	// authoring time on the AST.
	violations = append(violations, ValidatePageLayoutGrid(prog)...)

	// Flag control-bar buttons that pass $currentObject — a control bar is
	// not row-scoped, so the argument is unbound (CE1571) at build time.
	violations = append(violations, ValidatePageButtonContext(prog)...)

	// Flag a database-connection TYPE Studio Pro does not offer. mxcli writes
	// the string through and mxbuild does not check it, so a wrong value
	// builds green and simply does not connect.
	violations = append(violations, ValidateDatabaseConnectionType(prog)...)

	// Flag OData property names nothing below will act on. The grammar takes
	// any `name: value` pair, so a typo used to be discarded in silence and
	// the model quietly lacked what the author asked for.
	violations = append(violations, ValidateODataProperties(prog)...)

	// Flag a microflow-backed OData resource whose read microflow cannot keep
	// the promises the service makes for it. A read microflow has no
	// System.HttpResponse parameter, so it cannot answer 400 — its contract
	// has to be declared correctly up front, and nothing else checks that.
	violations = append(violations, ValidateODataReadContract(prog)...)

	// Flag `authentication microflow` with no microflow named. The grammar
	// makes the name optional, so this parses and executes into a service
	// Mendix refuses to build (CE0333).
	violations = append(violations, ValidateODataAuth(prog)...)

	// Flag two service shapes mxbuild rejects — a Path that breaks its
	// slash rules, and the PublishAssociations mode whose name invites
	// exactly the wrong value. A Path with no slash at all is the reason
	// this is worth a check: mxbuild throws out of its own validator with
	// no error code, so there is nothing to look up.
	violations = append(violations, ValidateODataServiceShape(prog)...)

	// Flag a page whose widgets point at a page created further down the same
	// script. `exec` resolves page references in statement order and is not
	// transactional, so this fails after earlier statements are already
	// written. --references catches it too, but the ordering needs no project
	// when the target is created by a plain CREATE (#9).
	violations = append(violations, ValidateScriptPageOrder(prog)...)

	// Flag the same ordering mistake for the other reference kinds the executor
	// resolves at write time — a flow's parameter and return types, an entity
	// attribute's enumeration, an association's endpoints, a CALL, and a GRANT.
	// `exec` already produces the diagnosis once a statement has failed; this
	// says it before the first write (#955).
	violations = append(violations, ValidateScriptDefinitionOrder(prog)...)

	// Flag a document-access GRANT naming a role from another module — Mendix
	// rejects it with CE0148. Needs no project, so it runs here rather than
	// under --references, where it would only fire with -p (#836).
	violations = append(violations, ValidateGrantRoles(prog)...)

	// Flag an export mapping value whose member is a nested path — an export has
	// to produce the intermediate node, so Mendix rejects it with CE5015. The
	// answer is in the statement, so it runs here rather than under --references
	// (#927).
	violations = append(violations, ValidateExportMappingMembers(prog)...)

	// Flag a REST client operation whose Body/Response mapping clause has no
	// `{ ... }` body — Mendix cannot reference a mapping document from an
	// operation, so the mapping would be dropped in silence (#843).
	violations = append(violations, ValidateRestClientMappings(prog)...)

	// Flag a scheduled event whose Repeat and fields disagree (a Multiplier on
	// a Daily repeat, an HourOfDay of 99). Decidable from the statement, so it
	// runs here rather than at exec, where the script would already have
	// passed check.
	violations = append(violations, ValidateScheduledEvents(prog)...)

	return violations
}
