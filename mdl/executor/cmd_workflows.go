// SPDX-License-Identifier: Apache-2.0

// Package executor - Workflow SHOW/DESCRIBE commands
package executor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// listWorkflows handles SHOW WORKFLOWS command.
func listWorkflows(ctx *ExecContext, moduleName string) error {
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	wfs, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return mdlerrors.NewBackend("list workflows", err)
	}

	type row struct {
		qualifiedName string
		module        string
		name          string
		activities    int
		userTasks     int
		decisions     int
		paramEntity   string
	}
	var rows []row

	for _, wf := range wfs {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if moduleName != "" && modName != moduleName {
			continue
		}

		qualifiedName := modName + "." + wf.Name
		paramEntity := ""
		if wf.Parameter != nil {
			paramEntity = wf.Parameter.EntityRef
		}

		acts, uts, decs := countWorkflowActivities(wf)

		rows = append(rows, row{qualifiedName, modName, wf.Name, acts, uts, decs, paramEntity})
	}

	// Sort by qualified name
	sort.Slice(rows, func(i, j int) bool {
		return strings.ToLower(rows[i].qualifiedName) < strings.ToLower(rows[j].qualifiedName)
	})

	result := &TableResult{
		Columns: []string{"Qualified Name", "Activities", "User Tasks", "Decisions", "Parameter Entity"},
		Summary: fmt.Sprintf("(%d workflows)", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualifiedName, r.activities, r.userTasks, r.decisions, r.paramEntity})
	}
	return writeResult(ctx, result)
}

// countWorkflowActivities counts total activities, user tasks, and decisions in a workflow.
func countWorkflowActivities(wf *workflows.Workflow) (total, userTasks, decisions int) {
	if wf.Flow == nil {
		return
	}
	countFlowActivities(wf.Flow, &total, &userTasks, &decisions)
	return
}

// countFlowActivities recursively counts activities in a flow and its sub-flows.
func countFlowActivities(flow *workflows.Flow, total, userTasks, decisions *int) {
	if flow == nil {
		return
	}
	for _, act := range flow.Activities {
		*total++
		switch a := act.(type) {
		case *workflows.UserTask:
			*userTasks++
			for _, outcome := range a.Outcomes {
				countFlowActivities(outcome.Flow, total, userTasks, decisions)
			}
		case *workflows.ExclusiveSplitActivity:
			*decisions++
			for _, outcome := range a.Outcomes {
				if co, ok := outcome.(*workflows.BooleanConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				} else if co, ok := outcome.(*workflows.EnumerationValueConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				} else if co, ok := outcome.(*workflows.VoidConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				}
			}
		case *workflows.ParallelSplitActivity:
			for _, outcome := range a.Outcomes {
				countFlowActivities(outcome.Flow, total, userTasks, decisions)
			}
		case *workflows.CallMicroflowTask:
			for _, outcome := range a.Outcomes {
				if co, ok := outcome.(*workflows.BooleanConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				} else if co, ok := outcome.(*workflows.EnumerationValueConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				} else if co, ok := outcome.(*workflows.VoidConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				}
			}
		case *workflows.SystemTask:
			for _, outcome := range a.Outcomes {
				if co, ok := outcome.(*workflows.BooleanConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				} else if co, ok := outcome.(*workflows.EnumerationValueConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				} else if co, ok := outcome.(*workflows.VoidConditionOutcome); ok {
					countFlowActivities(co.Flow, total, userTasks, decisions)
				}
			}
		}
	}
}

// describeWorkflow handles DESCRIBE WORKFLOW command.
func describeWorkflow(ctx *ExecContext, name ast.QualifiedName) error {
	output, _, err := describeWorkflowToString(ctx, name)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Output, output)
	return nil
}

