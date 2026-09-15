// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/wfnames"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

const workflowDocType = "Workflows$Workflow"

// CreateWorkflow creates a workflow document via PED (ped_create_document). The
// workflow's flow is a linear, ordered list of activities (Start … End); coverage
// of activity types grows one at a time, like microflow activities — unmapped
// activity types are rejected with a clear error.
func (b *Backend) CreateWorkflow(wf *workflows.Workflow) error {
	moduleName, folderPath, err := b.resolveDocContainer(wf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve container for workflow %q: %w", wf.Name, err)
	}
	contextShape := b.workflowConstructorTakesContext()
	content, err := b.mapWorkflowContent(wf, contextShape)
	if err != nil {
		return err
	}
	if b.multiUserTaskConstructorTakesTaskPage() {
		adaptMultiUserTaskPages(content)
	}
	if err := b.ensureSchema(workflowDocType); err != nil {
		return err
	}
	if err := b.pedCreateDocument(moduleName, workflowDocType, wf.Name, content, folderPath); err != nil {
		return err
	}
	if err := b.applyMoreThanHalfFallbacks(moduleName+"."+wf.Name, wf); err != nil {
		return err
	}
	if err := b.applyUserTaskBoundaryEvents(moduleName+"."+wf.Name, wf); err != nil {
		return err
	}
	if contextShape {
		// The context-shaped constructor has no documentation or description;
		// both are leaves of the created element.
		var ops []pedOpEntry
		if wf.Documentation != "" {
			ops = append(ops, pedOpEntry{Path: "/documentation", Operation: pedOperation{Type: "set", Value: wf.Documentation}})
		}
		if wf.WorkflowDescription != "" {
			ops = append(ops, pedOpEntry{Path: "/workflowDescription/text", Operation: pedOperation{Type: "set", Value: wf.WorkflowDescription}})
		}
		if len(ops) > 0 {
			if err := b.pedUpdateDoc(workflowDocType, moduleName+"."+wf.Name, ops...); err != nil {
				return err
			}
		}
	}
	if wf.ID == "" {
		wf.ID = model.ID("mcp~workflow~" + moduleName + "~" + wf.Name)
	}
	b.sessionWorkflows = append(b.sessionWorkflows, wf)
	return b.pedCheckDocument(workflowDocType, moduleName+"."+wf.Name)
}

// UpdateWorkflow handles CREATE OR REPLACE/MODIFY on an existing workflow. PED has
// no document-replace tool, so it rewrites the document in place via
// ped_update_document: the flow's activities are swapped (the new ones are appended,
// then the originals removed — appending first avoids a transient empty flow, which
// PED rejects) and the workflow-level property leaves are set. The document's $ID is
// preserved (the executor passes it through), so references and the git diff stay
// stable. exportLevel and overviewPage are not reapplied (no settable PED path, as
// in ALTER WORKFLOW).
func (b *Backend) UpdateWorkflow(wf *workflows.Workflow) error {
	mod, err := b.GetModule(wf.ContainerID)
	if err != nil {
		return fmt.Errorf("resolve module for workflow %q: %w", wf.Name, err)
	}
	qn := mod.Name + "." + wf.Name

	flowVal, err := b.mapWorkflowFlow(wf.Flow)
	if err != nil {
		return err
	}
	if err := markNotificationEventsInSplits(flowVal, false); err != nil {
		return err
	}
	esps, err := mapEventSubProcesses(wf.EventSubProcesses)
	if err != nil {
		return err
	}
	// Replace only the *middle* activities: PED keeps the workflow's structural
	// Start/End (it refuses to remove them) and auto-positions an index-less add by
	// activity type, so we strip the executor's leading Start / trailing End and
	// let PED place the new middles between the existing ones.
	if b.multiUserTaskConstructorTakesTaskPage() {
		adaptMultiUserTaskPages(flowVal)
	}
	middles := stripStartEnd(flowVal["activities"].([]any))

	n, err := b.workflowActivityCount(qn)
	if err != nil {
		return err
	}
	storedHandlers, err := b.workflowListCount(qn, "/onWorkflowEvent")
	if err != nil {
		return err
	}

	var ops []pedOpEntry
	// Drop the original middle activities (indices 1..n-2, high→low; index 0 is
	// Start and n-1 is End, both preserved — PED refuses to remove either). Then
	// insert the new middles just after Start. Each insert targets index 1 (always
	// valid, since Start holds index 0) — an index-less add appends *after* End, and
	// an explicit incrementing index is validated against the array's *original*
	// length so it can't grow the flow. Inserting in REVERSE order at index 1
	// leaves the middles in their intended sequence.
	for i := n - 2; i >= 1; i-- {
		ops = append(ops, removeAtOp("/flow/activities", i))
	}
	for i := len(middles) - 1; i >= 0; i-- {
		afterStart := 1
		ops = append(ops, pedOpEntry{Path: "/flow/activities", Operation: pedOperation{Type: "add", Value: middles[i], Index: &afterStart}})
	}

	// Event handlers: replace the stored list with the statement's. The executor's
	// rewrite guard has already refused a statement declaring fewer than are stored.
	for i := storedHandlers - 1; i >= 0; i-- {
		ops = append(ops, removeAtOp("/onWorkflowEvent", i))
	}
	for _, h := range mapWorkflowEventHandlers(wf.EventHandlers) {
		ops = append(ops, pedOpEntry{Path: "/onWorkflowEvent", Operation: pedOperation{Type: "add", Value: h}})
	}

	// Event sub-processes: replaced whole, like the handlers — the rewrite guard
	// has already refused a statement declaring fewer than are stored. Removing a
	// container (not its start event) is the one-call form Studio Pro's
	// workflow-update skill allows.
	if storedESPs, err := b.workflowListCount(qn, "/eventSubProcesses"); err != nil {
		return err
	} else {
		for i := storedESPs - 1; i >= 0; i-- {
			ops = append(ops, removeAtOp("/eventSubProcesses", i))
		}
	}
	for _, e := range esps {
		ops = append(ops, pedOpEntry{Path: "/eventSubProcesses", Operation: pedOperation{Type: "add", Value: e}})
	}

	title := wf.WorkflowName
	if title == "" {
		title = wf.Name
	}
	set := func(path string, v any) {
		ops = append(ops, pedOpEntry{Path: path, Operation: pedOperation{Type: "set", Value: v}})
	}
	set("/title", title)
	set("/workflowName/text", wf.WorkflowName)
	set("/workflowDescription/text", wf.WorkflowDescription)
	set("/dueDate", wf.DueDate)
	set("/documentation", wf.Documentation)
	if wf.Parameter != nil {
		set("/parameter/entity", wf.Parameter.EntityRef)
	}

	if err := b.pedUpdateDoc(workflowDocType, qn, ops...); err != nil {
		return err
	}
	if err := b.applyMoreThanHalfFallbacks(qn, wf); err != nil {
		return err
	}
	if err := b.applyUserTaskBoundaryEvents(qn, wf); err != nil {
		return err
	}
	b.markDirty(mod.Name)
	b.upsertSessionWorkflow(wf)
	return b.pedCheckDocument(workflowDocType, qn)
}

// stripStartEnd drops a leading StartWorkflowActivity and trailing
// EndWorkflowActivity from a mapped activity list, leaving the middle activities.
func stripStartEnd(acts []any) []any {
	activityType := func(a any) string {
		m, _ := a.(map[string]any)
		s, _ := m["$Type"].(string)
		return s
	}
	if len(acts) > 0 && activityType(acts[0]) == "Workflows$StartWorkflowActivity" {
		acts = acts[1:]
	}
	if len(acts) > 0 && activityType(acts[len(acts)-1]) == "Workflows$EndWorkflowActivity" {
		acts = acts[:len(acts)-1]
	}
	return acts
}

// workflowActivityCount returns the number of top-level activities in a workflow's flow.
func (b *Backend) workflowActivityCount(qn string) (int, error) {
	return b.workflowListCount(qn, "/flow/activities")
}

