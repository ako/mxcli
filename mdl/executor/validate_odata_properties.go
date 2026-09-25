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
	"regexp"
	"sort"
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
			loc := "odata client " + s.Name.String()
			out = append(out, legacyODataExpression(loc, "HttpUsername", s.HttpUsername)...)
			out = append(out, legacyODataExpression(loc, "HttpPassword", s.HttpPassword)...)
			out = append(out, legacyODataExpression(loc, "ClientCertificate", s.ClientCertificate)...)
			for _, h := range s.Headers {
				out = append(out, legacyODataExpression(loc, "header "+h.Key, h.Value)...)
			}
		case *ast.AlterODataClientStmt:
			loc := "alter odata client " + s.Name.String()
			names := make([]string, 0, len(s.Changes))
			for name := range s.Changes {
				names = append(names, name)
			}
			sort.Strings(names) // map order would make two runs report differently
			for _, name := range names {
				if str, ok := s.Changes[name].(string); ok && isODataClientExpressionName(name) {
					out = append(out, legacyODataExpression(loc, name, str)...)
				}
			}
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

// quotedConstantRef matches the text of a string literal that is really a
// constant reference: `@Module.Name`.
var quotedConstantRef = regexp.MustCompile(`^@[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)+$`)

func isODataClientExpressionName(name string) bool {
	switch strings.ToLower(name) {
	case "httpusername", "httppassword", "clientcertificate":
		return true
	}
	return false
}

// legacyODataExpression (MDL-ODATA07) reports the two spellings whose meaning
// changed when HttpUsername / HttpPassword / ClientCertificate / header values
// became first-class expressions (PROPOSAL_first_class_expressions.md §6.4):
//
//	'''admin'''   was the string 'admin'; now a string that CONTAINS the quotes
//	'@Mod.C'      was a constant reference; now the literal text @Mod.C
//
// Both still parse, and both would now store something else without a word —
// a credential with stray quote characters, or a constant's name sent as the
// password. So they are errors that name the new spelling. The false positive
// is a real credential that begins and ends with a quote, or is shaped exactly
// like a qualified name after an @; the message says how to write either.
func legacyODataExpression(location, prop, expr string) []linter.Violation {
	content, isLiteral := mendixStringLiteral(expr)
	if !isLiteral {
		return nil
	}
	var msg, fix string
	switch {
	case len(content) >= 2 && strings.HasPrefix(content, "'") && strings.HasSuffix(content, "'"):
		msg = fmt.Sprintf("%s: %s is written %s — the doubled quotes are the old spelling of the string %s, "+
			"and now store the quote characters as part of the value", location, prop, expr, content)
		fix = fmt.Sprintf("Write %s: %s. A value that really does begin and end with a quote character "+
			"is written as a concatenation, which this check does not flag: %s",
			prop, content, "'''' + '"+strings.Trim(content, "'")+"' + ''''")
	case quotedConstantRef.MatchString(content):
		msg = fmt.Sprintf("%s: %s is written %s — a quoted @-name used to mean the constant %s, "+
			"and now stores the literal text %s", location, prop, expr, content[1:], content)
		fix = fmt.Sprintf("Write %s: %s (no quotes) to read the constant.", prop, content)
	default:
		return nil
	}
	return []linter.Violation{{
		RuleID:     "MDL-ODATA07",
		Severity:   linter.SeverityError,
		Message:    msg,
		Suggestion: fix,
	}}
}
