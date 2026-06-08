// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

const workflowDocType = "Workflows$Workflow"

// CreateWorkflow creates a workflow document via PED (ped_create_document). The
// workflow's flow is a linear, ordered list of activities (Start … End); coverage
// of activity types grows one at a time, like microflow activities — unmapped
// activity types are rejected with a clear error.
func (b *Backend) CreateWorkflow(wf *workflows.Workflow) error {
	mod, err := b.GetModule(wf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for workflow %q: %w", wf.Name, err)
	}
	content, err := b.mapWorkflow(wf)
	if err != nil {
		return err
	}
	if err := b.ensureSchema(workflowDocType); err != nil {
		return err
	}
	if err := b.pedCreateDocument(mod.Name, workflowDocType, wf.Name, content); err != nil {
		return err
	}
	if wf.ID == "" {
		wf.ID = model.ID("mcp~workflow~" + mod.Name + "~" + wf.Name)
	}
	b.sessionWorkflows = append(b.sessionWorkflows, wf)
	return b.pedCheckDocument(workflowDocType, mod.Name+"."+wf.Name)
}

// UpdateWorkflow (CREATE OR REPLACE on an existing workflow) is not yet supported.
func (b *Backend) UpdateWorkflow(wf *workflows.Workflow) error {
	return fmt.Errorf("CREATE OR REPLACE WORKFLOW %q is not yet supported by the MCP backend", wf.Name)
}

// DeleteWorkflow drops a workflow via Concord's delete_document (PED has no delete
// tool). Requires --mcp-concord.
func (b *Backend) DeleteWorkflow(id model.ID) error {
	wf, err := b.GetWorkflow(id)
	if err != nil {
		return fmt.Errorf("resolve workflow %s for DROP: %w", id, err)
	}
	modName, err := b.moduleNameForContainer(wf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for workflow %q: %w", wf.Name, err)
	}
	return b.concordDeleteDocument(modName, wf.Name)
}

// ListWorkflows merges local (on-disk) workflows with those created this session.
func (b *Backend) ListWorkflows() ([]*workflows.Workflow, error) {
	local, err := b.reader.ListWorkflows()
	if err != nil {
		return nil, err
	}
	if len(b.sessionWorkflows) == 0 {
		return local, nil
	}
	seen := make(map[string]bool, len(b.sessionWorkflows))
	out := make([]*workflows.Workflow, 0, len(local)+len(b.sessionWorkflows))
	for _, w := range b.sessionWorkflows {
		seen[string(w.ContainerID)+"."+w.Name] = true
		out = append(out, w)
	}
	for _, w := range local {
		if !seen[string(w.ContainerID)+"."+w.Name] {
			out = append(out, w)
		}
	}
	return out, nil
}

// GetWorkflow resolves by ID, preferring session-created workflows.
func (b *Backend) GetWorkflow(id model.ID) (*workflows.Workflow, error) {
	for _, w := range b.sessionWorkflows {
		if w.ID == id {
			return w, nil
		}
	}
	return b.reader.GetWorkflow(id)
}

// mapWorkflow maps the executor's Workflow onto the PED Workflows$Workflow content.
func (b *Backend) mapWorkflow(wf *workflows.Workflow) (map[string]any, error) {
	flow, err := b.mapWorkflowFlow(wf.Flow)
	if err != nil {
		return nil, err
	}
	title := wf.WorkflowName
	if title == "" {
		title = wf.Name
	}
	content := map[string]any{
		"name":                wf.Name,
		"documentation":       wf.Documentation,
		"excluded":            wf.Excluded,
		"title":               title,
		"flow":                flow,
		"workflowName":        mapWorkflowStringTemplate(wf.WorkflowName),
		"workflowDescription": mapWorkflowStringTemplate(wf.WorkflowDescription),
		"workflowV2":          false,
	}
	if wf.Parameter != nil {
		content["parameter"] = map[string]any{
			"$Type":  "Workflows$Parameter",
			"entity": wf.Parameter.EntityRef,
			"name":   "WorkflowContext",
		}
	}
	return content, nil
}

