// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/xpathrefs"
)

// renameAttributeInXPath rewrites the renamed attribute inside every stored XPath
// constraint in the project.
//
// dmUnitID and entityName identify the renamed entity's own domain model unit,
// which is how an access rule's constraint is attributed: the rule carries no
// entity reference, it simply lives inside one.
func renameAttributeInXPath(ctx *ExecContext, entityQN, dmUnitID, entityName, oldAttr, newAttr string) (xpathrefs.Result, error) {
	model, err := buildXPathModel(ctx)
	if err != nil {
		return xpathrefs.Result{}, err
	}
	return xpathrefs.RenameAttribute(ctx.Backend, model, entityQN, dmUnitID, entityName, oldAttr, newAttr)
}

// reportXPathRename prints what the XPath pass did and what it refused to do.
//
// The refusals are the half that matters. A constraint mxcli could not resolve is
// left exactly as it was, so the project is no worse than before — but the user
// has to be told which one, or the rename reads as complete and the breakage
// surfaces later as a CE0161 with no obvious cause.
func reportXPathRename(ctx *ExecContext, res xpathrefs.Result) {
	if n := res.Total(); n > 0 {
		fmt.Fprintf(ctx.Output, "Updated %d XPath constraint(s) in %d document(s)\n", n, res.Units)
	}
	if len(res.Skipped) == 0 {
		return
	}
	fmt.Fprintf(ctx.Output,
		"Warning: %d XPath constraint(s) name the attribute but could not be resolved, "+
			"and were left unchanged — check them by hand:\n", len(res.Skipped))
	for _, occ := range dedupeOccurrences(res.Skipped) {
		fmt.Fprintf(ctx.Output, "  %s: %s\n", describeOccurrence(occ), occ.Constraint)
	}
}

// describeOccurrence labels a constraint by the document it sits in, falling back
// to the unit ID when the document has no name of its own.
func describeOccurrence(occ xpathrefs.Occurrence) string {
	label := occ.Document
	if label == "" {
		label = occ.UnitID
	}
	typeName := occ.UnitType
	if i := strings.Index(typeName, "$"); i >= 0 {
		typeName = typeName[i+1:]
	}
	if typeName == "" {
		return label
	}
	return fmt.Sprintf("%s (%s)", label, typeName)
}

// dedupeOccurrences collapses identical (document, constraint) pairs and sorts
// them, so the warning is stable across runs and a constraint repeated in one
// document is listed once.
func dedupeOccurrences(occs []xpathrefs.Occurrence) []xpathrefs.Occurrence {
	seen := map[string]bool{}
	out := make([]xpathrefs.Occurrence, 0, len(occs))
	for _, o := range occs {
		key := o.UnitID + "\x00" + o.Constraint
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, o)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Document != out[j].Document {
			return out[i].Document < out[j].Document
		}
		return out[i].Constraint < out[j].Constraint
	})
	return out
}
