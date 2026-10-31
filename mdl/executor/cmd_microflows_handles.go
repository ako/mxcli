// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend/mfmutator"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// microflowTargets returns the content-addressable activities of a stored
// microflow, in the order `describe microflow` prints them, which is the order
// `@n` ordinals count in. The statement each is matched against is the one
// formatActivity renders for describe, so what a reader sees is what an
// `alter microflow` target matches.
//
// It also returns the describe body it rendered to establish that order and
// the body line each activity starts on (annotations included), so describe
// … with handles need not render twice.
func microflowTargets(
	ctx *ExecContext,
	mf *microflows.Microflow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) (cands []mfmutator.Candidate, warnings, body []string, startLine map[model.ID]int) {
	if mf == nil || mf.ObjectCollection == nil {
		return nil, nil, nil, nil
	}
	// formatActivity renders an end event differently in a flow with a return
	// value; set the flag the way describe does, whoever the caller is.
	prev := ctx.DescribingMicroflowHasReturnValue
	ctx.DescribingMicroflowHasReturnValue = microflowHasReturnValue(mf)
	defer func() { ctx.DescribingMicroflowHasReturnValue = prev }()

	sourceMap := map[string]elkSourceRange{}
	warnings, body = formatMicroflowBodyWithSourceMap(ctx, mf, entityNames, microflowNames, sourceMap, 0)

	startLine = make(map[model.ID]int, len(sourceMap))
	ranges := make(map[model.ID]elkSourceRange, len(sourceMap))
	for key, r := range sourceMap {
		if id, ok := strings.CutPrefix(key, "node-"); ok {
			startLine[model.ID(id)] = r.StartLine
			ranges[model.ID(id)] = r
		}
	}

	render := func(obj microflows.MicroflowObject) string {
		return formatActivity(ctx, obj, entityNames, microflowNames)
	}
	cands = mfmutator.OrderBy(mfmutator.Collect(mf.ObjectCollection, render), startLine)
	for i := range cands {
		if r, ok := ranges[cands[i].ID]; ok {
			cands[i].SetPrinted(printedStatement(cands[i].Object, body, r))
		}
	}
	return cands, warnings, body, startLine
}

// printedStatement extracts, from the describe lines an activity occupies,
// the statement itself: the annotation and comment lines before it are
// skipped, and the statement runs to the line that ends it, continuation
// lines included — describe breaks a long condition without indenting the
// rest. An `if` ends at `then`; an action at `;` (or the `begin` of an error
// handler block); a loop, an enumeration `case` and a `split type` open a
// block on the next line, so they are their first line.
//
// This is what the reader sees, which is not always what formatActivity
// returns: an if with an empty then-branch is printed negated, with its
// branches swapped. Returns "" when no terminated statement is found.
func printedStatement(obj microflows.MicroflowObject, body []string, r elkSourceRange) string {
	var parts []string
	for i := r.StartLine; i <= r.EndLine && i < len(body); i++ {
		line := strings.TrimSpace(body[i])
		if len(parts) == 0 && (line == "" || strings.HasPrefix(line, "@") || strings.HasPrefix(line, "--")) {
			continue
		}
		parts = append(parts, line)
		switch obj.(type) {
		case *microflows.LoopedActivity, *microflows.InheritanceSplit:
			return line
		case *microflows.ExclusiveSplit:
			if len(parts) == 1 && strings.HasPrefix(line, "case ") {
				return line
			}
			if strings.HasSuffix(line, " then") {
				return strings.Join(parts, " ")
			}
		default:
			// The `begin` that opens an error handler block, and an empty
			// handler's whole `begin end error`, are not part of the statement:
			// a handle is written as an alter target, which names the activity
			// and ends before any fragment.
			if strings.HasSuffix(line, " begin end error;") {
				return strings.TrimSpace(strings.TrimSuffix(strings.Join(parts, " "), " begin end error;"))
			}
			if strings.HasSuffix(line, ";") {
				return strings.Join(parts, " ")
			}
			if strings.HasSuffix(line, " begin") {
				return strings.TrimSpace(strings.TrimSuffix(strings.Join(parts, " "), " begin"))
			}
		}
		if len(parts) >= 50 {
			break
		}
	}
	return ""
}

// formatMicroflowActivitiesWithHandles is formatMicroflowActivities with a
// `-- handle: <target>` comment above each activity: the address that selects
// it in an `alter microflow` target (ADR-0012, decision 2). The handles are
// comments, so the output is the plain description plus those lines and
// executes the same.
//
// An activity inside an error handler block — commented-out ones included —
// gets its handle like any other: the handler bodies are in the source map,
// so they are ranked where they are printed. An activity describe prints as
// nothing (a void end event) or only as a comment gets no handle line; it is
// still listed, with its ordinal, by an ambiguity error.
func formatMicroflowActivitiesWithHandles(
	ctx *ExecContext,
	mf *microflows.Microflow,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
) []string {
	defer ctx.describingFlowLayout(mf.Documentation)()
	cands, warnings, body, startLine := microflowTargets(ctx, mf, entityNames, microflowNames)

	handlesAt := map[int][]string{}
	for i, c := range cands {
		line, ok := startLine[c.ID]
		if !ok || line < 0 || line >= len(body) {
			continue
		}
		if h := mfmutator.Handle(cands, i); h != "" {
			handlesAt[line] = append(handlesAt[line], h)
		}
	}

	out := make([]string, 0, len(warnings)+len(body)+len(handlesAt))
	out = append(out, warnings...)
	for i, line := range body {
		indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
		for _, h := range handlesAt[i] {
			out = append(out, indent+"-- handle: "+h)
		}
		out = append(out, line)
	}
	return out
}