// workflowListCount returns the number of elements in a list property of a
// stored workflow, addressed by JSON pointer.
func (b *Backend) workflowListCount(qn, path string) (int, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": workflowDocType,
		"documentName": qn,
		"paths":        []string{path},
	})
	if err != nil {
		return 0, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return 0, fmt.Errorf("read %s flow: %s", qn, text)
	}
	var doc struct {
		Results []struct {
			Result []json.RawMessage `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil || len(doc.Results) == 0 {
		return 0, fmt.Errorf("parse %s flow: %v", qn, err)
	}
	return len(doc.Results[0].Result), nil
}

// upsertSessionWorkflow records (or replaces) a workflow in the session list so
// later reads/ALTERs in the same run see the new content rather than the stale .mpr.
func (b *Backend) upsertSessionWorkflow(wf *workflows.Workflow) {
	for i, w := range b.sessionWorkflows {
		if w.ID == wf.ID {
			b.sessionWorkflows[i] = wf
			return
		}
	}
	b.sessionWorkflows = append(b.sessionWorkflows, wf)
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

// workflowConstructorTakesContext reports whether the connected server's
// Workflows$Workflow constructor takes the context entity as `context` with a
// plain-string workflowName and caption. Studio Pro 11.14 does, and rejects the
// older shape outright — measured live: `"/context":"Expected reference (string),
// got undefined","/workflowName":"Expected string, got object"`, so every workflow
// create over MCP failed. Which release changed it is not established, so the
// shape is read off the live constructor schema rather than a version number.
// The probe doubles as the schema fetch PED asks for before a create.
func (b *Backend) workflowConstructorTakesContext() bool {
	if b.workflowCtorContext != nil {
		return *b.workflowCtorContext
	}
	takes := false
	if b.client != nil {
		res, err := b.client.CallTool("ped_get_schema", map[string]any{"elementTypes": []string{workflowDocType}})
		if err == nil && res != nil && !res.IsError {
			takes = strings.Contains(res.Text, "context: Reference<'DomainModels$Entity'")
			if b.schemaFetched == nil {
				b.schemaFetched = map[string]bool{}
			}
			b.schemaFetched[workflowDocType] = true
		}
	}
	b.workflowCtorContext = &takes
	return takes
}

// multiUserTaskConstructorTakesTaskPage reports whether the connected server's
// Workflows$MultiUserTaskActivity constructor takes the page as a `taskPage`
// element. Studio Pro 11.14 does, and rejects the bare `pageReference` string the
// mapper sends for older servers — measured live:
// `"/flow/activities/1/taskPage":"Expected an object, but the value is missing."`,
// so no multi-user task could be created over MCP. Read off the live constructor
// schema, like workflowConstructorTakesContext; the probe doubles as the schema
// fetch PED asks for before a create.
func (b *Backend) multiUserTaskConstructorTakesTaskPage() bool {
	if b.multiTaskPageCtor != nil {
		return *b.multiTaskPageCtor
	}
	const multiTaskType = "Workflows$MultiUserTaskActivity"
	takes := false
	if b.client != nil {
		res, err := b.client.CallTool("ped_get_schema", map[string]any{"elementTypes": []string{multiTaskType}})
		if err == nil && res != nil && !res.IsError {
			takes = strings.Contains(res.Text, "taskPage: Element<'Workflows$PageReference'>")
			if b.schemaFetched == nil {
				b.schemaFetched = map[string]bool{}
			}
			b.schemaFetched[multiTaskType] = true
		}
	}
	b.multiTaskPageCtor = &takes
	return takes
}

// adaptMultiUserTaskPages rewrites, at any depth, each mapped multi-user task's
// bare `pageReference` into the `taskPage` element the 11.14 constructor takes.
func adaptMultiUserTaskPages(v any) {
	switch t := v.(type) {
	case map[string]any:
		if t["$Type"] == "Workflows$MultiUserTaskActivity" {
			if page, ok := t["pageReference"].(string); ok {
				delete(t, "pageReference")
				t["taskPage"] = map[string]any{"$Type": "Workflows$PageReference", "page": page}
			}
		}
		for _, c := range t {
			adaptMultiUserTaskPages(c)
		}
	case []any:
		for _, c := range t {
			adaptMultiUserTaskPages(c)
		}
	}
}

// mapWorkflow maps the executor's Workflow onto the PED Workflows$Workflow content
// in the parameter-element shape.
func (b *Backend) mapWorkflow(wf *workflows.Workflow) (map[string]any, error) {
	return b.mapWorkflowContent(wf, false)
}

// mapWorkflowContent maps the executor's Workflow onto PED constructor content,
// in the context shape (Studio Pro 11.14: `context`, `caption`, string
// `workflowName`) or the parameter-element shape older servers take.
func (b *Backend) mapWorkflowContent(wf *workflows.Workflow, contextShape bool) (map[string]any, error) {
	flow, err := b.mapWorkflowFlow(wf.Flow)
	if err != nil {
		return nil, err
	}
	if err := markNotificationEventsInSplits(flow, false); err != nil {
		return nil, err
	}
	esps, err := mapEventSubProcesses(wf.EventSubProcesses)
	if err != nil {
		return nil, err
	}
	content, err := b.mapWorkflowContentShape(wf, flow, contextShape)
	if err != nil {
		return nil, err
	}
	if esps != nil {
		content["eventSubProcesses"] = esps
	}
	return content, nil
}

// mapEventSubProcesses maps the workflow's event sub-processes onto the
// constructor's `eventSubProcesses` (ped_get_schema, Studio Pro 11.14). Each flow
// is sent whole — its start event first — and nil for none so the key is omitted.
func mapEventSubProcesses(esps []*workflows.EventSubProcess) ([]any, error) {
	if len(esps) == 0 {
		return nil, nil
	}
	out := make([]any, 0, len(esps))
	for _, esp := range esps {
		fm, err := mapWorkflowFlowValue(esp.Flow)
		if err != nil {
			return nil, err
		}
		if fm == nil {
			fm = map[string]any{"$Type": "Workflows$Flow", "activities": []any{}}
		}
		if err := markNotificationEventsInSplits(fm, false); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"$Type":   "Workflows$EventSubProcess",
			"name":    esp.Name,
			"caption": esp.Caption,
			"flow":    fm,
		})
	}
	return out, nil
}

// markNotificationEventsInSplits sets `isInsideOfParallelSplit`, which PED's
// interrupting notification boundary event constructor requires, on every such
// event in a mapped flow: true when it sits under a parallel split at any depth.
//
// It also refuses a notification boundary path the constructor would rewrite. Its
// schema says, and Studio Pro 11.14 does, that an interrupting event's path is
// forced to end in an End — or in a jump inside a split — by removing the other
// terminator and appending its own; a non-interrupting path must end in the
// end-of-path marker. Measured: a `jump to` outside a split is stored as an End,
// and a path ending in the marker gets a second terminator appended and is
// refused. mxbuild builds all of these, so the rules are Studio Pro's; each shape
// is refused here with the remedy rather than rewritten or rejected after sending.
func markNotificationEventsInSplits(v any, inSplit bool) error {
	switch t := v.(type) {
	case map[string]any:
		typ, _ := t["$Type"].(string)
		switch typ {
		case "Workflows$InterruptingNotificationBoundaryEvent":
			t["isInsideOfParallelSplit"] = inSplit
			switch last := mappedFlowLastType(t["flow"]); {
			case inSplit && last != "Workflows$JumpToActivity":
				return fmt.Errorf("boundary event %v: over MCP, Studio Pro requires an interrupting notification boundary event inside a "+
					"parallel split to end its path with `jump to <activity>` — its constructor appends a jump with no target otherwise", t["name"])
			case !inSplit && last != "Workflows$EndWorkflowActivity":
				return fmt.Errorf("boundary event %v: over MCP, Studio Pro requires an interrupting notification boundary event outside a "+
					"parallel split to end its path with `end workflow;` — its constructor replaces a jump with an End, and refuses a path ending otherwise", t["name"])
			}
		case "Workflows$NonInterruptingNotificationBoundaryEvent":
			if last := mappedFlowLastType(t["flow"]); last != "Workflows$EndOfBoundaryEventPathActivity" {
				return fmt.Errorf("boundary event %v: over MCP, Studio Pro requires a non-interrupting notification boundary event's path "+
					"to run to its end — remove the final `jump to`", t["name"])
			}
		}
		childInSplit := inSplit || typ == "Workflows$ParallelSplitActivity"
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := markNotificationEventsInSplits(t[k], childInSplit); err != nil {
				return err
			}
		}
	case []any:
		for _, e := range t {
			if err := markNotificationEventsInSplits(e, inSplit); err != nil {
				return err
			}
		}
	}
	return nil
}

