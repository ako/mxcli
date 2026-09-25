// SPDX-License-Identifier: Apache-2.0

package main

import "github.com/mendixlabs/mxcli/mdl/linter"

// keepDocumentViolations drops findings that are not about one of the named
// documents.
//
// The SQL narrowing in LintContext covers the rules that walk a document
// iterator, which is where the time goes. It cannot cover a rule that reports
// from project settings or from a security policy — those still run, and a
// scoped lint would carry their findings alongside the one document the caller
// asked about. `lint -d Sales.ACT_Order` answering about anything else is the
// same class of lie as a gate that passes what it never read (ako/mxcli#681).
//
// Matching accepts BOTH spellings of Location.DocumentName, because the rules
// disagree: CONV010 sets it to the qualified name and MPR011 to the short name,
// so a matcher that picked one would silently drop half the rules' findings.
func keepDocumentViolations(violations []linter.Violation, documents []string) []linter.Violation {
	want := make(map[string]bool, len(documents))
	for _, d := range documents {
		want[d] = true
	}
	kept := make([]linter.Violation, 0, len(violations))
	for _, v := range violations {
		if want[v.Location.DocumentName] || want[v.Location.QualifiedName()] {
			kept = append(kept, v)
		}
	}
	return kept
}