func mapWorkflowStringTemplate(text string) map[string]any {
	return map[string]any{"$Type": "Microflows$StringTemplate", "text": text}
}

func (b *Backend) mapWorkflowFlow(flow *workflows.Flow) (map[string]any, error) {
	acts := []any{}
	if flow != nil {
		for _, a := range flow.Activities {
			m, err := mapWorkflowActivity(a)
			if err != nil {
				return nil, err
			}
			acts = append(acts, m)
		}
	}
	return map[string]any{"$Type": "Workflows$Flow", "activities": acts}, nil
}

// mapWorkflowActivity maps one workflow activity. Coverage grows one type at a
// time; unmapped types are rejected rather than silently dropped.
func mapWorkflowActivity(a workflows.WorkflowActivity) (map[string]any, error) {
	switch act := a.(type) {
	case *workflows.StartWorkflowActivity:
		return map[string]any{"$Type": "Workflows$StartWorkflowActivity", "name": act.Name, "caption": act.Caption}, nil
	case *workflows.EndWorkflowActivity:
		return map[string]any{"$Type": "Workflows$EndWorkflowActivity", "name": act.Name, "caption": act.Caption}, nil
	case *workflows.CallMicroflowTask:
		if len(act.BoundaryEvents) > 0 {
			return nil, fmt.Errorf("call-microflow workflow activity %q with boundary events is not yet supported by the MCP backend", act.Name)
		}
		outcomes, err := mapConditionOutcomes(act.Outcomes)
		if err != nil {
			return nil, err
		}
		// PED's element type is CallMicroflowActivity (the on-disk BSON $Type is
		// the older CallMicroflowTask — they differ).
		return map[string]any{
			"$Type":             "Workflows$CallMicroflowActivity",
			"name":              act.Name,
			"caption":           act.Caption,
			"microflow":         act.Microflow,
			"outcomes":          outcomes,
			"parameterMappings": mapWorkflowParamMappings(act.ParameterMappings),
		}, nil
	default:
		return nil, fmt.Errorf("workflow activity type %q is not yet supported by the MCP backend", a.ActivityType())
	}
}

// mapConditionOutcomes maps an activity's condition outcomes; each may carry a
// recursive sub-flow of activities for that branch.
func mapConditionOutcomes(outcomes []workflows.ConditionOutcome) ([]any, error) {
	out := make([]any, 0, len(outcomes))
	for _, oc := range outcomes {
		var m map[string]any
		var flow *workflows.Flow
		switch o := oc.(type) {
		case *workflows.VoidConditionOutcome:
			m = map[string]any{"$Type": "Workflows$VoidConditionOutcome"}
			flow = o.Flow
		case *workflows.BooleanConditionOutcome:
			m = map[string]any{"$Type": "Workflows$BooleanConditionOutcome", "value": o.Value}
			flow = o.Flow
		case *workflows.EnumerationValueConditionOutcome:
			m = map[string]any{"$Type": "Workflows$EnumerationValueConditionOutcome", "value": o.Value}
			flow = o.Flow
		default:
			return nil, fmt.Errorf("workflow outcome type %T is not yet supported by the MCP backend", oc)
		}
		if flow != nil {
			acts := make([]any, 0, len(flow.Activities))
			for _, a := range flow.Activities {
				am, err := mapWorkflowActivity(a)
				if err != nil {
					return nil, err
				}
				acts = append(acts, am)
			}
			m["flow"] = map[string]any{"$Type": "Workflows$Flow", "activities": acts}
		}
		out = append(out, m)
	}
	return out, nil
}

func mapWorkflowParamMappings(pms []*workflows.ParameterMapping) []any {
	out := make([]any, 0, len(pms))
	for _, pm := range pms {
		out = append(out, map[string]any{
			"$Type":      "Workflows$MicroflowCallParameterMapping",
			"expression": pm.Expression,
			"parameter":  pm.Parameter,
		})
	}
	return out
}
