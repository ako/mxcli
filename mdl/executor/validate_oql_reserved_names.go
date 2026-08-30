// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// MDL071 warns that a name will need QUOTING from OQL, at the point the name is
// chosen rather than at the point a view finally uses it.
//
// MDL032 already reports the collision, but only when the view's OQL is written
// — and by then the name is everywhere. In the reported case (ako/mxcli-captrack
// #1) `Month`, `Year` and `Quarter` had reached 121 places across nine
// generators and the database columns before the first view needed them.
//
// It is a WARNING, and deliberately mild, because OQL *does* have a
// quoted-identifier form — double quotes, exactly as in SQL — and mxcli writes
// it through unchanged. Measured on 11.13.0, all at 0 errors:
//
//	select s."Month" as MonthNo …          -- quoted source attribute
//	from MyFirstModule."Year" as y         -- quoted source entity
//
// So the remedy is usually "quote it in the view", not "rename it". The warning
// earns its place anyway: the unquoted form is what anyone writes first, it
// fails a whole build later with CE0174, and the fix gets more expensive with
// every reference added in between.
//
// THE ONE PLACE QUOTING DOES NOT REACH is the alias, and the limit is OQL's own:
// it accepts only a bare identifier there, for ANY name. Measured with `as
// "Total"`, a word reserved nowhere:
//
//	CE0174 "The '"' part is incomplete or incorrect.
//	        You could use here: IDENTIFIER."
//
// The two CE0174 texts differ exactly on this. A source position lists
// "ASTERISK, AT_SIGN, OPEN_QUOTE, or IDENTIFIER"; the alias position lists only
// IDENTIFIER. Reading them is what settles which positions can be quoted — an
// earlier revision guessed twice and was wrong twice.
//
// A view entity's own attribute name IS its alias (they must match), so a view
// entity cannot have an attribute called `Month` at all: unquoted the alias is
// CE0174, and quoted it is CE0174 for a different reason. That case needs a
// rename, and it is the only one that does. (MDL072 reports the quoted spelling
// with that explanation rather than letting it fail to parse.)
//
// Both an ATTRIBUTE and an ENTITY name are affected — each measured on its own,
// unquoted:
//
//	attribute Month  →  CE0174 "The 'Month' part is incomplete or incorrect.
//	                    You could use here: ASTERISK, AT_SIGN, OPEN_QUOTE, or
//	                    IDENTIFIER."
//	entity    Year   →  CE0174 "The 'Year' part is incomplete or incorrect. …"
//
// Note OPEN_QUOTE in what the parser says it expects: the quoted form is right
// there in the error text, which is worth reading before concluding a name is
// unusable. An earlier revision of this file asserted OQL had no quoted form at
// all; it was contradicted by the very error message it quoted.
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
			"%s '%s' is an OQL reserved word — valid Mendix, but a view entity referencing it "+
				"unquoted fails to build with CE0174",
			kind, name),
		Location: linter.Location{DocumentType: "entity", DocumentName: entityName},
		Suggestion: fmt.Sprintf(
			"Usually no rename needed: quote it in the view's OQL, as OQL takes double-quoted "+
				"identifiers like SQL — `select s.%q …`, `from Module.%q as s`. The exception is a "+
				"VIEW ENTITY's own attribute, whose name is also its select alias: OQL takes only "+
				"a bare identifier there, so a view column must not be called '%s'.",
			name, name, name),
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
//
// A NON-PERSISTENT entity is skipped for a stronger reason: it cannot be reached
// from OQL at all, so the collision this rule predicts cannot occur. See
// oqlUnreachable.
func validateOQLReservedAttributeName(attr ast.Attribute, kind entityPersistence, entityName string) []linter.Violation {
	if oqlUnreachable(kind) {
		return nil
	}
	if _, isAuto := autoMemberNames[attr.Type.Kind]; isAuto {
		return nil
	}
	if !isOQLReservedName(attr.Name) {
		return nil
	}
	return []linter.Violation{oqlReservedNameViolation("attribute", attr.Name, entityName)}
}

// oqlUnreachable reports whether the entity is one no OQL query can name, which
// makes the whole premise of MDL071 unreachable for it.
//
// Measured on mxbuild 11.13.0 by pointing a view entity at a non-persistent one
// (ledger #148) — Mendix refuses the reference itself rather than the name:
//
//	[error] [CE0174] "Error(s) in OQL query: Entity 'DS147.NpeYear' cannot be
//	        used in OQL, because it is a non-persistable entity"
//
// Only a KNOWN non-persistent entity is skipped. Unknown keeps the warning: the
// ALTER path has no kind to read, and a name that arrives late is the expensive
// case this rule exists for.
func oqlUnreachable(kind entityPersistence) bool { return kind == persistenceNonPersistent }

// ValidateOQLReservedEntityName warns when the ENTITY's own name collides. A
// view says `from Module.Year as y`, and the parser reads `Year` there exactly
// as it reads an attribute after a dot.
func ValidateOQLReservedEntityName(stmt *ast.CreateEntityStmt) []linter.Violation {
	if stmt == nil || oqlUnreachable(entityPersistenceOf(stmt.Kind)) || !isOQLReservedName(stmt.Name.Name) {
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