// mappedFlowLastType returns the $Type of a mapped flow's last activity, "" for none.
func mappedFlowLastType(flow any) string {
	f, _ := flow.(map[string]any)
	acts, _ := f["activities"].([]any)
	if len(acts) == 0 {
		return ""
	}
	last, _ := acts[len(acts)-1].(map[string]any)
	typ, _ := last["$Type"].(string)
	return typ
}

// taskBoundaryEvents is a single user task's boundary events, found at a PED path.
type taskBoundaryEvents struct {
	events  []*workflows.BoundaryEvent
	inSplit bool
}

// singleUserTaskBoundaryEvents collects, by PED path, every single user task that
// carries boundary events, at any depth of a flow.
func singleUserTaskBoundaryEvents(flow *workflows.Flow, base string, inSplit bool, out map[string]taskBoundaryEvents) {
	if flow == nil {
		return
	}
	for i, a := range flow.Activities {
		path := fmt.Sprintf("%s/%d", base, i)
		boundary := func(events []*workflows.BoundaryEvent) {
			for j, be := range events {
				if be != nil {
					singleUserTaskBoundaryEvents(be.Flow, fmt.Sprintf("%s/boundaryEvents/%d/flow/activities", path, j), inSplit, out)
				}
			}
		}
		conditions := func(outcomes []workflows.ConditionOutcome) {
			for j, o := range outcomes {
				if o != nil {
					singleUserTaskBoundaryEvents(o.GetFlow(), fmt.Sprintf("%s/outcomes/%d/flow/activities", path, j), inSplit, out)
				}
			}
		}
		switch t := a.(type) {
		case *workflows.UserTask:
			var notification []*workflows.BoundaryEvent
			for _, be := range t.BoundaryEvents {
				if be != nil && be.IsNotification() {
					notification = append(notification, be)
				}
			}
			if !t.IsMulti && len(notification) > 0 {
				out[path] = taskBoundaryEvents{events: notification, inSplit: inSplit}
			}
			for j, o := range t.Outcomes {
				if o != nil {
					singleUserTaskBoundaryEvents(o.Flow, fmt.Sprintf("%s/outcomes/%d/flow/activities", path, j), inSplit, out)
				}
			}
			boundary(t.BoundaryEvents)
		case *workflows.CallMicroflowTask:
			conditions(t.Outcomes)
			boundary(t.BoundaryEvents)
		case *workflows.ExclusiveSplitActivity:
			conditions(t.Outcomes)
		case *workflows.ParallelSplitActivity:
			for j, o := range t.Outcomes {
				if o != nil {
					singleUserTaskBoundaryEvents(o.Flow, fmt.Sprintf("%s/outcomes/%d/flow/activities", path, j), true, out)
				}
			}
		case *workflows.CallWorkflowActivity:
			boundary(t.BoundaryEvents)
		case *workflows.WaitForNotificationActivity:
			boundary(t.BoundaryEvents)
		}
	}
}

// applyUserTaskBoundaryEvents adds the notification boundary events Studio Pro
// dropped from single user tasks. Measured on Studio Pro 11.14: the single user
// task constructor drops `boundaryEvents` — timer and notification alike, at
// create and when an update adds the task — and reports no error; a multi-user
// task keeps them, and a notification event added afterwards at the task's
// `boundaryEvents` is stored. Timer events are not re-added: the timer
// constructor has no firstExecutionTime and rewrites the path's terminator, so a
// re-add would fail on those instead. Only an empty stored list is filled, so a
// server that keeps them gets nothing added twice.
func (b *Backend) applyUserTaskBoundaryEvents(qn string, wf *workflows.Workflow) error {
	found := map[string]taskBoundaryEvents{}
	singleUserTaskBoundaryEvents(wf.Flow, "/flow/activities", false, found)
	for i, esp := range wf.EventSubProcesses {
		if esp != nil {
			singleUserTaskBoundaryEvents(esp.Flow, fmt.Sprintf("/eventSubProcesses/%d/flow/activities", i), false, found)
		}
	}
	paths := make([]string, 0, len(found))
	for p := range found {
		paths = append(paths, p)
	}
	sort.Strings(paths) // a task before anything nested in its own events
	for _, path := range paths {
		stored, err := b.workflowListCount(qn, path+"/boundaryEvents")
		if err != nil {
			return err
		}
		if stored > 0 {
			continue
		}
		mapped, err := mapBoundaryEvents(found[path].events)
		if err != nil {
			return err
		}
		ops := make([]pedOpEntry, 0, len(mapped))
		for _, e := range mapped {
			if err := markNotificationEventsInSplits(e, found[path].inSplit); err != nil {
				return err
			}
			ops = append(ops, pedOpEntry{Path: path + "/boundaryEvents", Operation: pedOperation{Type: "add", Value: e}})
		}
		if err := b.pedUpdateDoc(workflowDocType, qn, ops...); err != nil {
			return err
		}
	}
	return nil
}