// describeWorkflowToString generates MDL-like output for a workflow and returns it as a string.
func describeWorkflowToString(ctx *ExecContext, name ast.QualifiedName) (string, map[string]elkSourceRange, error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return "", nil, mdlerrors.NewBackend("build hierarchy", err)
	}

	allWorkflows, err := ctx.Backend.ListWorkflows()
	if err != nil {
		return "", nil, mdlerrors.NewBackend("list workflows", err)
	}

	var targetWf *workflows.Workflow
	for _, wf := range allWorkflows {
		modID := h.FindModuleID(wf.ContainerID)
		modName := h.GetModuleName(modID)
		if modName == name.Module && wf.Name == name.Name {
			targetWf = wf
			break
		}
	}

	if targetWf == nil {
		return "", nil, mdlerrors.NewNotFound("workflow", name.String())
	}

	var lines []string
	qualifiedName := name.Module + "." + name.Name

	// Documentation
	if targetWf.Documentation != "" {
		lines = append(lines, "/**")
		for docLine := range strings.SplitSeq(targetWf.Documentation, "\n") {
			lines = append(lines, " * "+docLine)
		}
		lines = append(lines, " */")
	}

	// Header
	lines = append(lines, fmt.Sprintf("-- Workflow: %s", qualifiedName))
	if targetWf.Annotation != "" {
		lines = append(lines, fmt.Sprintf("-- %s", targetWf.Annotation))
	}
	lines = append(lines, "")

	lines = append(lines, fmt.Sprintf("create workflow %s", qualifiedName))
	if clause := describeFolderClause(ctx, targetWf.ContainerID); clause != "" {
		lines = append(lines, "  "+strings.TrimSpace(clause))
	}

	// Context parameter
	if targetWf.Parameter != nil && targetWf.Parameter.EntityRef != "" {
		lines = append(lines, fmt.Sprintf("  parameter $WorkflowContext: %s", targetWf.Parameter.EntityRef))
	}

	// Display name
	if targetWf.WorkflowName != "" {
		lines = append(lines, fmt.Sprintf("  display %s", mdlQuoted(targetWf.WorkflowName)))
	}

	// Description
	if targetWf.WorkflowDescription != "" {
		lines = append(lines, fmt.Sprintf("  description %s", mdlQuoted(targetWf.WorkflowDescription)))
	}

	// Export level (only emit when non-empty)
	if targetWf.ExportLevel != "" {
		lines = append(lines, fmt.Sprintf("  export level %s", targetWf.ExportLevel))
	}

	// Overview page
	if targetWf.OverviewPage != "" {
		lines = append(lines, fmt.Sprintf("  overview page %s", targetWf.OverviewPage))
	}

	// Due date
	if targetWf.DueDate != "" {
		lines = append(lines, fmt.Sprintf("  due date %s", mdlQuoted(targetWf.DueDate)))
	}

	lines = append(lines, "")

	lines = append(lines, "begin")
	// Activities
	if targetWf.Flow != nil {
		actLines := formatWorkflowActivities(targetWf.Flow, "  ")
		lines = append(lines, actLines...)
	}

	lines = append(lines, "end workflow")
	lines = append(lines, "/")

	return strings.Join(lines, "\n"), nil, nil
}

// formatAnnotation renders an activity's annotation as MDL comment lines.
//
// It used to emit `annotation '<text>';`, and its own doc comment claimed that
// statement "survives round-trips". It does not, and has not since MDL-WF04: a
// standalone `annotation` in a workflow body is refused at check time AND by
// execCreateWorkflow, because Mendix constructs every child of the activity flow
// with a Flow parent and no annotation type takes one — the written unit cannot
// be LOADED, so Studio Pro will not open the project. The describer was emitting
// the one construct the writer refuses, and a 23-activity workflow produced 13
// MDL-WF04 errors from unmodified DESCRIBE output (mendixlabs/mxcli#1007).
//
// A comment is the honest emit today, not a workaround. The annotation being
// re-emitted here is ATTACHED to an activity, and although the write path stores
// an attached annotation (addActivityBaseFields), no MDL input can produce one:
// MDLWorkflow.g4 has only the standalone `workflowAnnotationStmt`. So the text is
// unwritable either way, and carrying it as a comment at least keeps it in front
// of whoever edits the script. The `annotation:` marker says what the line was.
//
// The microflow domain does have an attached form (`@annotation 'text'`, see
// MDLMicroflow.g4) and it round-trips properly. Giving workflow activities the
// same prefix is the fix that would preserve the annotation rather than
// commenting it out; it is a grammar change, and deliberately not bundled here.
func formatAnnotation(annotation string, indent string) string {
	if annotation == "" {
		return ""
	}
	return annotationComment(annotation, indent)
}

// annotationComment renders text as one or more `-- annotation:` lines. An
// annotation may contain newlines, and a `--` comment runs to end of line, so a
// multi-line note has to be prefixed line by line or everything after the first
// newline becomes stray tokens — the same failure the statement form had.
func annotationComment(text, indent string) string {
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		l = strings.TrimRight(l, "\r")
		if i == 0 {
			lines[i] = indent + "-- annotation: " + l
			continue
		}
		lines[i] = indent + "--   " + l
	}
	return strings.Join(lines, "\n")
}

// boundaryEventKeyword maps an EventType string to the MDL BOUNDARY EVENT keyword sequence.
func boundaryEventKeyword(eventType string) string {
	switch eventType {
	case "InterruptingTimer":
		return "boundary event interrupting timer"
	case "NonInterruptingTimer":
		return "boundary event non interrupting timer"
	default:
		return "boundary event timer"
	}
}

