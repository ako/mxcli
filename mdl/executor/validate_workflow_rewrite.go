// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// checkNoDroppedWorkflowConstructs refuses a CREATE OR REPLACE/MODIFY WORKFLOW
// that would delete a stored construct the statement does not restate.
//
// A rewrite rebuilds the workflow from the statement, so anything the script
// does not mention is gone, and nothing signals the loss afterwards: measured on
// the v1 fixture, a stored interrupting timer boundary event and its whole
// handler flow went 1 -> 0 while exec reported "Created workflow" and exit 0
// (issue #948). That is the same shape as a dropped queue binding
// (checkNoQueuedCalls) — guard-don't-drop, ADR-0005.
//
// Boundary events ARE authorable (`boundary event interrupting timer '…' { … }`),
// so a script that restates them is allowed straight through — that is the normal
// way to edit a workflow that has one. Event sub-processes are not authorable in
// MDL at all, so any stored one refuses the rewrite outright.
//
// The stored side is read from the raw unit rather than through the semantic
// model deliberately: the reader is what was blind here in the first place, and a
// guard that shares the reader's blind spot cannot see what it is meant to
// protect. Reading the BSON also covers constructs no engine models yet.
func checkNoDroppedWorkflowConstructs(ctx *ExecContext, workflowID model.ID, qualifiedName string, stmt *ast.CreateWorkflowStmt) error {
	if ctx == nil || ctx.Backend == nil || workflowID == "" {
		return nil
	}
	raw, err := ctx.Backend.GetRawUnit(workflowID)
	if err != nil {
		// An unreadable stored unit is not this guard's business; the rewrite
		// path reports its own errors.
		return nil
	}

	if n := countRawWorkflowNodes(raw, "EventSubProcess"); n > 0 {
		return mdlerrors.NewUnsupported(fmt.Sprintf(
			"workflow %s has %d event sub-process(es), and MDL cannot express one — "+
				"rewriting the workflow would delete it.\n"+
				"  Edit the workflow in Studio Pro, or use ALTER WORKFLOW to change one activity at a time.",
			qualifiedName, n))
	}

	storedBE := countRawWorkflowNodes(raw, "BoundaryEvent")
	if storedBE == 0 {
		return nil
	}
	authored := countAuthoredBoundaryEvents(stmt.Activities)
	if authored >= storedBE {
		return nil
	}
	return mdlerrors.NewUnsupported(fmt.Sprintf(
		"workflow %s has %d stored boundary event(s) but this statement declares %d — "+
			"rewriting it would delete the difference, along with each one's handler flow.\n"+
			"  Restate them (`boundary event interrupting timer '<expr>' { … }`), which "+
			"`describe workflow %s` now emits, or use ALTER WORKFLOW to change one activity at a time.",
		qualifiedName, storedBE, authored, qualifiedName))
}

// countRawWorkflowNodes counts BSON sub-documents whose $Type contains the given
// marker. Matching on a substring rather than an exact type is deliberate: the
// three timer boundary-event variants and the two event-sub-process start
// activities all differ by prefix, and a variant added later should be caught by
// the guard rather than slip past it.
func countRawWorkflowNodes(v any, marker string) int {
	switch t := v.(type) {
	case map[string]any:
		n := 0
		if s, ok := t["$Type"].(string); ok && strings.Contains(s, marker) {
			n++
		}
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic traversal; the count itself is order-free
		for _, k := range keys {
			if k == "$Type" {
				continue
			}
			n += countRawWorkflowNodes(t[k], marker)
		}
		return n
	case []any:
		n := 0
		for _, e := range t {
			n += countRawWorkflowNodes(e, marker)
		}
		return n
	}
	return 0
}

// countAuthoredBoundaryEvents counts the boundary events the statement declares,
// walking nested flows the same way the rest of workflow validation does.
func countAuthoredBoundaryEvents(activities []ast.WorkflowActivityNode) int {
	n := 0
	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		switch a := act.(type) {
		case *ast.WorkflowUserTaskNode:
			n += len(a.BoundaryEvents)
		case *ast.WorkflowCallMicroflowNode:
			n += len(a.BoundaryEvents)
		case *ast.WorkflowWaitForNotificationNode:
			n += len(a.BoundaryEvents)
		}
	})
	return n
}
