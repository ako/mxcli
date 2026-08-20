// SPDX-License-Identifier: Apache-2.0

// Package microflowgraph analyses the control-flow structure of a microflow.
//
// It answers one question: can this graph be rendered faithfully as nested
// `if/then/else`? MDL's conditional is a single-entry/single-exit block, while a
// Mendix microflow is an arbitrary directed graph. When a branch re-enters a
// sibling branch's path the two regions interleave, and a describer that walks
// the graph as though it were a tree emits MDL that means something else.
//
// That is mendixlabs/mxcli#936's sibling, #923: a microflow whose log activity
// ran on `¬c1 ∨ c2` was described as `c1 ∧ c2` — with the reporter's actual
// expressions, a program that always logs described as one that never does.
//
// # Why this does not reuse the describer's join search
//
// mdl/executor already has two ways to find a split's merge
// (findSplitMergePointsForGraph and commonMergeAfter) and on an irreducible
// graph they disagree — which is the second half of #923, and the reason the
// emitted @merge annotation lands in the wrong place. Reusing either one here
// would inherit whichever is wrong. This package computes post-dominators
// instead: a standard, self-contained definition that owes nothing to the
// describer, so a disagreement between them is a signal rather than a shared
// blind spot.
//
// The package is deliberately dependency-light (sdk/microflows and model only)
// because both mdl/executor and mdl/linter need it, and mdl/executor already
// imports mdl/linter — so the analysis cannot live in either one.
package microflowgraph