func (b *Backend) mapWorkflowContentShape(wf *workflows.Workflow, flow map[string]any, contextShape bool) (map[string]any, error) {
	if contextShape {
		title := wf.WorkflowName
		if title == "" {
			title = wf.Name
		}
		content := map[string]any{
			"name":         wf.Name,
			"caption":      title,
			"workflowName": wf.WorkflowName,
			"flow":         flow,
		}
		if wf.Parameter != nil {
			content["context"] = wf.Parameter.EntityRef
		}
		if len(wf.EventHandlers) > 0 {
			content["onWorkflowEvent"] = mapWorkflowEventHandlers(wf.EventHandlers)
		}
		return content, nil
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
	if len(wf.EventHandlers) > 0 {
		content["onWorkflowEvent"] = mapWorkflowEventHandlers(wf.EventHandlers)
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
	if flow == nil {
		return map[string]any{"$Type": "Workflows$Flow", "activities": []any{}}, nil
	}
	return mapWorkflowFlowValue(flow)
}

// mapWorkflowFlowValue maps a Workflows$Flow (the top-level flow or an outcome's
// sub-flow) onto its pg form, recursing into each activity. Returns nil for a nil
// flow so callers can omit the key.
func mapWorkflowFlowValue(flow *workflows.Flow) (map[string]any, error) {
	if flow == nil {
		return nil, nil
	}
	acts := make([]any, 0, len(flow.Activities))
	for _, a := range flow.Activities {
		m, err := mapWorkflowActivity(a)
		if err != nil {
			return nil, err
		}
		acts = append(acts, m)
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
	// The end-of-path markers are sent like End: every parallel split path
	// carries one, and without it the engine skips the path's contents.
	case *workflows.EndOfParallelSplitPathActivity:
		return map[string]any{"$Type": "Workflows$EndOfParallelSplitPathActivity", "name": act.Name, "caption": act.Caption}, nil
	case *workflows.EndOfBoundaryEventPathActivity:
		return map[string]any{"$Type": "Workflows$EndOfBoundaryEventPathActivity", "name": act.Name, "caption": act.Caption}, nil
	case *workflows.NotificationActivity:
		return map[string]any{"$Type": "Workflows$NotificationActivity", "name": act.Name, "caption": act.Caption}, nil
	case *workflows.EventSubProcessStartActivity:
		m := map[string]any{"$Type": act.StorageType(), "name": act.Name, "caption": act.Caption}
		if act.Timer {
			m["firstExecutionTime"] = act.FirstExecutionTime
		}
		return m, nil
	case *workflows.CallMicroflowTask:
		outcomes, err := mapConditionOutcomes(act.Outcomes)
		if err != nil {
			return nil, err
		}
		bevents, err := mapBoundaryEvents(act.BoundaryEvents)
		if err != nil {
			return nil, err
		}
		// PED's element type is CallMicroflowActivity (the on-disk BSON $Type is
		// the older CallMicroflowTask — they differ).
		// An AI agent task has the same PED shape under its own element type
		// (ped_get_schema, Studio Pro 11.14: name, caption, microflow, outcomes,
		// parameterMappings, boundaryEvents).
		elementType := "Workflows$CallMicroflowActivity"
		if act.IsAgent {
			elementType = "Workflows$AIAgentTaskActivity"
		}
		m := map[string]any{
			"$Type":             elementType,
			"name":              act.Name,
			"caption":           act.Caption,
			"microflow":         act.Microflow,
			"outcomes":          outcomes,
			"parameterMappings": mapWorkflowParamMappings(act.ParameterMappings),
		}
		if bevents != nil {
			m["boundaryEvents"] = bevents
		}
		return m, nil
	case *workflows.CallWorkflowActivity:
		bevents, err := mapBoundaryEvents(act.BoundaryEvents)
		if err != nil {
			return nil, err
		}
		// PED's CallWorkflowActivity has no parameterExpression — Studio Pro binds
		// the $WorkflowContext implicitly; only explicit parameterMappings are sent.
		m := map[string]any{
			"$Type":             "Workflows$CallWorkflowActivity",
			"name":              act.Name,
			"caption":           act.Caption,
			"workflow":          act.Workflow,
			"parameterMappings": mapWorkflowParamMappings(act.ParameterMappings),
		}
		if bevents != nil {
			m["boundaryEvents"] = bevents
		}
		return m, nil
	case *workflows.WaitForNotificationActivity:
		bevents, err := mapBoundaryEvents(act.BoundaryEvents)
		if err != nil {
			return nil, err
		}
		m := map[string]any{
			"$Type":   "Workflows$WaitForNotificationActivity",
			"name":    act.Name,
			"caption": act.Caption,
		}
		if bevents != nil {
			m["boundaryEvents"] = bevents
		}
		return m, nil
	case *workflows.WaitForTimerActivity:
		return map[string]any{
			"$Type":   "Workflows$WaitForTimerActivity",
			"name":    act.Name,
			"caption": act.Caption,
			"delay":   act.DelayExpression,
		}, nil
	case *workflows.JumpToActivity:
		return map[string]any{
			"$Type":          "Workflows$JumpToActivity",
			"name":           act.Name,
			"caption":        act.Caption,
			"targetActivity": act.TargetActivity,
		}, nil
	case *workflows.ParallelSplitActivity:
		outcomes := make([]any, 0, len(act.Outcomes))
		for _, oc := range act.Outcomes {
			m := map[string]any{"$Type": "Workflows$ParallelSplitOutcome"}
			if fm, err := mapWorkflowFlowValue(oc.Flow); err != nil {
				return nil, err
			} else if fm != nil {
				m["flow"] = fm
			}
			outcomes = append(outcomes, m)
		}
		return map[string]any{
			"$Type":    "Workflows$ParallelSplitActivity",
			"name":     act.Name,
			"caption":  act.Caption,
			"outcomes": outcomes,
		}, nil
	case *workflows.ExclusiveSplitActivity:
		outcomes, err := mapConditionOutcomes(act.Outcomes)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"$Type":      "Workflows$ExclusiveSplitActivity",
			"name":       act.Name,
			"caption":    act.Caption,
			"expression": act.Expression,
			"outcomes":   outcomes,
		}, nil
	case *workflows.UserTask:
		bevents, err := mapBoundaryEvents(act.BoundaryEvents)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		if act.IsMulti {
			// MultiUserTaskActivity differs from the single variant: the page is a
			// bare string (pageReference, not a taskPage element), there is no
			// taskName/taskDescription/dueDate, participiantInput is required, and
			// outcomes are plain value strings (not UserTaskOutcome elements — so a
			// multi-task outcome carries no per-outcome sub-flow). Completion
			// defaults (Consensus, Percentage threshold) are supplied by PED.
			vals := make([]any, 0, len(act.Outcomes))
			for _, oc := range act.Outcomes {
				v := oc.Value
				if v == "" {
					v = oc.Caption
				}
				if oc.Flow != nil && len(oc.Flow.Activities) > 0 {
					return nil, fmt.Errorf("multi user task %q outcome %q: per-outcome sub-flows are not supported via the MCP backend (PED models multi-task outcomes as plain values)", act.Name, v)
				}
				vals = append(vals, v)
			}
			m = map[string]any{
				"$Type":             "Workflows$MultiUserTaskActivity",
				"name":              act.Name,
				"caption":           act.Caption,
				"pageReference":     act.Page,
				"participiantInput": "AllTargetUsers",
				"userTargeting":     mapUserTargeting(act.UserSource),
				"onCreatedEvent":    mapOnCreatedEvent(act.OnCreated),
				"outcomes":          vals,
			}
			mapMultiUserTaskCompletion(m, act, vals)
		} else {
			outcomes, err := mapUserTaskOutcomes(act.Outcomes)
			if err != nil {
				return nil, err
			}
			m = map[string]any{
				"$Type":                      "Workflows$SingleUserTaskActivity",
				"name":                       act.Name,
				"caption":                    act.Caption,
				"taskPage":                   map[string]any{"$Type": "Workflows$PageReference", "page": act.Page},
				"taskName":                   mapWorkflowStringTemplate(act.TaskName),
				"taskDescription":            mapWorkflowStringTemplate(act.TaskDescription),
				"dueDate":                    act.DueDate,
				"userTargeting":              mapUserTargeting(act.UserSource),
				"onCreatedEvent":             mapOnCreatedEvent(act.OnCreated),
				"outcomes":                   outcomes,
				"autoAssignSingleTargetUser": false,
			}
		}
		if bevents != nil {
			m["boundaryEvents"] = bevents
		}
		return m, nil
	default:
		return nil, fmt.Errorf("workflow activity type %q is not yet supported by the MCP backend", a.ActivityType())
	}
}

// mapMultiUserTaskCompletion sets a multi-user task constructor's participant and
// completion fields (ped_get_schema, Studio Pro 11.14). The constructor names
// outcomes by value. With no rule it sends consensus falling back to the first
// outcome — what the file engine writes — because PED's own default is consensus
// with no fallback, which is CE1866.
func mapMultiUserTaskCompletion(m map[string]any, act *workflows.UserTask, outcomes []any) {
	if t := act.TargetUserInput; t != nil {
		switch t.Kind {
		case "Absolute":
			m["participiantInput"] = "AbsoluteNumber"
			m["participiantValue"] = t.Amount
		case "Percentage":
			m["participiantInput"] = "Percentage"
			m["participiantValue"] = t.Percentage
		}
	}
	if act.AwaitAllUsers {
		m["awaitAllUsers"] = true
	}
	cc := act.CompletionCriteria
	if cc == nil {
		m["completionCriteria"] = "Consensus"
		if len(outcomes) > 0 {
			m["fallbackOutcome"] = outcomes[0]
		}
		return
	}
	setFallback := func() {
		if cc.FallbackOutcome != "" {
			m["fallbackOutcome"] = cc.FallbackOutcome
		}
	}
	switch cc.Kind {
	case "Majority":
		m["completionCriteria"] = "Majority"
		m["majorityType"] = "MostChosen"
		if cc.CompletionType == "Absolute" {
			m["majorityType"] = "MoreThanHalf"
		}
		setFallback()
	case "Threshold":
		m["completionCriteria"] = "Threshold"
		m["thresholdType"] = "AbsoluteNumber"
		if cc.CompletionType == "Relative" {
			m["thresholdType"] = "Percentage"
		}
		m["thresholdValue"] = cc.Threshold
		setFallback()
	case "Veto":
		m["completionCriteria"] = "Veto"
		m["vetoOutcome"] = cc.VetoOutcome
	case "Microflow":
		m["completionCriteria"] = "Microflow"
		m["microflow"] = cc.Microflow
	default:
		m["completionCriteria"] = "Consensus"
		setFallback()
	}
}

// moreThanHalfFallbacks lists, by JSON path, every multi-user task in a flow that
// decides by majority-more-than-half with a fallback, and that fallback's outcome
// index. PED's constructor drops `fallbackOutcome` for MoreThanHalf (measured,
// Studio Pro 11.14: stored null, then "Fallback outcome cannot be empty"), so it
// is set afterwards by the outcome's $ID.
func moreThanHalfFallbacks(flow *workflows.Flow, base string) map[string]int {
	out := map[string]int{}
	if flow == nil {
		return out
	}
	for i, a := range flow.Activities {
		path := fmt.Sprintf("%s/%d", base, i)
		sub := func(f *workflows.Flow, p string) {
			for k, v := range moreThanHalfFallbacks(f, p) {
				out[k] = v
			}
		}
		switch t := a.(type) {
		case *workflows.UserTask:
			if cc := t.CompletionCriteria; t.IsMulti && cc != nil && cc.Kind == "Majority" && cc.CompletionType == "Absolute" && cc.FallbackOutcome != "" {
				for j, o := range t.Outcomes {
					if o.Value == cc.FallbackOutcome || (o.Value == "" && o.Caption == cc.FallbackOutcome) {
						out[path] = j
					}
				}
			}
			for j, o := range t.Outcomes {
				sub(o.Flow, fmt.Sprintf("%s/outcomes/%d/flow/activities", path, j))
			}
			for j, be := range t.BoundaryEvents {
				sub(be.Flow, fmt.Sprintf("%s/boundaryEvents/%d/flow/activities", path, j))
			}
		case *workflows.CallMicroflowTask:
			for j, o := range t.Outcomes {
				sub(o.GetFlow(), fmt.Sprintf("%s/outcomes/%d/flow/activities", path, j))
			}
			for j, be := range t.BoundaryEvents {
				sub(be.Flow, fmt.Sprintf("%s/boundaryEvents/%d/flow/activities", path, j))
			}
		case *workflows.ExclusiveSplitActivity:
			for j, o := range t.Outcomes {
				sub(o.GetFlow(), fmt.Sprintf("%s/outcomes/%d/flow/activities", path, j))
			}
		case *workflows.ParallelSplitActivity:
			for j, o := range t.Outcomes {
				if o != nil {
					sub(o.Flow, fmt.Sprintf("%s/outcomes/%d/flow/activities", path, j))
				}
			}
		}
	}
	return out
}

// applyMoreThanHalfFallbacks sets each majority-more-than-half fallback the
// constructor dropped, reading the outcome's $ID and pointing at it.
func (b *Backend) applyMoreThanHalfFallbacks(qn string, wf *workflows.Workflow) error {
	for path, idx := range moreThanHalfFallbacks(wf.Flow, "/flow/activities") {
		idPath := fmt.Sprintf("%s/outcomes/%d/$ID", path, idx)
		res, err := b.client.CallTool("ped_read_document", map[string]any{
			"documentType": workflowDocType,
			"documentName": qn,
			"paths":        []string{idPath},
		})
		if err != nil {
			return err
		}
		text := pedStripReminder(res.Text)
		var doc struct {
			Results []struct {
				Result string `json:"result"`
			} `json:"results"`
		}
		if res.IsError || json.Unmarshal([]byte(text), &doc) != nil || len(doc.Results) == 0 || doc.Results[0].Result == "" {
			return fmt.Errorf("read outcome id at %s of %s: %s", idPath, qn, text)
		}
		op := pedOpEntry{Path: path + "/completionCriteria/fallbackOutcome", Operation: pedOperation{Type: "set", Value: doc.Results[0].Result}}
		if err := b.pedUpdateDoc(workflowDocType, qn, op); err != nil {
			return err
		}
	}
	return nil
}

// mapOnCreatedEvent maps a user task's on-created microflow onto its PED element:
// Workflows$MicroflowBasedEvent when one is set, Workflows$NoEvent otherwise. This
// was hard-coded to NoEvent, so an on-created microflow written over MCP was
// silently dropped.
func mapOnCreatedEvent(microflow string) map[string]any {
	if microflow == "" {
		return map[string]any{"$Type": "Workflows$NoEvent"}
	}
	return map[string]any{"$Type": "Workflows$MicroflowBasedEvent", "microflow": microflow}
}

// mapWorkflowEventHandlers maps a workflow's event handlers onto PED
// Workflows$WorkflowEventHandler elements. PED types description as
// MinLength<1, 'MW0006'>: Studio Pro warns on a handler without one.
func mapWorkflowEventHandlers(handlers []*workflows.WorkflowEventHandler) []any {
	out := make([]any, 0, len(handlers))
	for _, h := range handlers {
		if h == nil {
			continue
		}
		types := make([]any, 0, len(h.EventTypes))
		for _, t := range h.EventTypes {
			types = append(types, t)
		}
		out = append(out, map[string]any{
			"$Type":         "Workflows$WorkflowEventHandler",
			"description":   h.Description,
			"documentation": h.Documentation,
			"eventTypes":    types,
			"microflowEventHandler": map[string]any{
				"$Type":     "Workflows$MicroflowEventHandler",
				"microflow": h.Microflow,
			},
		})
	}
	return out
}

// mapUserTargeting maps a user task's user source onto its pg targeting element.
func mapUserTargeting(source workflows.UserSource) map[string]any {
	switch s := source.(type) {
	case *workflows.XPathBasedUserSource:
		return map[string]any{"$Type": "Workflows$XPathUserTargeting", "xPathConstraint": s.XPath}
	case *workflows.MicroflowBasedUserSource:
		return map[string]any{"$Type": "Workflows$MicroflowUserTargeting", "microflow": s.Microflow}
	case *workflows.XPathGroupSource:
		return map[string]any{"$Type": "Workflows$XPathGroupTargeting", "xPathConstraint": s.XPath}
	case *workflows.MicroflowGroupSource:
		return map[string]any{"$Type": "Workflows$MicroflowGroupTargeting", "microflow": s.Microflow}
	default:
		return map[string]any{"$Type": "Workflows$NoUserTargeting"}
	}
}

// mapUserTaskOutcomes maps a user task's outcomes; each carries a recursive
// sub-flow of activities run when that outcome is chosen.
func mapUserTaskOutcomes(outcomes []*workflows.UserTaskOutcome) ([]any, error) {
	out := make([]any, 0, len(outcomes))
	for _, oc := range outcomes {
		value := oc.Value
		if value == "" {
			value = oc.Caption
		}
		m := map[string]any{"$Type": "Workflows$UserTaskOutcome", "value": value}
		if fm, err := mapWorkflowFlowValue(oc.Flow); err != nil {
			return nil, err
		} else if fm != nil {
			m["flow"] = fm
		}
		out = append(out, m)
	}
	return out, nil
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
		if fm, err := mapWorkflowFlowValue(flow); err != nil {
			return nil, err
		} else if fm != nil {
			m["flow"] = fm
		}
		out = append(out, m)
	}
	return out, nil
}

