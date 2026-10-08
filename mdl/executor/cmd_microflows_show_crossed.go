// SPDX-License-Identifier: Apache-2.0

package executor

// Mode 2 of PROPOSAL_structured_microflow_description.md: describe a graph whose
// branches CROSS faithfully, instead of flattening it into MDL that means
// something else.
//
// Phase E made an error path's rejoin sayable (`merge`/`join`). This says the
// other half with the same vocabulary: an inner split's branch landing where an
// outer split's branch lands — the shape no nesting of `if` reproduces, and the
// one #923 was actually reported for.
//
// # Why a crossed merge is handled the opposite way from a rejoin merge
//
// Both get a label, but they are emitted differently, and getting this backwards
// produces MDL that does not parse:
//
//   - An ERROR-REJOIN merge is declared INLINE where the traversal meets it
//     (traverseFlowUntilMerge does this), because the normal path owns it and
//     only the handler needs to name it.
//   - A CROSSED merge is reached by two or more BRANCHES of the same split. It
//     can be declared only once, so each branch ends in `join <label>` and stops,
//     and the declaration is emitted at the split's own level once both branches
//     are closed.
//
// That is why crossedMerges is a separate set rather than more entries in the
// same map: the label alone does not say which of the two emissions applies.

import (
	"fmt"
	"sort"

	"github.com/mendixlabs/mxcli/mdl/microflowgraph"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// labelCrossedMerges labels the shared-suffix entry of every RECOMBINABLE split,
// which is exactly the node a nested rendering cannot place.
//
// It deliberately ignores Interleaved findings. Those overlap at more than one
// entry, and per Böhm-Jacopini nesting them needs either a duplicated activity
// or a boolean variable the user never wrote; `merge`/`join` can express the
// graph, but the describer's branch structure cannot be built from one entry, so
// they keep MDL-FLOW01 and are left alone rather than half-described. The one
// exception is a multi-way split whose arms would each PRINT a shared activity
// (sharedArmEntries): leaving that alone is not "half-described", it is a
// description with an activity the model does not have.
//
// Labels are assigned in position order, for the same reason labelRejoinMerges
// does it: map iteration order would make DESCRIBE unstable and turn every
// re-describe into a spurious diff.
func labelCrossedMerges(col *microflows.MicroflowObjectCollection) mergeLabels {
	if col == nil {
		return mergeLabels{}
	}

	objects := map[model.ID]microflows.MicroflowObject{}
	for _, o := range col.Objects {
		if o != nil {
			objects[o.GetID()] = o
		}
	}

	// BOTH the shared entry and the split's post-dominator are labelled, and the
	// second is not optional. mxcli's `merge` permits fall-through (an ordinary
	// path entering a following `merge` joins it), which is friendly for a
	// hand-author and ambiguous for a generated description: split2's false arm
	// is EMPTY here and belongs at the post-dominator, but an empty arm followed
	// by `merge shared1` falls into shared1 instead — silently describing a
	// different graph, which is the whole failure Mode 2 exists to end.
	//
	// Labelling the join too lets every branch end in an explicit `join`, which
	// is what the proposal's Mode 2 sketch does and why it wanted fall-through
	// banned outright.
	needsLabel := map[model.ID]bool{}
	findings := microflowgraph.Analyze(col.Objects, col.Flows)
	for id := range sharedArmEntries(col, objects, findings) {
		needsLabel[id] = true
	}
	for id := range sharedHandlerEntries(col, objects) {
		needsLabel[id] = true
	}
	for _, f := range findings {
		if f.Class != microflowgraph.Recombinable || len(f.Entries) != 1 {
			continue
		}
		// When the shared entry IS the split's own post-dominator, nothing is
		// crossed: `end if` / `end split` already places it, and the nested
		// description is faithful. Analyze reports these because some branches
		// reach the tail and others return early — measured on
		// Administration.ManageMyAccount, whose description round-trips with its
		// merge $ID intact and which therefore never needed naming. Labelling it
		// would put a `join` where the structure is already right, and (worse)
		// would retire MDL-FLOW01 on the strength of a label nothing used.
		if f.Entries[0] == f.JoinID {
			continue
		}
		for _, id := range []model.ID{f.Entries[0], f.JoinID} {
			if _, isMerge := objects[id].(*microflows.ExclusiveMerge); isMerge {
				needsLabel[id] = true
			}
		}
	}
	if len(needsLabel) == 0 {
		return mergeLabels{}
	}

	ids := make([]model.ID, 0, len(needsLabel))
	for id := range needsLabel {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		pi, pj := objects[ids[i]].GetPosition(), objects[ids[j]].GetPosition()
		if pi.X != pj.X {
			return pi.X < pj.X
		}
		if pi.Y != pj.Y {
			return pi.Y < pj.Y
		}
		return ids[i] < ids[j]
	})

	out := mergeLabels{
		byID:    make(map[model.ID]string, len(ids)),
		crossed: make(map[model.ID]bool, len(ids)),
	}
	for i, id := range ids {
		out.byID[id] = fmt.Sprintf("shared%d", i+1)
		out.crossed[id] = true
	}
	return out
}