// formatBoundaryEvents formats boundary events for describe output.
func formatBoundaryEvents(events []*workflows.BoundaryEvent, indent string) []string {
	if len(events) == 0 {
		return nil
	}

	var lines []string
	for _, event := range events {
		keyword := boundaryEventKeyword(event.EventType)
		if event.TimerDelay != "" {
			lines = append(lines, fmt.Sprintf("%s%s %s", indent, keyword, mdlQuoted(event.TimerDelay)))
		} else {
			lines = append(lines, fmt.Sprintf("%s%s", indent, keyword))
		}
		if event.Flow != nil && len(event.Flow.Activities) > 0 {
			lines = append(lines, fmt.Sprintf("%s{", indent))
			subLines := formatWorkflowActivities(event.Flow, indent+"  ")
			lines = append(lines, subLines...)
			lines = append(lines, fmt.Sprintf("%s}", indent))
		}
	}

	return lines
}

// formatWorkflowActivities generates MDL-like output for workflow activities.
func formatWorkflowActivities(flow *workflows.Flow, indent string) []string {
	if flow == nil {
		return nil
	}

	var lines []string
	for _, act := range flow.Activities {
		var actLines []string
		isComment := false
		switch a := act.(type) {
		case *workflows.UserTask:
			actLines = formatUserTask(a, indent)
		case *workflows.CallMicroflowTask:
			actLines = formatCallMicroflowTask(a, indent)
		case *workflows.SystemTask:
			actLines = formatSystemTask(a, indent)
		case *workflows.CallWorkflowActivity:
			actLines = formatCallWorkflowActivity(a, indent)
		case *workflows.ExclusiveSplitActivity:
			actLines = formatExclusiveSplit(a, indent)
		case *workflows.ParallelSplitActivity:
			actLines = formatParallelSplit(a, indent)
		case *workflows.JumpToActivity:
			target := a.TargetActivity
			if target == "" {
				target = "?"
			}
			if a.Annotation != "" {
				actLines = append(actLines, formatAnnotation(a.Annotation, indent))
			}
			// Only emit `comment '...'` when it carries information the author
			// wrote. buildJumpTo defaults Caption to the target name, so echoing it
			// unconditionally rendered a plain `jump to Triage;` as
			// `jump to Triage comment 'Triage'` — a phantom comment nobody authored
			// (issuetracker #16). Re-applying the shorter form rebuilds the same
			// Caption, so dropping it is lossless.
			if caption := a.Caption; caption != "" && caption != target && caption != a.Name {
				actLines = append(actLines, fmt.Sprintf("%sjump to %s comment %s", indent, mdlIdent(target), mdlQuoted(caption)))
			} else {
				actLines = append(actLines, fmt.Sprintf("%sjump to %s", indent, mdlIdent(target)))
			}
		case *workflows.WaitForTimerActivity:
			caption := a.Caption
			if caption == "" {
				caption = a.Name
			}
			if a.Annotation != "" {
				actLines = append(actLines, formatAnnotation(a.Annotation, indent))
			}
			if a.DelayExpression != "" {
				actLines = append(actLines, fmt.Sprintf("%swait for timer %s comment %s", indent, mdlQuoted(a.DelayExpression), mdlQuoted(caption)))
			} else {
				actLines = append(actLines, fmt.Sprintf("%swait for timer comment %s", indent, mdlQuoted(caption)))
			}
		case *workflows.WaitForNotificationActivity:
			caption := a.Caption
			if caption == "" {
				caption = a.Name
			}
			if a.Annotation != "" {
				actLines = append(actLines, formatAnnotation(a.Annotation, indent))
			}
			actLines = append(actLines, fmt.Sprintf("%swait for notification -- %s", indent, caption))
			// BoundaryEvents
			actLines = append(actLines, formatBoundaryEvents(a.BoundaryEvents, indent+"  ")...)
		case *workflows.StartWorkflowActivity:
			// Skip start activities - they are implicit
			continue
		case *workflows.EndWorkflowActivity:
			// Skip end activities - they are implicit
			continue
		case *workflows.EndOfParallelSplitPathActivity:
			// Skip - auto-generated by Mendix, implicit in MDL syntax
			continue
		case *workflows.EndOfBoundaryEventPathActivity:
			// Skip - auto-generated by Mendix, implicit in MDL syntax
			continue
		case *workflows.WorkflowAnnotationActivity:
			// A standalone annotation (sticky note) read back from the model. Emitted
			// as a comment for the same reason as an attached one: the `annotation`
			// statement it used to produce is refused by MDL-WF04 and by exec, so the
			// describe output could not be re-run (mendixlabs/mxcli#1007).
			if a.Description == "" {
				continue
			}
			isComment = true
			actLines = []string{annotationComment(a.Description, indent)}
		case *workflows.GenericWorkflowActivity:
			isComment = true
			caption := a.Caption
			if caption == "" {
				caption = a.Name
			}
			actLines = []string{fmt.Sprintf("%s-- [%s] %s", indent, a.TypeString, caption)}
		default:
			isComment = true
			actLines = []string{fmt.Sprintf("%s-- [unknown activity]", indent)}
		}
		// Append semicolon to last line of activity (not for comments)
		// Insert before any -- comment to avoid the comment swallowing the semicolon
		if !isComment && len(actLines) > 0 {
			lastLine := actLines[len(actLines)-1]
			if idx := strings.Index(lastLine, " -- "); idx >= 0 {
				actLines[len(actLines)-1] = lastLine[:idx] + ";" + lastLine[idx:]
			} else {
				actLines[len(actLines)-1] = lastLine + ";"
			}
		}
		lines = append(lines, actLines...)
		lines = append(lines, "")
	}

	return lines
}

