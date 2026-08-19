// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for export mapping value members.
//
// An IMPORT mapping can collapse levels — `Attr = customer/name` reads a leaf
// several levels below the object element it belongs to, with no entity for the
// levels in between. An EXPORT mapping cannot: it has to PRODUCE the
// intermediate node, so something must map it.
//
// Measured on mxbuild 11.13 with a three-way control: the same collapsed member
// in an import mapping is 0 errors, the same export mapping with only top-level
// members is 0 errors, and the collapsed export is CE5015 "There is no child
// mapping matching schema element". See issue #927.
package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// ValidateExportMappingMembers reports (MDL-MAP01) an export mapping value whose
// member is a `/`-separated path.
//
// This lives in the no-project pass rather than the --references pass on
// purpose, for the same reason as ValidateGrantRoles: the answer is in the
// statement itself, so requiring -p would withhold something mxcli can always
// tell the author. It also means a plain `mxcli check` catches it — which is
// what lets the negative fixture be a .fail.mdl at all.
func ValidateExportMappingMembers(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		s, ok := stmt.(*ast.CreateExportMappingStmt)
		if !ok || s.RootElement == nil {
			continue
		}
		for _, member := range nestedExportMembers(s.RootElement.Children) {
			level := strings.Split(member, "/")[0]
			out = append(out, linter.Violation{
				RuleID:   "MDL-MAP01",
				Severity: linter.SeverityError,
				Message:  nestedExportMemberError(member).Error(),
				Location: linter.Location{
					Module:       s.Name.Module,
					DocumentType: "export mapping",
					DocumentName: s.Name.Name,
				},
				Suggestion: "Give " + level + " its own element: Association/Module.Entity as " + level + " { ... }",
			})
		}
	}
	return out
}

// nestedExportMembers collects every `/`-separated VALUE member in the tree. An
// object element's JsonName is the key it maps, which is always a direct child,
// so only value elements (no Entity) are considered.
func nestedExportMembers(elems []*ast.ExportMappingElementDef) []string {
	var out []string
	for _, e := range elems {
		if e == nil {
			continue
		}
		if e.Entity == "" && strings.Contains(e.JsonName, "/") {
			out = append(out, e.JsonName)
		}
		out = append(out, nestedExportMembers(e.Children)...)
	}
	return out
}
