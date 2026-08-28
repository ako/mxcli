// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL071 warns that a name will be unusable from OQL, at the point the name is
// CHOSEN rather than at the point a view finally uses it.
//
// MDL032 already reports the collision, but only when the view's OQL is written
// — and by then the name is everywhere. In the reported case (ako/mxcli-captrack
// #1) `Month`, `Year` and `Quarter` had reached 121 places across nine
// generators and the database columns before the first view needed them; the
// rename was mechanical and five hours late. The check costs nothing at CREATE
// and is worth a warning there even though most entities never appear in a view.
//
// It is a WARNING, not an error, and that is the whole design: the name is
// perfectly legal Mendix, and an app whose entities never reach a view is
// correct as written. Making it an error would refuse working models.
//
// Both an ATTRIBUTE and an ENTITY name are affected — measured on 11.13.0, each
// on its own:
//
//	attribute Month  →  CE0174 "The 'Month' part is incomplete or incorrect.
//	                    You could use here: ASTERISK, AT_SIGN, OPEN_QUOTE, or
//	                    IDENTIFIER."
//	entity    Year   →  CE0174 "The 'Year' part is incomplete or incorrect. …"
//
// The mechanism is the same for both: OQL reads the bare word as the keyword,
// and mxcli cannot escape it because the OQL grammar has no quoted-identifier
// form. Quoting in MDL does not help either — that escapes MDL parser keywords,
// which is a different grammar (see the "Quoting Escapes Parser Keywords" rule).
//
// The word list is oqlReservedWords, shared with MDL032 so the two cannot drift.

// oqlReservedNameSet is the lookup form of oqlReservedWords. Built from the same
// slice rather than restated, so a word added for MDL032 is covered here too.
var oqlReservedNameSet = func() map[string]bool {
	m := make(map[string]bool, len(oqlReservedWords))
	for _, w := range oqlReservedWords {
		m[strings.ToLower(w)] = true
	}
	return m
}()

// isOQLReservedName reports whether a bare model name collides with an OQL
// keyword. Case-insensitive: OQL's grammar does not care how it was typed.
func isOQLReservedName(name string) bool {
	return oqlReservedNameSet[strings.ToLower(strings.Trim(strings.TrimSpace(name), `"`))]
}

// oqlReservedNameViolation builds the MDL071 warning for one name. kind is the
// noun used in the message ("attribute" / "entity"), so the suggestion can name
// the right thing to rename — MDL032 cannot tell the two apart (its regex sees
// only "after a dot or after AS") and says "attribute" for both.
func oqlReservedNameViolation(kind, name, entityName string) linter.Violation {
	return linter.Violation{
		RuleID:   "MDL071",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf(
			"%s '%s' is an OQL reserved word — it is valid Mendix, but any view entity that "+
				"references it fails to build with CE0174, and mxcli cannot quote it",
			kind, name),
		Location: linter.Location{DocumentType: "entity", DocumentName: entityName},
		Suggestion: fmt.Sprintf(
			"Safe to keep if this %s will never appear in a view entity's OQL. Otherwise rename it "+
				"now — e.g. '%sValue' or a domain term like 'Reporting%s' — while it has few "+
				"references; a view added later cannot work around the name.",
			kind, name, name),
	}
}

// validateOQLReservedAttributeName is the per-attribute half, called from
// validateEntityAttribute so CREATE ENTITY and ALTER ENTITY … ADD ATTRIBUTE are
// held to one rule.
//
// AutoX pseudo-types are skipped: the declared identifier is discarded on write
// and the attribute materialises under its fixed system member name, so the name
// checked here never reaches the model. (None of the four are OQL words anyway —
// the skip is about not warning on a name that does not exist.)
func validateOQLReservedAttributeName(attr ast.Attribute, entityName string) []linter.Violation {
	if _, isAuto := autoMemberNames[attr.Type.Kind]; isAuto {
		return nil
	}
	if !isOQLReservedName(attr.Name) {
		return nil
	}
	return []linter.Violation{oqlReservedNameViolation("attribute", attr.Name, entityName)}
}

// ValidateOQLReservedEntityName warns when the ENTITY's own name collides. A
// view says `from Module.Year as y`, and the parser reads `Year` there exactly
// as it reads an attribute after a dot.
func ValidateOQLReservedEntityName(stmt *ast.CreateEntityStmt) []linter.Violation {
	if stmt == nil || !isOQLReservedName(stmt.Name.Name) {
		return nil
	}
	return []linter.Violation{oqlReservedNameViolation("entity", stmt.Name.Name, stmt.Name.String())}
}

// ValidateOQLReservedRename warns when ALTER ENTITY … RENAME ATTRIBUTE renames
// INTO a reserved word. Renaming is how the collision is usually FIXED, so it is
// also how it can be introduced — and a rename lands on a name that already has
// references, which is the expensive case this rule exists to prevent.
func ValidateOQLReservedRename(stmt *ast.AlterEntityStmt) []linter.Violation {
	if stmt == nil || stmt.Operation != ast.AlterEntityRenameAttribute {
		return nil
	}
	if !isOQLReservedName(stmt.NewName) {
		return nil
	}
	return []linter.Violation{oqlReservedNameViolation("attribute", stmt.NewName, stmt.Name.String())}
}
