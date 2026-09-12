// SPDX-License-Identifier: Apache-2.0

// Check-time (no-project) validation for annotations written before a CREATE.
//
// MDL's grammar attaches `annotation*` to createStatement itself, so all forty-odd
// create kinds accept one while only six read one. Everything else parsed and did
// nothing — the failure MDL059 already refuses one node family over, and the same
// one #884 was filed for: an annotation that parses and does nothing loses
// whatever it was meant to express, silently.
//
// Two shapes it lets through, both real:
//
//	@applyentityacces      -- a typo; the security setting is simply not applied
//	@applyentityaccess     -- on a NANOFLOW, which has no such property at all
//
// The second is not hypothetical: it was created by the change that added the
// annotation, and left open with the note that it fails safe. Failing safe is not
// the same as being reported.
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// documentAnnotations maps a document kind to the annotations its builder reads.
// A kind absent from this map reads none, which is the honest default: the
// grammar accepts an annotation there and nothing acts on it.
//
// Keys are the kind names ExitCreateStatement derives from the grammar's own rule
// names, so they track the grammar rather than a hand-kept list. Values are the
// names each visitor actually tests for — TestDocumentAnnotationsMatchTheVisitor
// pins the two together, because a name added to one and not the other either
// rejects a valid annotation or silently drops an invalid one.
var documentAnnotations = map[string]map[string]bool{
	// visitor_entity.go — @position places the entity on the domain-model canvas.
	// Covers view entities too: both go through createEntityStatement.
	"entity": {"position": true},
	// visitor_association.go — @anchor picks which side each end attaches to.
	"association": {"anchor": true},
	// visitor_microflow.go
	"microflow": {"excluded": true, "applyentityaccess": true},
	"nanoflow":  {"excluded": true},
	"rule":      {"excluded": true, "applyentityaccess": true},
	// visitor_page_v3.go
	"page": {"excluded": true},
}

// ValidateDocumentAnnotations reports (MDL059) an annotation the document it is
// written on does not read.
func ValidateDocumentAnnotations(prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for _, ann := range prog.DocumentAnnotations {
		if ann.Kind == "" {
			continue // no create statement parsed — a syntax error already reported
		}
		if documentAnnotations[ann.Kind][ann.Name] {
			continue
		}
		out = append(out, linter.Violation{
			RuleID:     "MDL059",
			Severity:   linter.SeverityError,
			Message:    documentAnnotationMessage(ann),
			Suggestion: documentAnnotationSuggestion(ann.Kind),
		})
	}
	return out
}

func documentAnnotationMessage(ann ast.DocumentAnnotation) string {
	target := ""
	if ann.Target != "" {
		target = " on " + ann.Target
	}
	return fmt.Sprintf("unknown annotation `@%s`%s — a %s does not read it, so it "+
		"parses and does nothing and whatever it was meant to express is silently lost",
		ann.Name, target, ann.Kind)
}

// documentAnnotationSuggestion names what the document DOES accept, so a typo is
// one line from being fixed — and so the nanoflow case reads as the real answer
// it is rather than as a bare refusal.
func documentAnnotationSuggestion(kind string) string {
	accepted := documentAnnotations[kind]
	if len(accepted) == 0 {
		return fmt.Sprintf("A %s reads no annotations at all. Annotations before CREATE "+
			"belong to entity (@position), association (@anchor), page/nanoflow (@excluded) "+
			"and microflow/rule (@excluded, @applyentityaccess); activity annotations "+
			"(@position, @caption, @colour, …) go inside the flow body, on the statement "+
			"they belong to.", kind)
	}
	names := make([]string, 0, len(accepted))
	for name := range accepted {
		names = append(names, "@"+name)
	}
	sort.Strings(names)
	return fmt.Sprintf("A %s reads %s. If this is a typo of one of those, correct it; "+
		"otherwise remove it.", kind, strings.Join(names, " and "))
}
