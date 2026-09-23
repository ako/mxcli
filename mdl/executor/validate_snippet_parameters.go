// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
)

// validateSnippetParameters (MDL087) refuses a primitive-typed snippet
// parameter. A snippet parameter must name an entity; mxbuild rejects every
// primitive with CE0046 "Invalid data type '<caption>'." — the measurements,
// and the page control that makes this specific to snippets, are in
// types.SnippetParameterTypeRule.
//
// # Why this is a check and not a write
//
// mendixlabs/mxcli#1028 reported the documented spelling failing at exec:
//
//	create snippet Test.SNIPPET_Label ( params: { $Label: string } )
//	  → Error: failed to build snippet: failed to resolve entity string:
//	           entity not found: string
//
// — a type nobody spelled, because the snippet's Params clause had its own
// visitor that handed the resolver the source text verbatim. The obvious repair
// is to write the primitive the way a page parameter writes one; the storage
// even allows it. mxbuild does not, so that repair trades an incomprehensible
// refusal for a document that fails a build later. The statement is refused
// here instead, in the project-free pass, naming the CE the author would
// otherwise meet at the far end of a build.
//
// Reported per parameter, so a clause with several bad ones names them all
// rather than one per run.
func validateSnippetParameters(stmt ast.Statement) []linter.Violation {
	snippet, ok := stmt.(*ast.CreateSnippetStmtV3)
	if !ok {
		return nil
	}

	var out []linter.Violation
	for _, p := range snippet.Parameters {
		caption := types.SnippetParameterTypeRule(pageParamBSONType(p.Type))
		if caption == "" {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:   "MDL087",
			Severity: linter.SeverityError,
			Location: linter.Location{
				Module:       snippet.Name.Module,
				DocumentType: "snippet",
				DocumentName: snippet.Name.Name,
			},
			Message: fmt.Sprintf(
				"snippet '%s' declares parameter $%s with the primitive type %s. "+
					"A snippet parameter must be an entity — mxbuild rejects a primitive one "+
					"with CE0046 (\"Invalid data type '%s'.\"). A page parameter may be primitive; "+
					"a snippet parameter may not.",
				snippet.Name.String(), p.Name, paramTypeSourceName(p.Type), caption),
			Suggestion: fmt.Sprintf(
				"pass the value on an object: declare `$%s: <Module>.<Entity>` and read the "+
					"member inside the snippet, or move the primitive to the calling PAGE's "+
					"parameters and keep it out of the snippet.", p.Name),
		})
	}
	return out
}

// paramTypeSourceName names a primitive parameter type the way MDL spells it,
// for the message. It is deliberately not the CE0046 caption: the author is
// looking for the word in their own script.
func paramTypeSourceName(dt ast.DataType) string {
	switch dt.Kind {
	case ast.TypeString:
		return "String"
	case ast.TypeInteger:
		return "Integer"
	case ast.TypeLong:
		return "Long"
	case ast.TypeDecimal:
		return "Decimal"
	case ast.TypeBoolean:
		return "Boolean"
	case ast.TypeDateTime:
		return "DateTime"
	default:
		return "a primitive"
	}
}