// formatUserTask formats a user task for describe output.
func formatUserTask(a *workflows.UserTask, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}
	nameStr := a.Name
	if nameStr == "" {
		nameStr = "unnamed"
	}

	taskKeyword := "user task"
	if a.IsMulti {
		taskKeyword = "multi user task"
	}
	lines = append(lines, fmt.Sprintf("%s%s %s %s", indent, taskKeyword, mdlIdent(nameStr), mdlQuoted(caption)))

	if a.Page != "" {
		lines = append(lines, fmt.Sprintf("%s  page %s", indent, a.Page))
	}

	// User targeting
	if a.UserSource != nil {
		switch us := a.UserSource.(type) {
		case *workflows.MicroflowBasedUserSource:
			if us.Microflow != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting users microflow %s", indent, us.Microflow))
			}
		case *workflows.XPathBasedUserSource:
			if us.XPath != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting users xpath %s", indent, mdlQuoted(us.XPath)))
			}
		case *workflows.MicroflowGroupSource:
			if us.Microflow != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting groups microflow %s", indent, us.Microflow))
			}
		case *workflows.XPathGroupSource:
			if us.XPath != "" {
				lines = append(lines, fmt.Sprintf("%s  targeting groups xpath %s", indent, mdlQuoted(us.XPath)))
			}
		}
	}

	if a.UserTaskEntity != "" {
		lines = append(lines, fmt.Sprintf("%s  entity %s", indent, a.UserTaskEntity))
	}

	// Due date (task-level)
	if a.DueDate != "" {
		lines = append(lines, fmt.Sprintf("%s  due date %s", indent, mdlQuoted(a.DueDate)))
	}

	// Task description
	if a.TaskDescription != "" {
		lines = append(lines, fmt.Sprintf("%s  description %s", indent, mdlQuoted(a.TaskDescription)))
	}

	// Outcomes
	if len(a.Outcomes) > 0 {
		lines = append(lines, fmt.Sprintf("%s  outcomes", indent))
		for _, outcome := range a.Outcomes {
			outValue := outcome.Value
			if outValue == "" {
				outValue = outcome.Caption
			}
			if outValue == "" {
				outValue = outcome.Name
			}
			if outcome.Flow != nil && len(outcome.Flow.Activities) > 0 {
				lines = append(lines, fmt.Sprintf("%s    %s {", indent, mdlQuoted(outValue)))
				subLines := formatWorkflowActivities(outcome.Flow, indent+"      ")
				lines = append(lines, subLines...)
				lines = append(lines, fmt.Sprintf("%s    }", indent))
			} else {
				lines = append(lines, fmt.Sprintf("%s    %s { }", indent, mdlQuoted(outValue)))
			}
		}
	}

	// BoundaryEvents
	lines = append(lines, formatBoundaryEvents(a.BoundaryEvents, indent+"  ")...)

	return lines
}

