// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// Two service-level shapes that MDL accepts, mxcli writes, and mxbuild then
// rejects — one of them without saying anything useful at all.
//
// Both were measured on Mendix 11.13 against a blank app, control and
// treatment, rather than read off the metamodel:
//
//	Path 'cat'        -> mxbuild CRASHES (ArgumentOutOfRangeException, no code)
//	Path '/cat'       -> CE6550 "The path should not start with a slash."
//	Path 'odata/cat'  -> CE6552 "The location should end with a single slash."
//	Path 'cat/'       -> 0 errors
//
//	PublishAssociations: No  -> CE7375, even with no associations published
//	PublishAssociations: Yes -> 0 errors

// ValidateODataServiceShape flags a published service that cannot build
// (MDL-ODATA05, MDL-ODATA06).
func ValidateODataServiceShape(prog *ast.Program) []linter.Violation {
	if prog == nil {
		return nil
	}
	var out []linter.Violation
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateODataServiceStmt:
			where := "odata service " + s.Name.String()
			out = append(out, checkODataPath(where, s.Path)...)
			// An unspecified value defaults to true (links) in the executor,
			// which is the buildable one — so only an explicit No is worth a
			// word.
			if s.PublishAssociationsSet && !s.PublishAssociations {
				out = append(out, associationModeViolation(where, len(s.Entities) > 0))
			}
		case *ast.AlterODataServiceStmt:
			where := "odata service " + s.Name.String()
			if v, ok := s.Changes["Path"].(string); ok {
				out = append(out, checkODataPath(where, v)...)
			}
			if v, ok := s.Changes["PublishAssociations"].(bool); ok && !v {
				out = append(out, associationModeViolation(where, true))
			}
		}
	}
	return out
}

// checkODataPath applies mxbuild's two rules for a service location.
//
// The no-slash case is called out separately because it is the one where
// mxbuild does not report an error at all — it throws
// ArgumentOutOfRangeException out of its own validator, with no error code, no
// element name and no line, which reads as a corrupt project rather than as a
// one-character mistake.
func checkODataPath(where, path string) []linter.Violation {
	if path == "" {
		return nil
	}
	var out []linter.Violation
	if strings.HasPrefix(path, "/") {
		out = append(out, linter.Violation{
			RuleID:     "MDL-ODATA05",
			Severity:   linter.SeverityError,
			Message:    fmt.Sprintf("%s: Path %q starts with a slash, which mxbuild rejects (CE6550)", where, path),
			Suggestion: fmt.Sprintf("Use %q.", strings.TrimPrefix(path, "/")+"/"),
		})
		return out
	}
	if !strings.HasSuffix(path, "/") {
		v := linter.Violation{
			RuleID:     "MDL-ODATA05",
			Severity:   linter.SeverityError,
			Message:    fmt.Sprintf("%s: Path %q does not end with a slash, which mxbuild rejects (CE6552)", where, path),
			Suggestion: fmt.Sprintf("Use %q.", path+"/"),
		}
		if !strings.Contains(path, "/") {
			v.Message = fmt.Sprintf("%s: Path %q contains no slash at all, which makes `mx check` "+
				"throw ArgumentOutOfRangeException instead of reporting an error", where, path)
			v.Suggestion = fmt.Sprintf("Use %q. mxbuild wants a location ending in a single slash (CE6552); "+
				"with no slash its own validator crashes, so there is no error code to look up.", path+"/")
		}
		out = append(out, v)
	}
	return out
}

// associationModeViolation explains the property whose name invites the wrong
// value.
//
// `PublishAssociations` is not "publish associations yes/no" — it is a two-value
// representation: true is "as a link (recommended)", false is "as an associated
// object id", which are Studio Pro's own labels for it. False then requires the
// system `ID` attribute published as the key, which MDL cannot express, so the
// build fails with CE7375 naming a concept the script never mentions.
//
// It stays a warning rather than an error because false is a legitimate Mendix
// mode for a service whose key is arranged elsewhere; what it is not is what it
// sounds like.
func associationModeViolation(where string, publishesEntities bool) linter.Violation {
	v := linter.Violation{
		RuleID:   "MDL-ODATA06",
		Severity: linter.SeverityWarning,
		Message: fmt.Sprintf("%s: PublishAssociations: No does not mean \"publish no associations\" — "+
			"it selects the \"as an associated object id\" representation", where),
		Suggestion: "That representation requires the system ID attribute published as the key, " +
			"which MDL cannot express, so mxbuild fails with CE7375 even when the service publishes " +
			"no associations at all. Use Yes for \"as a link (recommended)\", which is also the " +
			"default when the property is omitted.",
	}
	if !publishesEntities {
		v.Severity = linter.SeverityInfo
	}
	return v
}