// mergeAllLabels combines the rejoin labels with the crossed ones.
//
// A merge that is BOTH an error rejoin and a crossed entry keeps its rejoin
// label — Phase E's output is already in the wild and its label names appear in
// scripts people have saved — but it joins the crossed set, because how it is
// emitted is decided by the branch structure, not by which pass named it.
func mergeAllLabels(rejoin, crossed mergeLabels) mergeLabels {
	if rejoin.len() == 0 {
		return crossed
	}
	if crossed.len() == 0 {
		return rejoin
	}
	out := mergeLabels{
		byID:    make(map[model.ID]string, rejoin.len()+crossed.len()),
		crossed: crossed.crossed,
	}
	for id, l := range crossed.byID {
		out.byID[id] = l
	}
	for id, l := range rejoin.byID {
		out.byID[id] = l
	}
	return out
}

// emitCrossedMergeSections writes the declaration and body of every crossed
// merge, after the main traversal has finished and each branch has emitted its
// `join`.
//
// Order is label order, which is position order, so the same graph always
// describes the same way.
//
// Each section traverses from the merge's SUCCESSOR rather than from the merge
// itself. That is what keeps the two emissions from colliding: the merge node is
// never "arrived at" during its own section, so the join-and-stop rule in
// traverseFlow cannot fire here and turn the declaration into a `join` to
// itself. The body then runs on until it reaches the NEXT crossed merge, where
// it emits a `join` and stops — which is how a chain of shared suffixes comes
// out as a flat sequence of sections instead of a nesting that does not exist.
func emitCrossedMergeSections(
	ctx *ExecContext,
	col *microflows.MicroflowObjectCollection,
	activityMap map[model.ID]microflows.MicroflowObject,
	flowsByOrigin map[model.ID][]*microflows.SequenceFlow,
	flowsByDest map[model.ID][]*microflows.SequenceFlow,
	splitMergeMap map[model.ID]model.ID,
	visited map[model.ID]bool,
	entityNames map[model.ID]string,
	microflowNames map[model.ID]string,
	lines *[]string,
	sourceMap map[string]elkSourceRange,
	headerLineCount int,
	annotationsByTarget *annotationEmitter,
	labels mergeLabels,
) map[model.ID]bool {
	declared := map[model.ID]bool{}
	if len(labels.crossed) == 0 {
		return declared
	}

	ids := make([]model.ID, 0, len(labels.crossed))
	for id := range labels.crossed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		li, _ := labels.of(ids[i])
		lj, _ := labels.of(ids[j])
		if li != lj {
			return li < lj
		}
		return ids[i] < ids[j]
	})

	for _, id := range ids {
		if visited[id] {
			continue
		}
		label, ok := labels.of(id)
		if !ok {
			continue
		}
		visited[id] = true
		declared[id] = true
		*lines = append(*lines, mergeDeclarationLines(0, label, activityMap[id], annotationsByTarget.layoutKeep())...)
		for _, flow := range flowsByOrigin[id] {
			traverseFlow(ctx, flow.DestinationID, activityMap, flowsByOrigin, flowsByDest,
				splitMergeMap, visited, entityNames, microflowNames, lines, 0,
				sourceMap, headerLineCount, annotationsByTarget, labels)
		}
	}
	return declared
}

