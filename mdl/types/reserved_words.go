// SPDX-License-Identifier: Apache-2.0

package types

import "strings"

// PlatformReservedWords are names the Mendix PLATFORM refuses for a model
// element, whatever MDL thinks of them. Java reserved words plus Mendix's own
// reserved identifiers; using one triggers CE7247 "The name 'X' is a reserved
// word."
//
// This is a different thing from an MDL parser keyword, and the difference is
// load-bearing. A parser keyword is escaped by quoting — `write ("Title")` — and
// the name reaches the model intact. A platform-reserved word is NOT: the check
// strips the quotes and validates the bare name, so quoting buys nothing and the
// element has to be renamed. Advice that confuses the two is worse than no
// advice, because it sends someone either to a rename they did not need or to a
// build error they were told they had avoided.
//
// It lives in mdl/types rather than beside its first caller because BOTH the
// validator (mdl/executor) and the parse-error hinting (mdl/visitor) need it,
// and neither may import the other.
var PlatformReservedWords = map[string]bool{
	// Java reserved words
	"abstract": true, "assert": true, "boolean": true, "break": true,
	"byte": true, "case": true, "catch": true, "char": true,
	"class": true, "const": true, "continue": true, "default": true,
	"do": true, "double": true, "else": true, "enum": true,
	"extends": true, "false": true, "final": true, "finally": true,
	"float": true, "for": true, "goto": true, "if": true,
	"implements": true, "import": true, "instanceof": true, "int": true,
	"interface": true, "long": true, "native": true, "new": true,
	"null": true, "package": true, "private": true, "protected": true,
	"public": true, "return": true, "short": true, "static": true,
	"strictfp": true, "super": true, "switch": true, "synchronized": true,
	"this": true, "throw": true, "throws": true, "transient": true,
	"true": true, "type": true, "void": true, "volatile": true,
	"while": true,
	// Mendix-specific reserved identifiers
	"changedby": true, "changeddate": true, "con": true, "context": true,
	"createddate": true, "currentuser": true, "guid": true,
	"id": true, "mendixobject": true, "owner": true, "submetaobjectname": true,
}

// SystemAttributeNames are attribute names the Mendix runtime manages itself on
// a persistent entity. Declaring one is MDL020; the AutoCreatedDate /
// AutoChangedDate / AutoOwner / AutoChangedBy pseudo-types are how a model asks
// for the audit field it means.
var SystemAttributeNames = map[string]bool{
	"createddate": true,
	"changeddate": true,
	"owner":       true,
	"changedby":   true,
}

// IsPlatformReserved reports whether a bare name is refused by the Mendix
// platform, and therefore cannot be rescued by quoting it in MDL.
func IsPlatformReserved(name string) bool {
	lower := strings.ToLower(strings.Trim(strings.TrimSpace(name), `"`))
	return PlatformReservedWords[lower] || SystemAttributeNames[lower]
}
