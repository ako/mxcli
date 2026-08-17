// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation of OData property names.
//
// The grammar accepts any `name: value` pair inside an OData property list, and
// the visitor's switch had no default — so `ReadMicroflow:` or `Pagesize:` was
// discarded in silence and the model was quietly missing what the author asked
// for. The ALTER path has always answered "unknown OData service property: %s";
// this applies the same rule to CREATE and to the PUBLISH ENTITY block.
// (mxcli-formula1 findings, suggested issue 8.)
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Known property names, in the spelling the syntax help uses. These are for the
// error message only — the visitor is the authority on what is accepted, and it
// matches case-insensitively.
//
// The two drifted once already: Countable/SkipSupported/TopSupported were added
// to the visitor and the hint went on advertising six properties, so a user
// reading it would think three accepted properties were not. TestKnownODataProps
// keeps them in step by running every name below through the visitor.
var (
	knownODataServiceProps = []string{
		"Path", "Version", "ODataVersion", "Namespace", "ServiceName",
		"Summary", "Description", "PublishAssociations", "SupportsGraphQL", "Folder",
	}
	knownPublishEntityProps = []string{
		"ReadMode", "InsertMode", "UpdateMode", "DeleteMode", "UsePaging", "PageSize",
		"Countable", "SkipSupported", "TopSupported",
	}
	knownODataClientProps = []string{
		"Version", "ODataVersion", "MetadataUrl", "Timeout", "ProxyType",
		"Description", "ServiceUrl", "UseAuthentication", "HttpUsername",
		"HttpPassword", "ClientCertificate", "ConfigurationMicroflow",
		"HeadersMicroflow", "ErrorHandlingMicroflow", "ProxyHost", "ProxyPort",
		"ProxyUsername", "ProxyPassword", "Folder",
	}
	knownExternalEntityProps = []string{
		"EntitySet", "RemoteName", "Countable", "Creatable", "Deletable",
		"Updatable", "AllowCreateChangeLocally",
	}
)

// ValidateODataProperties flags (MDL-ODATA01) property names in OData
// statements that no layer below will act on.
func ValidateODataProperties(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateODataServiceStmt:
			out = append(out, unknownODataProps(
				"odata service "+s.Name.String(), s.UnknownProperties, knownODataServiceProps)...)
			for _, e := range s.Entities {
				if e == nil {
					continue
				}
				out = append(out, unknownODataProps(
					fmt.Sprintf("publish entity %s in %s", e.Entity.String(), s.Name.String()),
					e.UnknownProperties, knownPublishEntityProps)...)
			}
		case *ast.CreateODataClientStmt:
			out = append(out, unknownODataProps(
				"odata client "+s.Name.String(), s.UnknownProperties, knownODataClientProps)...)
		case *ast.CreateExternalEntityStmt:
			out = append(out, unknownODataProps(
				"external entity "+s.Name.String(), s.UnknownProperties, knownExternalEntityProps)...)
		}
	}
	return out
}

func unknownODataProps(location string, unknown, known []string) []linter.Violation {
	var out []linter.Violation
	for _, name := range unknown {
		v := linter.Violation{
			RuleID:   "MDL-ODATA01",
			Severity: linter.SeverityError,
			Message: fmt.Sprintf("%s: unknown property %q — it is accepted by the parser and then discarded, so the model will not have it",
				location, name),
			Suggestion: fmt.Sprintf("Known properties here: %s.", strings.Join(known, ", ")),
		}
		if near := closestProperty(name, known); near != "" {
			v.Suggestion = fmt.Sprintf("Did you mean %q? Known properties here: %s.", near, strings.Join(known, ", "))
		}
		out = append(out, v)
	}
	return out
}

// closestProperty returns the known property a misspelling most likely meant,
// or "" when nothing is close enough to be worth guessing. Case-insensitive
// prefix/substring first, then a single edit.
func closestProperty(name string, known []string) string {
	lower := strings.ToLower(name)
	for _, k := range known {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, lower) || strings.HasPrefix(lower, lk) || strings.Contains(lk, lower) {
			return k
		}
	}
	for _, k := range known {
		if withinOneEdit(lower, strings.ToLower(k)) {
			return k
		}
	}
	return ""
}

// withinOneEdit reports whether a and b differ by at most one insertion,
// deletion or substitution.
func withinOneEdit(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	if len(b)-len(a) > 1 {
		return false
	}
	i, j, edits := 0, 0, 0
	for i < len(a) && j < len(b) {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		edits++
		if edits > 1 {
			return false
		}
		if len(a) == len(b) {
			i++
		}
		j++
	}
	return true
}