// sharedArmEntries finds the shared regions a multi-way split's arms would each
// print, and returns the merges that must be labelled so they are printed once.
//
// A `case` or `split type` renders every arm with its own copy of the visited
// set (emitEnumSplitStatement, emitInheritanceSplitStatement), walking each one
// until the split's join. When two arms reach the same activity BEFORE that
// join, both copies walk it, and the description prints it twice — the second
// copy a different activity with the same output variable, which is what made
// OIDC.handleAuthorizationCode fail check with MDL063 (Evora). The model has
// one REST call where two of the four arms meet; the description had two.
//
// The fix is the crossed-merge vocabulary: every arm that reaches the shared
// region `join`s it, and the region is printed once, in its own section. So the
// entry of the region has to be a merge (there is nothing to name otherwise),
// and the split's join is labelled too, because the section has to say where it
// goes when it reaches the join — walking into it would print the rest of the
// flow inline a second time.
//
// Deliberately narrow:
//
//   - only splits Analyze already flags, and only enumeration/type splits — the
//     ones whose arms are walked independently. An if/else's ELSE copy already
//     contains the THEN arm's objects, so it does not print a region twice;
//   - only when the shared region holds a real activity. Arms that converge on
//     a merge chain and nothing else print nothing twice, describe faithfully
//     today, and must not change;
//   - only when the join is the split's own describer merge and every entry is
//     a merge. Anything else is left to the MDL-FLOW01 warning rather than half
//     described.
//
// Nodes that can reach the split again are not part of a shared region: they
// are a loop's way back round, which the arms share by construction.
func sharedArmEntries(
	col *microflows.MicroflowObjectCollection,
	objects map[model.ID]microflows.MicroflowObject,
	findings []microflowgraph.Finding,
) map[model.ID]bool {
	out := map[model.ID]bool{}
	var candidates []model.ID
	for _, f := range findings {
		switch s := f.Split.(type) {
		case *microflows.InheritanceSplit:
			candidates = append(candidates, f.SplitID)
		case *microflows.ExclusiveSplit:
			if _, ok := enumSplitVariable(s); ok {
				candidates = append(candidates, f.SplitID)
			}
		}
	}
	if len(candidates) == 0 {
		return out
	}

	flowsByOrigin := map[model.ID][]*microflows.SequenceFlow{}
	succ := map[model.ID][]model.ID{}
	pred := map[model.ID][]model.ID{}
	for _, fl := range col.Flows {
		if fl == nil {
			continue
		}
		flowsByOrigin[fl.OriginID] = append(flowsByOrigin[fl.OriginID], fl)
		if fl.IsErrorHandler {
			continue
		}
		succ[fl.OriginID] = append(succ[fl.OriginID], fl.DestinationID)
		pred[fl.DestinationID] = append(pred[fl.DestinationID], fl.OriginID)
	}
	splitMerge := findSplitMergePointsForGraph(nil, objects, flowsByOrigin)

	for _, splitID := range candidates {
		flows := findNormalFlows(flowsByOrigin[splitID])
		if _, ok := objects[splitID].(*microflows.ExclusiveSplit); ok && !hasEnumCaseFlows(flows) {
			continue
		}
		join := splitMerge[splitID]
		if _, isMerge := objects[join].(*microflows.ExclusiveMerge); !isMerge {
			continue
		}
		reachesSplit := reachersOf(splitID, pred)

		count := map[model.ID]int{}
		seenArm := map[model.ID]bool{}
		for _, fl := range flows {
			if seenArm[fl.DestinationID] {
				continue // two case values on one arm are one arm
			}
			seenArm[fl.DestinationID] = true
			for id := range reachableUntil(fl.DestinationID, join, succ) {
				if !reachesSplit[id] {
					count[id]++
				}
			}
		}
		shared := map[model.ID]bool{}
		holdsActivity := false
		for id, n := range count {
			if n < 2 {
				continue
			}
			shared[id] = true
			switch objects[id].(type) {
			case *microflows.ExclusiveMerge, nil:
			default:
				holdsActivity = true
			}
		}
		if !holdsActivity {
			continue
		}
		var entries []model.ID
		allMerges := true
		for id := range shared {
			for _, p := range pred[id] {
				if !shared[p] {
					entries = append(entries, id)
					if _, isMerge := objects[id].(*microflows.ExclusiveMerge); !isMerge {
						allMerges = false
					}
					break
				}
			}
		}
		if !allMerges || len(entries) == 0 {
			continue
		}
		for _, id := range entries {
			out[id] = true
		}
		out[join] = true
	}
	return out
}