import (
	"sort"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// exitID is the virtual exit every terminal node flows into, so that a graph
// with several end events still has a single sink to post-dominate towards.
// Microflow element IDs are UUIDs, so this cannot collide with a real one.
const exitID = model.ID("\x00microflowgraph.exit")

// Classification says how a split's branches relate to one another.
type Classification string

const (
	// Nested — the branch bodies are disjoint and rejoin at one point. MDL's
	// if/then/else renders it faithfully. This is the overwhelmingly common case.
	Nested Classification = "nested"

	// Recombinable — the branches overlap, but the overlap has a single entry:
	// it is a shared suffix reached by more than one branch. Semantics can be
	// preserved by folding the guards into one condition (the region runs on the
	// disjunction of the path conditions) without duplicating any activity.
	Recombinable Classification = "recombinable"

	// Interleaved — the branches overlap at more than one entry point. Nesting
	// this requires either duplicating activities (two nodes where the user drew
	// one) or introducing a boolean variable the user never wrote, per
	// Böhm-Jacopini. Neither is a description, so both are refused.
	Interleaved Classification = "interleaved"
)

// Finding reports one split whose branches do not form disjoint regions.
type Finding struct {
	// SplitID is the ExclusiveSplit or InheritanceSplit that branches.
	SplitID model.ID
	// Split is the object itself, for callers that want its position or caption.
	Split microflows.MicroflowObject
	// Class is Recombinable or Interleaved (a Nested split produces no Finding).
	Class Classification
	// JoinID is the split's immediate post-dominator — the first node every path
	// from the split must pass through. Empty when the branches never rejoin
	// (each returns), in which case the split is Nested by definition.
	JoinID model.ID
	// Overlap holds the nodes reachable from more than one branch before the
	// join, sorted for determinism. These are the nodes a nested rendering
	// cannot place.
	Overlap []model.ID
	// Entries holds the Overlap nodes entered from outside the overlap. One
	// entry means a shared suffix (Recombinable); more means Interleaved.
	Entries []model.ID
	// BranchCount is the number of normal (non-error-handler) outgoing flows.
	BranchCount int
}

// Analyze reports every split in one object collection whose branches overlap.
//
// Nested collections (a loop body) are their own scope and are analysed
// separately: Mendix wires a loop's contents as an independent graph, so a
// LoopedActivity's children never share flows with their parent.
//
// The result is sorted by split ID so callers produce stable output.
func Analyze(objects []microflows.MicroflowObject, flows []*microflows.SequenceFlow) []Finding {
	var out []Finding
	analyzeScope(objects, flows, &out)
	sort.Slice(out, func(i, j int) bool { return out[i].SplitID < out[j].SplitID })
	return out
}

func analyzeScope(objects []microflows.MicroflowObject, flows []*microflows.SequenceFlow, out *[]Finding) {
	nodes := map[model.ID]microflows.MicroflowObject{}
	for _, o := range objects {
		if o == nil {
			continue
		}
		nodes[o.GetID()] = o
		// A loop body is a separate graph with its own flows.
		if loop, ok := o.(*microflows.LoopedActivity); ok && loop.ObjectCollection != nil {
			analyzeScope(loop.ObjectCollection.Objects, loop.ObjectCollection.Flows, out)
		}
	}

	succ := successors(nodes, flows)
	pdom := postDominators(nodes, succ)

	for id, obj := range nodes {
		if !isSplit(obj) {
			continue
		}
		branches := branchTargets(succ, id)
		if len(branches) < 2 {
			continue
		}
		if f, ok := classify(id, obj, branches, succ, pdom); ok {
			*out = append(*out, f)
		}
	}
}

// classify decides whether one split's branch bodies overlap, and how.
func classify(
	splitID model.ID,
	split microflows.MicroflowObject,
	branches []model.ID,
	succ map[model.ID][]model.ID,
	pdom map[model.ID]map[model.ID]bool,
) (Finding, bool) {
	join := immediatePostDominator(splitID, pdom)

	// Each branch's body: what it reaches before the join. The join itself and
	// everything beyond it is the shared continuation and is not part of a body.
	bodies := make([]map[model.ID]bool, 0, len(branches))
	for _, b := range branches {
		bodies = append(bodies, reachableBefore(b, join, succ))
	}

	// Overlap: nodes some pair of branches can both reach before the join.
	overlap := map[model.ID]bool{}
	for i := 0; i < len(bodies); i++ {
		for j := i + 1; j < len(bodies); j++ {
			for id := range bodies[i] {
				if bodies[j][id] {
					overlap[id] = true
				}
			}
		}
	}
	if len(overlap) == 0 {
		return Finding{}, false // properly nested
	}

	// Entry points of the overlap region: overlap nodes reachable from a node
	// outside it. A single entry is a shared suffix — the branches converge once
	// and stay converged, so folding the guards preserves semantics without
	// duplicating anything. Several entries mean the branches genuinely cross.
	entries := map[model.ID]bool{}
	for id := range overlap {
		if hasPredecessorOutside(id, overlap, succ) {
			entries[id] = true
		}
	}

	class := Interleaved
	if len(entries) <= 1 {
		class = Recombinable
	}
	f := Finding{
		SplitID:     splitID,
		Split:       split,
		Class:       class,
		BranchCount: len(branches),
		Overlap:     sortedIDs(overlap),
		Entries:     sortedIDs(entries),
	}
	if join != exitID {
		f.JoinID = join
	}
	return f, true
}

// successors builds the normal-flow adjacency list. Error-handler flows are
// excluded: they are not branches of the split's condition and do not
// participate in structural pairing (the describer excludes them too).
//
// Every node with no normal successor flows into the virtual exit, so that a
// microflow with several end events still post-dominates towards one sink.
func successors(nodes map[model.ID]microflows.MicroflowObject, flows []*microflows.SequenceFlow) map[model.ID][]model.ID {
	succ := map[model.ID][]model.ID{}
	for _, f := range flows {
		if f == nil || f.IsErrorHandler {
			continue
		}
		// Ignore flows pointing outside this scope; a dangling destination would
		// otherwise create a phantom node that post-dominates nothing.
		if _, ok := nodes[f.DestinationID]; !ok {
			continue
		}
		succ[f.OriginID] = append(succ[f.OriginID], f.DestinationID)
	}
	for id := range nodes {
		if len(succ[id]) == 0 {
			succ[id] = []model.ID{exitID}
		}
	}
	succ[exitID] = nil
	return succ
}

// postDominators computes, for each node, the set of nodes every path from it to
// the exit must pass through (itself included).
//
// Iterative dataflow to a fixed point: pdom(n) = {n} ∪ ⋂ pdom(s) over successors.
// Non-exit nodes start at the universe so the intersection converges downward,
// which is what makes this correct on cyclic graphs — a retry loop's back edge
// (issue #281) is an ordinary cycle here and needs no special case.
func postDominators(nodes map[model.ID]microflows.MicroflowObject, succ map[model.ID][]model.ID) map[model.ID]map[model.ID]bool {
	all := make([]model.ID, 0, len(nodes)+1)
	for id := range nodes {
		all = append(all, id)
	}
	all = append(all, exitID)

	pdom := map[model.ID]map[model.ID]bool{}
	for _, id := range all {
		s := map[model.ID]bool{}
		if id == exitID {
			s[exitID] = true
		} else {
			for _, o := range all {
				s[o] = true
			}
		}
		pdom[id] = s
	}

	// Bounded by construction: each pass can only shrink a set, so the number of
	// passes is at most the number of node-set memberships.
	for changed := true; changed; {
		changed = false
		for _, id := range all {
			if id == exitID {
				continue
			}
			var inter map[model.ID]bool
			for _, s := range succ[id] {
				if inter == nil {
					inter = copySet(pdom[s])
					continue
				}
				for k := range inter {
					if !pdom[s][k] {
						delete(inter, k)
					}
				}
			}
			if inter == nil {
				inter = map[model.ID]bool{}
			}
			inter[id] = true
			if !sameSet(inter, pdom[id]) {
				pdom[id] = inter
				changed = true
			}
		}
	}
	return pdom
}

// immediatePostDominator returns the nearest node every path from id passes
// through. Post-dominator sets nest along the tree — pdom(n) = {n} ∪
// pdom(ipdom(n)) — so the nearest is the strict post-dominator with the largest
// set. Ties are broken by ID so the result is deterministic.
func immediatePostDominator(id model.ID, pdom map[model.ID]map[model.ID]bool) model.ID {
	best := exitID
	bestSize := -1
	for cand := range pdom[id] {
		if cand == id {
			continue
		}
		if n := len(pdom[cand]); n > bestSize || (n == bestSize && cand < best) {
			best, bestSize = cand, n
		}
	}
	return best
}

// reachableBefore returns the nodes reachable from start without passing
// through stop. stop itself is excluded: it is the shared continuation, not part
// of any branch body.
func reachableBefore(start, stop model.ID, succ map[model.ID][]model.ID) map[model.ID]bool {
	seen := map[model.ID]bool{}
	if start == stop || start == exitID {
		return seen
	}
	queue := []model.ID{start}
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if n == stop || n == exitID || seen[n] {
			continue
		}
		seen[n] = true
		queue = append(queue, succ[n]...)
	}
	return seen
}

// hasPredecessorOutside reports whether any node outside the region flows into
// id. Scanning succ rather than keeping a reverse index costs nothing at
// microflow scale and keeps one source of adjacency.
func hasPredecessorOutside(id model.ID, region map[model.ID]bool, succ map[model.ID][]model.ID) bool {
	for from, tos := range succ {
		if region[from] {
			continue
		}
		for _, to := range tos {
			if to == id {
				return true
			}
		}
	}
	return false
}

func branchTargets(succ map[model.ID][]model.ID, id model.ID) []model.ID {
	var out []model.ID
	for _, t := range succ[id] {
		if t != exitID {
			out = append(out, t)
		}
	}
	return out
}

func isSplit(o microflows.MicroflowObject) bool {
	switch o.(type) {
	case *microflows.ExclusiveSplit, *microflows.InheritanceSplit:
		return true
	}
	return false
}

func copySet(s map[model.ID]bool) map[model.ID]bool {
	out := make(map[model.ID]bool, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

func sameSet(a, b map[model.ID]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

func sortedIDs(s map[model.ID]bool) []model.ID {
	out := make([]model.ID, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