// mapBoundaryEvents maps an activity's boundary events (timers, each with an
// optional handler sub-flow) onto their PED elements. Returns nil for none so
// callers can omit the key.
func mapBoundaryEvents(events []*workflows.BoundaryEvent) ([]any, error) {
	if len(events) == 0 {
		return nil, nil
	}
	out := make([]any, 0, len(events))
	for _, be := range events {
		el, err := boundaryEventElement(be)
		if err != nil {
			return nil, err
		}
		if be.Flow != nil {
			fm, err := mapWorkflowFlowValue(be.Flow)
			if err != nil {
				return nil, err
			}
			if fm != nil {
				el["flow"] = fm
			}
		}
		out = append(out, el)
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

// ALTER WORKFLOW (in-place edits). Unlike ALTER PAGE (pg_read_page returns the
// full widget tree), ped_read_document collapses nested workflow elements to
// their $Type, so the page-style read-modify-whole-tree approach does not work
// for workflows. Instead the mutator applies ped_update_document path ops. This
// first increment covers the workflow-level SET properties (which need no
// activity-index resolution); the activity/outcome/path/branch/boundary-event
// ops are not yet mapped and return a clear error.

// OpenWorkflowForMutation loads a workflow and returns a mutator that applies its
// edits via ped_update_document on Save.
func (b *Backend) OpenWorkflowForMutation(unitID model.ID) (backend.WorkflowMutator, error) {
	wf, err := b.GetWorkflow(unitID)
	if err != nil {
		return nil, fmt.Errorf("resolve workflow %s for mutation: %w", unitID, err)
	}
	modName, err := b.moduleNameForContainer(wf.ContainerID)
	if err != nil {
		return nil, fmt.Errorf("resolve module for workflow %q: %w", wf.Name, err)
	}
	return &mcpWorkflowMutator{backend: b, moduleName: modName, workflowName: wf.Name}, nil
}

type mcpWorkflowMutator struct {
	backend      *Backend
	moduleName   string
	workflowName string
	ops          []pedOpEntry
	applied      bool // an activity-level op was sent to PED (see Save)
}

var _ backend.WorkflowMutator = (*mcpWorkflowMutator)(nil)

func (m *mcpWorkflowMutator) set(path string, value any) {
	m.ops = append(m.ops, pedOpEntry{Path: path, Operation: pedOperation{Type: "set", Value: value}})
}

// SetProperty sets a workflow-level scalar/template property.
func (m *mcpWorkflowMutator) SetProperty(prop, value string) error {
	switch prop {
	case "display":
		// workflowName is a StringTemplate that already exists; set its text field
		// (replacing the whole element is rejected). title mirrors the display name.
		m.set("/workflowName/text", value)
		m.set("/title", value)
	case "description":
		m.set("/workflowDescription/text", value)
	case "due_date":
		m.set("/dueDate", value)
	default:
		return fmt.Errorf("ALTER WORKFLOW SET %s is not yet supported by the MCP backend", prop)
	}
	return nil
}

// SetPropertyWithEntity sets a workflow-level property that references an entity.
func (m *mcpWorkflowMutator) SetPropertyWithEntity(prop, value, entity string) error {
	switch prop {
	case "parameter":
		// The parameter element already exists; set its entity by-name reference
		// (replacing the whole element is rejected, like workflowName).
		m.set("/parameter/entity", entity)
		return nil
	default:
		return fmt.Errorf("ALTER WORKFLOW SET %s is not yet supported by the MCP backend", prop)
	}
}

// Save flushes the accumulated property sets to Studio Pro and validates.
//
// Activity-level ops have already been sent by the time Save runs (they apply
// eagerly through apply), but they must still be validated: ped_update_document
// only reports op-level failures, so a model-consistency error like CE0495
// "Duplicate name" comes back as SUCCESS and is visible nowhere except
// ped_check_errors. Save is the single place that check belongs — running it
// per op would pay the error-list settle each time (see pedCheckDocument) and
// would fail on the intermediate states a multi-op ALTER legitimately passes
// through.
func (m *mcpWorkflowMutator) Save() error {
	if len(m.ops) == 0 && !m.applied {
		return nil
	}
	qn := m.qn()
	if len(m.ops) > 0 {
		if err := m.backend.pedUpdateDoc(workflowDocType, qn, m.ops...); err != nil {
			return err
		}
	}
	m.backend.markDirty(m.moduleName)
	return m.backend.pedCheckDocument(workflowDocType, qn)
}

// qn returns the workflow's qualified document name.
func (m *mcpWorkflowMutator) qn() string { return m.moduleName + "." + m.workflowName }

// activityRefMatch is a resolved activity location: the activities array that
// contains it (a PED path) and its index within that array.
type activityRefMatch struct {
	arrayPath string
	index     int
	name      string // the activity's own name (activityRef may be its caption)
}

// wfLocation is a resolved activity location together with the set of every
// activity name in the workflow. Both come out of a single walk of the flow
// tree, so deduplicating an inserted activity's name costs no extra PED reads.
type wfLocation struct {
	arrayPath string // PED path of the activities array holding the match
	index     int    // the match's index within that array
	actPath   string // full PED path to the matched activity element
	name      string // the matched activity's own name (ref may be its caption)
	taken     map[string]bool
}

// resolve finds an activity reference (caption or name, optional 1-based @position)
// anywhere in the flow tree and returns its location plus the workflow's
// activity-name set.
func (m *mcpWorkflowMutator) resolve(ref string, atPos int) (wfLocation, error) {
	var matches []activityRefMatch
	taken := map[string]bool{}
	if err := m.searchActivities("/flow/activities", ref, &matches, taken); err != nil {
		return wfLocation{}, err
	}
	var pick activityRefMatch
	switch {
	case len(matches) == 0:
		return wfLocation{}, fmt.Errorf("activity %q not found in workflow %q", ref, m.qn())
	case atPos > 0:
		if atPos > len(matches) {
			return wfLocation{}, fmt.Errorf("activity %q @%d not found (only %d matches)", ref, atPos, len(matches))
		}
		pick = matches[atPos-1]
	case len(matches) > 1:
		return wfLocation{}, fmt.Errorf("ambiguous activity %q (%d matches); use @N to disambiguate", ref, len(matches))
	default:
		pick = matches[0]
	}
	return wfLocation{
		arrayPath: pick.arrayPath,
		index:     pick.index,
		actPath:   fmt.Sprintf("%s/%d", pick.arrayPath, pick.index),
		name:      pick.name,
		taken:     taken,
	}, nil
}

// searchActivities walks an activities array and every descendant sub-flow
// (each activity's outcome flows, then its boundary-event flows, in order),
// appending every activity whose name or caption equals ref and recording every
// activity name it passes in taken. The depth-first, in-order traversal matches
// DESCRIBE, so @N numbering lines up.
//
// Name collection rides along on the search rather than being its own pass
// because each level costs a PED round-trip; the two consumers always want both.
func (m *mcpWorkflowMutator) searchActivities(arrayPath, ref string, matches *[]activityRefMatch, taken map[string]bool) error {
	acts, err := m.readArrayRaw(arrayPath)
	if err != nil {
		return err
	}
	for i, a := range acts {
		if name := mapString(a, "name"); name != "" {
			taken[name] = true
		}
		if mapString(a, "name") == ref || mapString(a, "caption") == ref {
			*matches = append(*matches, activityRefMatch{arrayPath: arrayPath, index: i, name: mapString(a, "name")})
		}
		actPath := fmt.Sprintf("%s/%d", arrayPath, i)
		for _, sub := range m.subFlowArrays(actPath, mapString(a, "$Type")) {
			if err := m.searchActivities(sub, ref, matches, taken); err != nil {
				return err
			}
		}
	}
	return nil
}

// subFlowArrays returns the activities-array paths of an activity's sub-flows
// (outcome flows then boundary-event flows) that actually carry a flow.
func (m *mcpWorkflowMutator) subFlowArrays(actPath, sType string) []string {
	var out []string
	for _, field := range []string{"outcomes", "boundaryEvents"} {
		if !nestableField(sType, field) {
			continue
		}
		elems, err := m.readArrayRaw(actPath + "/" + field)
		if err != nil {
			continue // best-effort: an absent/unreadable sub-array just isn't traversed
		}
		for j, e := range elems {
			if _, hasFlow := e["flow"]; hasFlow {
				out = append(out, fmt.Sprintf("%s/%s/%d/flow/activities", actPath, field, j))
			}
		}
	}
	return out
}

// nestableField reports whether an activity of the given PED type can hold
// sub-flows under the given field (outcome flows / boundary-event flows). It
// bounds the recursive search to the arrays worth reading (a multi user task's
// outcomes are plain strings, so they are not outcome-nestable).
func nestableField(sType, field string) bool {
	switch field {
	case "outcomes":
		switch sType {
		case "Workflows$SingleUserTaskActivity", "Workflows$ExclusiveSplitActivity",
			"Workflows$ParallelSplitActivity", "Workflows$CallMicroflowActivity":
			return true
		}
	case "boundaryEvents":
		switch sType {
		case "Workflows$SingleUserTaskActivity", "Workflows$MultiUserTaskActivity",
			"Workflows$CallMicroflowActivity", "Workflows$CallWorkflowActivity",
			"Workflows$WaitForNotificationActivity":
			return true
		}
	}
	return false
}

// readArrayRaw reads a PED array path and returns its elements as generic maps.
func (m *mcpWorkflowMutator) readArrayRaw(path string) ([]map[string]any, error) {
	res, err := m.backend.client.CallTool("ped_read_document", map[string]any{
		"documentType": workflowDocType,
		"documentName": m.qn(),
		"paths":        []string{path},
	})
	if err != nil {
		return nil, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return nil, fmt.Errorf("read %s %s: %s", m.qn(), path, text)
	}
	var doc struct {
		Results []struct {
			Result []map[string]any `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil || len(doc.Results) == 0 {
		return nil, fmt.Errorf("parse %s %s: %v", m.qn(), path, err)
	}
	return doc.Results[0].Result, nil
}

func mapString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

// apply sends activity-level ops to PED immediately (only the workflow-level
// SETs defer through m.set()/Save()). It records that a write happened so Save
// still validates the document even when no property set was queued.
func (m *mcpWorkflowMutator) apply(ops ...pedOpEntry) error {
	m.applied = true
	return m.backend.pedUpdateDoc(workflowDocType, m.qn(), ops...)
}

// SetActivityProperty sets a primitive/reference leaf on a top-level activity.
// PED forbids replacing a nested element wholesale, so a property whose change
// would swap an element's *kind* (e.g. switching user targeting from XPath to
// Microflow) is rejected rather than silently dropped.
func (m *mcpWorkflowMutator) SetActivityProperty(activityRef string, atPos int, prop, value string) error {
	// Reject an unsupported property before the (live) index resolution.
	leaf := map[string]string{
		"page":        "/taskPage/page",
		"description": "/taskDescription/text",
		"due_date":    "/dueDate",
	}[strings.ToLower(prop)]
	switch strings.ToLower(prop) {
	case "page", "description", "due_date", "targeting_microflow", "targeting_xpath":
	default:
		return fmt.Errorf("ALTER WORKFLOW set activity %s is not supported by the MCP backend (supported: page, description, due_date, targeting_microflow, targeting_xpath)", prop)
	}
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	switch strings.ToLower(prop) {
	case "targeting_microflow":
		return m.setUserTargetingLeaf(actPath, "Workflows$MicroflowUserTargeting", "microflow", value)
	case "targeting_xpath":
		return m.setUserTargetingLeaf(actPath, "Workflows$XPathUserTargeting", "xPathConstraint", value)
	default:
		return m.apply(pedOpEntry{Path: actPath + leaf, Operation: pedOperation{Type: "set", Value: value}})
	}
}

// setUserTargetingLeaf sets the microflow/xpath leaf on an activity's existing
// userTargeting element. Because PED can't replace the element, this only works
// when the activity is already targeted the requested way.
func (m *mcpWorkflowMutator) setUserTargetingLeaf(actPath, wantType, field, value string) error {
	cur, err := m.readActivityElementType(actPath, "userTargeting")
	if err != nil {
		return err
	}
	if cur != "" && cur != wantType {
		return fmt.Errorf("activity user targeting is %s; changing the targeting kind via MCP is not supported (PED cannot replace the element) — set it in Studio Pro, or recreate the user task", cur)
	}
	return m.apply(pedOpEntry{
		Path:      fmt.Sprintf("%s/userTargeting/%s", actPath, field),
		Operation: pedOperation{Type: "set", Value: value},
	})
}

func (m *mcpWorkflowMutator) InsertAfterActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	arrayPath, idx := loc.arrayPath, loc.index
	wfnames.Dedup(activities, loc.taken)
	ops := make([]pedOpEntry, 0, len(activities))
	for i, a := range activities {
		mapped, err := mapWorkflowActivity(a)
		if err != nil {
			return err
		}
		at := idx + 1 + i
		ops = append(ops, pedOpEntry{Path: arrayPath, Operation: pedOperation{Type: "add", Value: mapped, Index: &at}})
	}
	return m.apply(ops...)
}

func (m *mcpWorkflowMutator) DropActivity(activityRef string, atPos int) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	return m.apply(removeAtOp(loc.arrayPath, loc.index))
}

func (m *mcpWorkflowMutator) ReplaceActivity(activityRef string, atPos int, activities []workflows.WorkflowActivity) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	arrayPath, idx := loc.arrayPath, loc.index
	// Free the outgoing activity's name before deduplicating: the remove and the
	// adds go to PED in one call, so the replaced activity is gone by the time
	// any name is resolved, and a replacement reusing its name is the ordinary
	// in-place edit rather than a collision (issue #944).
	delete(loc.taken, loc.name)
	wfnames.Dedup(activities, loc.taken)
	mapped := make([]map[string]any, 0, len(activities))
	for _, a := range activities {
		mm, err := mapWorkflowActivity(a)
		if err != nil {
			return err
		}
		mapped = append(mapped, mm)
	}
	// Replace = remove the slot then add the new activities at it. (A set on the
	// array index — a whole-element replace — is rejected by PED, like a
	// whole-element set of a nested constructor.)
	ops := []pedOpEntry{removeAtOp(arrayPath, idx)}
	for i, mm := range mapped {
		at := idx + i
		ops = append(ops, pedOpEntry{Path: arrayPath, Operation: pedOperation{Type: "add", Value: mm, Index: &at}})
	}
	return m.apply(ops...)
}

// --- outcome / path / branch ops (all live in an activity's `outcomes` array) ---

// InsertOutcome adds a named outcome (with an optional sub-flow) to a user task.
func (m *mcpWorkflowMutator) InsertOutcome(activityRef string, atPos int, outcomeName string, activities []workflows.WorkflowActivity) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	wfnames.Dedup(activities, loc.taken)
	el := map[string]any{"$Type": "Workflows$UserTaskOutcome", "value": outcomeName}
	if err := attachSubFlow(el, activities); err != nil {
		return err
	}
	return m.addToActivityArray(actPath, "outcomes", el)
}

// DropOutcome removes a user-task outcome by its value ("Default" matches a void outcome).
func (m *mcpWorkflowMutator) DropOutcome(activityRef string, atPos int, outcomeName string) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	return m.dropFromActivityArray(actPath, "outcomes", activityRef, "outcome", func(o pedOutcomeElem) bool {
		return o.valueString() == outcomeName ||
			(strings.EqualFold(outcomeName, "Default") && o.SType == "Workflows$VoidConditionOutcome")
	})
}

// InsertPath adds a concurrent path (with an optional sub-flow) to a parallel split.
func (m *mcpWorkflowMutator) InsertPath(activityRef string, atPos int, pathCaption string, activities []workflows.WorkflowActivity) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	// End the path with Mendix's marker before dedup names it — without it the
	// engine skips the path's contents at runtime (workflows.EndParallelSplitPath).
	// PED assigns element ids itself, so the factory's value is never sent.
	activities = workflows.EndParallelSplitPath(&workflows.Flow{Activities: activities}, func() model.ID {
		return ""
	}).Activities
	wfnames.Dedup(activities, loc.taken)
	el := map[string]any{"$Type": "Workflows$ParallelSplitOutcome"}
	if err := attachSubFlow(el, activities); err != nil {
		return err
	}
	return m.addToActivityArray(actPath, "outcomes", el)
}

// DropPath removes a parallel-split path. Paths have no stored name; the caption
// "Path N" addresses the N-th path, and an empty caption drops the last one.
func (m *mcpWorkflowMutator) DropPath(activityRef string, atPos int, pathCaption string) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	paths, err := m.readActivityArray(actPath, "outcomes")
	if err != nil {
		return err
	}
	target := len(paths) - 1 // empty caption -> last
	if pathCaption != "" {
		target = -1
		for i := range paths {
			if fmt.Sprintf("Path %d", i+1) == pathCaption {
				target = i
				break
			}
		}
	}
	if target < 0 || target >= len(paths) {
		return fmt.Errorf("path %q not found on parallel split %q", pathCaption, activityRef)
	}
	return m.apply(removeAtOp(actPath+"/outcomes", target))
}

// InsertBranch adds a condition branch (true/false/default/enum-value) to a decision.
func (m *mcpWorkflowMutator) InsertBranch(activityRef string, atPos int, condition string, activities []workflows.WorkflowActivity) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	wfnames.Dedup(activities, loc.taken)
	el := branchOutcomeElement(condition)
	if err := attachSubFlow(el, activities); err != nil {
		return err
	}
	return m.addToActivityArray(actPath, "outcomes", el)
}

// DropBranch removes a decision branch by name (true/false/default, or an enum value).
func (m *mcpWorkflowMutator) DropBranch(activityRef string, atPos int, branchName string) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	return m.dropFromActivityArray(actPath, "outcomes", activityRef, "branch", func(o pedOutcomeElem) bool {
		switch strings.ToLower(branchName) {
		case "true":
			return o.SType == "Workflows$BooleanConditionOutcome" && o.valueBool()
		case "false":
			return o.SType == "Workflows$BooleanConditionOutcome" && !o.valueBool()
		case "default":
			return o.SType == "Workflows$VoidConditionOutcome"
		default:
			return o.valueString() == branchName
		}
	})
}

// --- boundary event ops (live in an activity's `boundaryEvents` array) ---

// InsertBoundaryEvent attaches a (non-)interrupting timer boundary event, with an
// optional handler sub-flow, to a user task or call-microflow activity.
func (m *mcpWorkflowMutator) InsertBoundaryEvent(activityRef string, atPos int, eventType, delay string, activities []workflows.WorkflowActivity) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	// End the path with Mendix's marker before dedup names it
	// (workflows.EndBoundaryEventPath). PED assigns ids, so the factory's value
	// is never sent.
	activities = workflows.EndBoundaryEventPath(&workflows.Flow{Activities: activities}, func() model.ID {
		return ""
	}).Activities
	wfnames.Dedup(activities, loc.taken)
	be := &workflows.BoundaryEvent{EventType: eventType, TimerDelay: delay}
	if be.IsNotification() {
		// The op carries no name, and a notification event is reached by its name.
		return fmt.Errorf("insert boundary event: %q boundary events cannot be inserted by ALTER WORKFLOW", eventType)
	}
	el, err := boundaryEventElement(be)
	if err != nil {
		return err
	}
	if err := attachSubFlow(el, activities); err != nil {
		return err
	}
	return m.addToActivityArray(actPath, "boundaryEvents", el)
}

// DropBoundaryEvent removes the activity's (first) boundary event.
func (m *mcpWorkflowMutator) DropBoundaryEvent(activityRef string, atPos int) error {
	loc, err := m.resolve(activityRef, atPos)
	if err != nil {
		return err
	}
	actPath := loc.actPath
	events, err := m.readActivityArray(actPath, "boundaryEvents")
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("activity %q has no boundary events", activityRef)
	}
	return m.apply(removeAtOp(actPath+"/boundaryEvents", 0))
}

// --- shared helpers for nested-array ops ---

// pedOutcomeElem is the shallow shape of an outcome/branch/path element as PED
// returns it from a nested-array read (deeper structure collapses to $Type).
type pedOutcomeElem struct {
	SType string          `json:"$Type"`
	Value json.RawMessage `json:"value"`
}

func (o pedOutcomeElem) valueString() string {
	var s string
	_ = json.Unmarshal(o.Value, &s)
	return s
}

func (o pedOutcomeElem) valueBool() bool {
	var b bool
	_ = json.Unmarshal(o.Value, &b)
	return b
}

func removeAtOp(path string, idx int) pedOpEntry {
	return pedOpEntry{Path: path, Operation: pedOperation{Type: "remove", Index: &idx}}
}

// attachSubFlow maps the activities into a Workflows$Flow and stores it under
// the element's `flow` key (omitted when there are no activities).
func attachSubFlow(el map[string]any, activities []workflows.WorkflowActivity) error {
	if len(activities) == 0 {
		return nil
	}
	flow, err := mapWorkflowFlowValue(&workflows.Flow{Activities: activities})
	if err != nil {
		return err
	}
	el["flow"] = flow
	return nil
}

// addToActivityArray appends an element to an activity's nested array (outcomes,
// boundaryEvents) at actPath. A PED `add` without an index appends.
func (m *mcpWorkflowMutator) addToActivityArray(actPath, field string, el map[string]any) error {
	return m.apply(pedOpEntry{
		Path:      actPath + "/" + field,
		Operation: pedOperation{Type: "add", Value: el},
	})
}

// dropFromActivityArray reads an activity's nested array, finds the first element
// matching the predicate, and removes it by index.
func (m *mcpWorkflowMutator) dropFromActivityArray(actPath, field, activityRef, kind string, match func(pedOutcomeElem) bool) error {
	elems, err := m.readActivityArray(actPath, field)
	if err != nil {
		return err
	}
	for i, e := range elems {
		if match(e) {
			return m.apply(removeAtOp(actPath+"/"+field, i))
		}
	}
	return fmt.Errorf("%s not found on activity %q", kind, activityRef)
}

// readActivityArray reads an activity's nested array (outcomes, boundaryEvents)
// for index resolution on DROP.
func (m *mcpWorkflowMutator) readActivityArray(actPath, field string) ([]pedOutcomeElem, error) {
	res, err := m.backend.client.CallTool("ped_read_document", map[string]any{
		"documentType": workflowDocType,
		"documentName": m.qn(),
		"paths":        []string{actPath + "/" + field},
	})
	if err != nil {
		return nil, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return nil, fmt.Errorf("read %s %s/%s: %s", m.qn(), actPath, field, text)
	}
	var doc struct {
		Results []struct {
			Result []pedOutcomeElem `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil || len(doc.Results) == 0 {
		return nil, fmt.Errorf("parse %s %s/%s: %v", m.qn(), actPath, field, err)
	}
	return doc.Results[0].Result, nil
}

// readActivityElementType reads the $Type of a single nested element on an activity.
func (m *mcpWorkflowMutator) readActivityElementType(actPath, field string) (string, error) {
	res, err := m.backend.client.CallTool("ped_read_document", map[string]any{
		"documentType": workflowDocType,
		"documentName": m.qn(),
		"paths":        []string{actPath + "/" + field},
	})
	if err != nil {
		return "", err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return "", fmt.Errorf("read %s %s/%s: %s", m.qn(), actPath, field, text)
	}
	var doc struct {
		Results []struct {
			Result struct {
				SType string `json:"$Type"`
			} `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil || len(doc.Results) == 0 {
		return "", nil // unknown -> caller proceeds (PED will reject a real mismatch)
	}
	return doc.Results[0].Result.SType, nil
}

// branchOutcomeElement builds the condition outcome for an exclusive-split branch.
func branchOutcomeElement(condition string) map[string]any {
	switch strings.ToLower(condition) {
	case "true":
		return map[string]any{"$Type": "Workflows$BooleanConditionOutcome", "value": true}
	case "false":
		return map[string]any{"$Type": "Workflows$BooleanConditionOutcome", "value": false}
	case "default":
		return map[string]any{"$Type": "Workflows$VoidConditionOutcome"}
	default:
		return map[string]any{"$Type": "Workflows$EnumerationValueConditionOutcome", "value": condition}
	}
}

// boundaryEventElement builds a boundary event (the MPR backend's type mapping,
// workflows.BoundaryEventStorageType; PED auto-assigns $ID/PersistentId). A
// notification event's constructor takes a name and caption (ped_get_schema,
// Studio Pro 11.14); the interrupting one's isInsideOfParallelSplit is set by
// markNotificationEventsInSplits once the whole flow is mapped.
func boundaryEventElement(be *workflows.BoundaryEvent) (map[string]any, error) {
	typeName, ok := workflows.BoundaryEventStorageType(be.EventType)
	if !ok {
		return nil, fmt.Errorf("boundary event kind %q is not supported by the MCP backend", be.EventType)
	}
	if be.IsNotification() {
		el := map[string]any{"$Type": typeName, "name": be.Name, "caption": be.Caption}
		if !be.IsNonInterrupting() {
			el["isInsideOfParallelSplit"] = false
		}
		return el, nil
	}
	el := map[string]any{"$Type": typeName, "caption": ""}
	if be.TimerDelay != "" {
		el["firstExecutionTime"] = be.TimerDelay
	}
	if typeName == "Workflows$NonInterruptingTimerBoundaryEvent" {
		el["recurrence"] = nil
	}
	return el, nil
}
