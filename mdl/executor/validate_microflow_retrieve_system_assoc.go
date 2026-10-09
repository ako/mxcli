// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// retrieveSystemAssocRule is the rule ID for "an association retrieve over
// System.owner / System.changedBy".
const retrieveSystemAssocRule = "MDL-RETRIEVE02"

// checkRetrieveOverSystemAssociation flags `retrieve $u from $obj/System.owner`
// (and System.changedBy) — mendixlabs/mxcli#1358.
//
// owner and changedBy are not associations any domain model lists: an entity
// carries them as the HasOwner / HasChangedBy flags on its generalization root.
// XPath and page paths resolve them (#641, #1338), but a retrieve-by-association
// does not. Measured on mxbuild 11.12.2, entity declaring `owner: AutoOwner,
// changedBy: AutoChangedBy`:
//
//	retrieve $u from $task/System.owner;                               CE0136 "Retrieve object must specify the 'Entity' property."
//	retrieve $u from $task/System.changedBy;                           CE0136
//	retrieve $u from $task/System.Owner;                               CE1613 (the name itself does not resolve)
//	retrieve $u from System.User where [id = $task/System.owner] first; clean
//
// So the name resolves and the source still yields no entity — there is nothing
// to write that mxbuild accepts. The rule needs no project: no module but System
// can declare these names, and the entity flags do not change the outcome.
func (v *microflowValidator) checkRetrieveOverSystemAssociation(body []ast.MicroflowStatement) {
	forEachMicroflowStatement(body, func(s ast.MicroflowStatement) {
		r, ok := s.(*ast.RetrieveStmt)
		if !ok || r.StartVariable == "" || !strings.EqualFold(r.Source.Module, "System") {
			return
		}
		var member string
		switch strings.ToLower(r.Source.Name) {
		case "owner":
			member = "System.owner"
		case "changedby":
			member = "System.changedBy"
		default:
			return
		}
		v.addViolation(retrieveSystemAssocRule, linter.SeverityError,
			fmt.Sprintf("retrieve $%s from $%s/%s.%s: %s is not a modelled association, and a "+
				"retrieve by association over it has no entity — mxbuild rejects it with CE0136 "+
				"\"Retrieve object must specify the 'Entity' property.\"",
				r.Variable, r.StartVariable, r.Source.Module, r.Source.Name, member),
			fmt.Sprintf("Retrieve the user from the database instead: "+
				"`retrieve $%s from System.User where [id = $%s/%s] first;`",
				r.Variable, r.StartVariable, member))
	})
}