// sharedHandlerEntries finds the merges two or more error handlers settle on
// that no normal path reaches: one handler body drawn once and wired to several
// activities.
//
// Each guarded activity describes its own `on error … begin … end error` block,
// and the handler walk (collectErrorHandlerStatementSpans) stops at the first
// merge it meets — emitting `join <label>` if the merge has one, and nothing at
// all if it does not. labelRejoinMerges only names merges the normal path also
// reaches, so a merge reached by handlers alone had no name, every block came
// out EMPTY, and the shared handler body — every activity in it — vanished from
// the description (Evora: PrePopulateData.ASU_CheckForWorkforce, three java
// action calls sharing a log + end; GenAICommons.ToolCall_ProcessAndExecuteTool,
// two calls sharing four activities that rejoin the main path).
//
// Treating it as a crossed merge says exactly what the model holds: each block
// ends in `join <label>`, and the body is printed once, in its own section,
// after the main flow. A merge one handler alone settles on is left alone: it
// is not shared, and is not what this describes.
func sharedHandlerEntries(
	col *microflows.MicroflowObjectCollection,
	objects map[model.ID]microflows.MicroflowObject,
) map[model.ID]bool {
	out := map[model.ID]bool{}
	normalSucc := map[model.ID][]model.ID{}
	var errorFlows []*microflows.SequenceFlow
	var startID model.ID
	for _, o := range col.Objects {
		if _, ok := o.(*microflows.StartEvent); ok {
			startID = o.GetID()
		}
	}
	for _, fl := range col.Flows {
		if fl == nil {
			continue
		}
		if fl.IsErrorHandler {
			errorFlows = append(errorFlows, fl)
			continue
		}
		normalSucc[fl.OriginID] = append(normalSucc[fl.OriginID], fl.DestinationID)
	}
	if len(errorFlows) < 2 {
		return out
	}
	normal := reachableUntil(startID, "", normalSucc)
	handlers := map[model.ID]map[model.ID]bool{}
	for _, ef := range errorFlows {
		m := firstMergeFrom(ef.DestinationID, objects, normalSucc)
		if m == "" || normal[m] {
			continue
		}
		if handlers[m] == nil {
			handlers[m] = map[model.ID]bool{}
		}
		handlers[m][ef.OriginID] = true
	}
	for m, origins := range handlers {
		if len(origins) >= 2 {
			out[m] = true
		}
	}
	return out
}

// reachableUntil is every node reachable from start over normal flows without
// passing through stop.
func reachableUntil(start, stop model.ID, succ map[model.ID][]model.ID) map[model.ID]bool {
	seen := map[model.ID]bool{}
	queue := []model.ID{start}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if id == "" || id == stop || seen[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, succ[id]...)
	}
	return seen
}

// reachersOf is every node from which target is reachable, target included.
func reachersOf(target model.ID, pred map[model.ID][]model.ID) map[model.ID]bool {
	seen := map[model.ID]bool{}
	queue := []model.ID{target}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		queue = append(queue, pred[id]...)
	}
	return seen
}