// formatCallMicroflowTask formats a call microflow task for describe output.
func formatCallMicroflowTask(a *workflows.CallMicroflowTask, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	mf := a.Microflow
	if mf == "" {
		mf = "?"
	}

	if len(a.ParameterMappings) > 0 {
		var params []string
		for _, pm := range a.ParameterMappings {
			paramName := pm.Parameter
			if idx := strings.LastIndex(paramName, "."); idx >= 0 {
				paramName = paramName[idx+1:]
			}
			params = append(params, fmt.Sprintf("%s = %s", paramName, mdlQuoted(pm.Expression)))
		}
		lines = append(lines, fmt.Sprintf("%scall microflow %s with (%s) -- %s", indent, mf, strings.Join(params, ", "), caption))
	} else {
		lines = append(lines, fmt.Sprintf("%scall microflow %s -- %s", indent, mf, caption))
	}

	// Outcomes, then boundary events — the order the grammar requires
	// (workflowCallMicroflowStmt: … OUTCOMES? BOUNDARY EVENT?). Emitting them
	// the other way round produced DESCRIBE output that would not re-parse:
	// "mismatched input 'outcomes' expecting ';'" (issue #948). It only showed
	// once the default engine could read boundary events back at all.
	lines = append(lines, formatConditionOutcomes(a.Outcomes, indent)...)
	lines = append(lines, formatBoundaryEvents(a.BoundaryEvents, indent+"  ")...)

	return lines
}

// formatSystemTask formats a system task for describe output.
func formatSystemTask(a *workflows.SystemTask, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	mf := a.Microflow
	if mf == "" {
		mf = "?"
	}

	lines = append(lines, fmt.Sprintf("%scall microflow %s -- %s", indent, mf, caption))

	// Outcomes
	lines = append(lines, formatConditionOutcomes(a.Outcomes, indent)...)

	return lines
}

// formatCallWorkflowActivity formats a call workflow activity for describe output.
func formatCallWorkflowActivity(a *workflows.CallWorkflowActivity, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	wf := a.Workflow
	if wf == "" {
		wf = "?"
	}

	if len(a.ParameterMappings) > 0 {
		var params []string
		for _, pm := range a.ParameterMappings {
			paramName := pm.Parameter
			if idx := strings.LastIndex(paramName, "."); idx >= 0 {
				paramName = paramName[idx+1:]
			}
			params = append(params, fmt.Sprintf("%s = %s", paramName, mdlQuoted(pm.Expression)))
		}
		lines = append(lines, fmt.Sprintf("%scall workflow %s comment %s with (%s)", indent, wf, mdlQuoted(caption), strings.Join(params, ", ")))
	} else {
		lines = append(lines, fmt.Sprintf("%scall workflow %s comment %s", indent, wf, mdlQuoted(caption)))
	}

	// BoundaryEvents
	lines = append(lines, formatBoundaryEvents(a.BoundaryEvents, indent+"  ")...)

	return lines
}

// formatExclusiveSplit formats an exclusive split (decision) for describe output.
func formatExclusiveSplit(a *workflows.ExclusiveSplitActivity, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	if a.Expression != "" {
		lines = append(lines, fmt.Sprintf("%sdecision %s -- %s", indent, mdlQuoted(a.Expression), caption))
	} else {
		lines = append(lines, fmt.Sprintf("%sdecision -- %s", indent, caption))
	}

	lines = append(lines, formatConditionOutcomes(a.Outcomes, indent)...)

	return lines
}

// formatParallelSplit formats a parallel split for describe output.
func formatParallelSplit(a *workflows.ParallelSplitActivity, indent string) []string {
	var lines []string

	if a.Annotation != "" {
		lines = append(lines, formatAnnotation(a.Annotation, indent))
	}

	caption := a.Caption
	if caption == "" {
		caption = a.Name
	}

	lines = append(lines, fmt.Sprintf("%sparallel split -- %s", indent, caption))
	for i, outcome := range a.Outcomes {
		lines = append(lines, fmt.Sprintf("%s  path %d {", indent, i+1))
		if outcome.Flow != nil && len(outcome.Flow.Activities) > 0 {
			subLines := formatWorkflowActivities(outcome.Flow, indent+"    ")
			lines = append(lines, subLines...)
		}
		lines = append(lines, fmt.Sprintf("%s  }", indent))
	}

	return lines
}

// formatConditionOutcomes formats condition outcomes for describe output.
func formatConditionOutcomes(outcomes []workflows.ConditionOutcome, indent string) []string {
	if len(outcomes) == 0 {
		return nil
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("%s  outcomes", indent))
	for _, outcome := range outcomes {
		name := outcome.GetName()
		flow := outcome.GetFlow()
		if flow != nil && len(flow.Activities) > 0 {
			lines = append(lines, fmt.Sprintf("%s    %s -> {", indent, name))
			subLines := formatWorkflowActivities(flow, indent+"      ")
			lines = append(lines, subLines...)
			lines = append(lines, fmt.Sprintf("%s    }", indent))
		} else {
			lines = append(lines, fmt.Sprintf("%s    %s -> { }", indent, name))
		}
	}

	return lines
}
