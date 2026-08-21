// SPDX-License-Identifier: Apache-2.0

// Package wfnames holds the workflow activity-name uniqueness policy that every
// backend shares.
//
// Mendix requires activity names to be unique across a whole workflow — nested
// sub-flows included — and reports CE0495 "Duplicate name" when they are not.
// Activity names are frequently *derived* rather than authored (a CALL MICROFLOW
// activity is named after its target microflow), so two calls to the same
// microflow collide before anything has a chance to notice.
//
// The policy lives here rather than in a backend because the backends disagree
// on everything except the policy: the file backends walk BSON to learn which
// names are taken, while the MCP backend learns them from PED reads. Only the
// rename rule is common — and it is the part that must not drift, because a
// backend that skips it writes a model Studio Pro refuses to open cleanly while
// mxcli's own read-back looks fine (issue #945).
package wfnames

import (
	"fmt"

	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// maxSuffix bounds the candidate search. It is a runaway guard, not a real
// limit: a workflow with 1000 same-named activities is a bug upstream.
const maxSuffix = 1000

// Unique returns name if it is free in taken, otherwise the first free
// "<name>_<n>" (n counting from 2), and marks the result as taken. An empty
// name is returned unchanged and never recorded — Mendix generates one for
// activity types that carry no author-visible name.
//
// The taken-set is deliberately not a seen-*count*: counting collides again the
// moment a workflow already contains the suffixed name (names A, A, A_2 count
// their way to A, A_2, A_2).
func Unique(name string, taken map[string]bool) string {
	if name == "" {
		return name
	}
	if !taken[name] {
		taken[name] = true
		return name
	}
	for i := 2; i < maxSuffix; i++ {
		candidate := fmt.Sprintf("%s_%d", name, i)
		if !taken[candidate] {
			taken[candidate] = true
			return candidate
		}
	}
	return name
}

// Dedup renames the given activities — and every activity in the sub-flows they
// carry — so that no name collides with taken or with each other, updating taken
// as it goes.
//
// Seed taken with the names already in the target workflow (the file backends
// collect them from the stored BSON, the MCP backend from PED); an empty map
// deduplicates a fresh set against itself only.
func Dedup(activities []workflows.WorkflowActivity, taken map[string]bool) {
	for _, act := range activities {
		if act == nil {
			continue
		}
		act.SetName(Unique(act.GetName(), taken))
		for _, flow := range SubFlows(act) {
			Dedup(flow.Activities, taken)
		}
	}
}

// SubFlows returns every sub-flow an activity carries: its outcome flows first,
// then its boundary-event flows. The order matches the depth-first, in-order
// traversal DESCRIBE and the mutators' @N positional addressing use, so a
// rename here lands on the activity the author would point at.
//
// An activity type absent from the switch simply has no sub-flows; a new type
// that gains one has to be added here, or its nested activities silently skip
// deduplication.
func SubFlows(act workflows.WorkflowActivity) []*workflows.Flow {
	var out []*workflows.Flow
	add := func(f *workflows.Flow) {
		if f != nil {
			out = append(out, f)
		}
	}
	addConditionOutcomes := func(outcomes []workflows.ConditionOutcome) {
		for _, o := range outcomes {
			if o != nil {
				add(o.GetFlow())
			}
		}
	}
	addBoundaryEvents := func(events []*workflows.BoundaryEvent) {
		for _, e := range events {
			if e != nil {
				add(e.Flow)
			}
		}
	}

	switch a := act.(type) {
	case *workflows.UserTask:
		for _, o := range a.Outcomes {
			if o != nil {
				add(o.Flow)
			}
		}
		addBoundaryEvents(a.BoundaryEvents)
	case *workflows.SystemTask:
		addConditionOutcomes(a.Outcomes)
	case *workflows.CallMicroflowTask:
		addConditionOutcomes(a.Outcomes)
		addBoundaryEvents(a.BoundaryEvents)
	case *workflows.CallWorkflowActivity:
		addBoundaryEvents(a.BoundaryEvents)
	case *workflows.ExclusiveSplitActivity:
		addConditionOutcomes(a.Outcomes)
	case *workflows.ParallelSplitActivity:
		for _, o := range a.Outcomes {
			if o != nil {
				add(o.Flow)
			}
		}
	case *workflows.WaitForNotificationActivity:
		addBoundaryEvents(a.BoundaryEvents)
	}
	return out
}
