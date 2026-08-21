// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genMf "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genWf "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	"github.com/mendixlabs/mxcli/modelsdk/mprread"
	"github.com/mendixlabs/mxcli/sdk/workflows"
)

// ListWorkflows reads every Workflows$Workflow unit and converts it to the
// semantic type, mirroring the legacy (*mpr.Reader).ListWorkflows for the
// top-level fields plus the flow/activity tree the catalog's references walker
// consumes: the context Parameter entity, OverviewPage, and the activity types
// that carry references (user tasks → page/entity/user-source/outcome flows,
// call-microflow / call-workflow tasks, exclusive and parallel splits).
//
// Start/end, jump-to (with target), and the wait activities are reconstructed as
// typed activities so DESCRIBE round-trips them as executable MDL (Bug 11b).
// Documented gap: boundary events, parameter mappings and the completion/criteria
// sub-parts of multi-user tasks are not reconstructed — no catalog or describe
// path that reaches this method reads them. Unrecognised activity types decode to
// a GenericWorkflowActivity carrying their $Type, so no activity is silently dropped.
func (b *Backend) ListWorkflows() ([]*workflows.Workflow, error) {
	units, err := mprread.ListUnitsWithContainer[*genWf.Workflow](b.reader)
	if err != nil {
		return nil, err
	}
	out := make([]*workflows.Workflow, 0, len(units))
	for _, u := range units {
		g := u.Element
		w := &workflows.Workflow{
			ContainerID:         model.ID(u.ContainerID),
			Name:                g.Name(),
			Documentation:       g.Documentation(),
			ExportLevel:         g.ExportLevel(),
			Excluded:            g.Excluded(),
			OverviewPage:        g.OverviewPageQualifiedName(),
			DueDate:             g.DueDate(),
			WorkflowName:        workflowTemplateText(g.WorkflowName()),
			WorkflowDescription: workflowTemplateText(g.WorkflowDescription()),
		}
		w.ID = model.ID(g.ID())
		w.TypeName = "Workflows$Workflow"
		w.Annotation = annotationText(g.Annotation())
		if p, ok := g.Parameter().(*genWf.Parameter); ok && p != nil {
			wp := &workflows.WorkflowParameter{EntityRef: p.EntityQualifiedName()}
			wp.ID = model.ID(p.ID())
			w.Parameter = wp
		}
		if f, ok := g.Flow().(*genWf.Flow); ok && f != nil {
			w.Flow = workflowFlowFromGen(f)
		}
		out = append(out, w)
	}
	return out, nil
}

// workflowFlowFromGen converts a gen Flow to the semantic Flow.
func workflowFlowFromGen(g *genWf.Flow) *workflows.Flow {
	f := &workflows.Flow{}
	f.ID = model.ID(g.ID())
	for _, actEl := range g.ActivitiesItems() {
		if a := workflowActivityFromGen(actEl); a != nil {
			f.Activities = append(f.Activities, a)
		}
	}
	return f
}

// workflowActivityFromGen dispatches a gen activity element to its semantic type.
func workflowActivityFromGen(el element.Element) workflows.WorkflowActivity {
	switch a := el.(type) {
	case *genWf.UserTask:
		t := &workflows.UserTask{
			Page:            a.PageQualifiedName(),
			UserTaskEntity:  a.UserTaskEntityQualifiedName(),
			TaskName:        workflowTemplateText(a.TaskName()),
			TaskDescription: workflowTemplateText(a.TaskDescription()),
			DueDate:         a.DueDate(),
			UserSource:      userSourceFromGen(a.UserSource(), nil),
			OnCreated:       microflowEventName(a.OnCreatedEvent()),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$UserTask")
		if t.Page == "" {
			t.Page = taskPageName(a.TaskPage())
		}
		t.Outcomes = userTaskOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.SingleUserTaskActivity:
		t := &workflows.UserTask{
			TaskName:        workflowTemplateText(a.TaskName()),
			TaskDescription: workflowTemplateText(a.TaskDescription()),
			DueDate:         a.DueDate(),
			UserSource:      userSourceFromGen(a.UserSource(), a.UserTargeting()),
			OnCreated:       microflowEventName(a.OnCreatedEvent()),
			Page:            taskPageName(a.TaskPage()),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$SingleUserTaskActivity")
		t.BoundaryEvents = boundaryEventsFromGen(a.BoundaryEventsItems())
		t.Outcomes = userTaskOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.MultiUserTaskActivity:
		t := &workflows.UserTask{
			IsMulti:         true,
			TaskName:        workflowTemplateText(a.TaskName()),
			TaskDescription: workflowTemplateText(a.TaskDescription()),
			DueDate:         a.DueDate(),
			UserSource:      userSourceFromGen(a.UserSource(), a.UserTargeting()),
			OnCreated:       microflowEventName(a.OnCreatedEvent()),
			Page:            taskPageName(a.TaskPage()),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$MultiUserTaskActivity")
		t.BoundaryEvents = boundaryEventsFromGen(a.BoundaryEventsItems())
		t.Outcomes = userTaskOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.CallMicroflowTask:
		t := &workflows.CallMicroflowTask{Microflow: a.MicroflowQualifiedName()}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$CallMicroflowTask")
		t.BoundaryEvents = boundaryEventsFromGen(a.BoundaryEventsItems())
		t.Outcomes = conditionOutcomesFromGen(a.OutcomesItems())
		t.ParameterMappings = microflowParamMappingsFromGen(a.ParameterMappingsItems())
		return t
	case *genWf.CallMicroflowActivity:
		t := &workflows.CallMicroflowTask{Microflow: a.MicroflowQualifiedName()}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$CallMicroflowActivity")
		t.BoundaryEvents = boundaryEventsFromGen(a.BoundaryEventsItems())
		t.Outcomes = conditionOutcomesFromGen(a.OutcomesItems())
		t.ParameterMappings = microflowParamMappingsFromGen(a.ParameterMappingsItems())
		return t
	case *genWf.CallWorkflowActivity:
		t := &workflows.CallWorkflowActivity{
			Workflow:            a.WorkflowQualifiedName(),
			ParameterExpression: a.ParameterExpression(),
		}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$CallWorkflowActivity")
		t.BoundaryEvents = boundaryEventsFromGen(a.BoundaryEventsItems())
		return t
	case *genWf.ExclusiveSplitActivity:
		t := &workflows.ExclusiveSplitActivity{Expression: a.Expression()}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$ExclusiveSplitActivity")
		t.Outcomes = conditionOutcomesFromGen(a.OutcomesItems())
		return t
	case *genWf.ParallelSplitActivity:
		t := &workflows.ParallelSplitActivity{}
		setWfBase(&t.BaseWorkflowActivity, a.ID(), a.Name(), a.Caption(), a.Annotation(), "Workflows$ParallelSplitActivity")
		for _, oEl := range a.OutcomesItems() {
			if o, ok := oEl.(*genWf.ParallelSplitOutcome); ok {
				out := &workflows.ParallelSplitOutcome{}
				out.ID = model.ID(o.ID())
				if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
					out.Flow = workflowFlowFromGen(f)
				}
				t.Outcomes = append(t.Outcomes, out)
			}
		}
		return t
	default:
		return workflowSimpleActivityFromGen(el)
	}
}

// workflowSimpleActivityFromGen reconstructs the "simple" workflow activities
// that the codec decodes as untyped elements because they have no genWf struct:
// start/end (implicit in MDL), jump-to, and the two wait activities. Reading
// their fields from the raw BSON lets DESCRIBE emit executable, round-trippable
// MDL instead of a "-- [Workflows$…]" comment (Bug 11b). This mirrors the legacy
// parser_workflow.go cases; anything still unrecognised falls back to
// GenericWorkflowActivity so no activity is ever dropped.
func workflowSimpleActivityFromGen(el element.Element) workflows.WorkflowActivity {
	typeName := el.TypeName()
	raw := el.Raw()
	setBase := func(a *workflows.BaseWorkflowActivity) {
		a.ID = model.ID(el.ID())
		a.Name = genWf.RawFieldString(raw, "Name")
		a.Caption = genWf.RawFieldString(raw, "Caption")
		a.TypeName = typeName
	}
	switch typeName {
	case "Workflows$StartWorkflowActivity":
		a := &workflows.StartWorkflowActivity{}
		setBase(&a.BaseWorkflowActivity)
		return a
	case "Workflows$EndWorkflowActivity":
		a := &workflows.EndWorkflowActivity{}
		setBase(&a.BaseWorkflowActivity)
		return a
	case "Workflows$JumpToActivity":
		a := &workflows.JumpToActivity{}
		setBase(&a.BaseWorkflowActivity)
		a.TargetActivity = genWf.RawFieldString(raw, "TargetActivity")
		if a.TargetActivity == "" {
			a.TargetActivity = genWf.RawFieldString(raw, "TargetActivityName")
		}
		return a
	case "Workflows$WaitForTimerActivity":
		a := &workflows.WaitForTimerActivity{}
		setBase(&a.BaseWorkflowActivity)
		a.DelayExpression = genWf.RawFieldString(raw, "Delay")
		if a.DelayExpression == "" {
			a.DelayExpression = genWf.RawFieldString(raw, "DelayExpression")
		}
		return a
	case "Workflows$WaitForNotificationActivity":
		a := &workflows.WaitForNotificationActivity{}
		setBase(&a.BaseWorkflowActivity)
		return a
	default:
		t := &workflows.GenericWorkflowActivity{TypeString: typeName}
		t.ID = model.ID(el.ID())
		t.TypeName = typeName
		return t
	}
}

// userTaskOutcomesFromGen converts gen user-task outcomes to semantic ones.
func userTaskOutcomesFromGen(items []element.Element) []*workflows.UserTaskOutcome {
	var out []*workflows.UserTaskOutcome
	for _, oEl := range items {
		o, ok := oEl.(*genWf.UserTaskOutcome)
		if !ok {
			continue
		}
		uto := &workflows.UserTaskOutcome{
			Name:    o.Name(),
			Caption: o.Caption(),
			Value:   o.Value(),
		}
		uto.ID = model.ID(o.ID())
		if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
			uto.Flow = workflowFlowFromGen(f)
		}
		out = append(out, uto)
	}
	return out
}

// conditionOutcomesFromGen converts gen condition outcomes to semantic ones,
// mirroring the legacy parseConditionOutcomes dispatch.
// microflowParamMappingsFromGen reads a call-microflow activity's parameter
// mappings back into the semantic model. Without this, DESCRIBE WORKFLOW dropped
// the `with (...)` clause even though it was stored — a describe→drop→exec cycle
// silently lost the mapping (FINDINGS #42).
func microflowParamMappingsFromGen(items []element.Element) []*workflows.ParameterMapping {
	var out []*workflows.ParameterMapping
	for _, el := range items {
		pm, ok := el.(*genWf.MicroflowCallParameterMapping)
		if !ok {
			continue
		}
		m := &workflows.ParameterMapping{
			Parameter:  pm.ParameterQualifiedName(),
			Expression: pm.Expression(),
		}
		m.ID = model.ID(pm.ID())
		out = append(out, m)
	}
	return out
}

func conditionOutcomesFromGen(items []element.Element) []workflows.ConditionOutcome {
	var out []workflows.ConditionOutcome
	for _, el := range items {
		switch o := el.(type) {
		case *genWf.BooleanConditionOutcome:
			c := &workflows.BooleanConditionOutcome{Value: o.Value()}
			c.ID = model.ID(o.ID())
			if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
				c.Flow = workflowFlowFromGen(f)
			}
			out = append(out, c)
		case *genWf.EnumerationValueConditionOutcome:
			c := &workflows.EnumerationValueConditionOutcome{Value: o.ValueQualifiedName()}
			c.ID = model.ID(o.ID())
			if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
				c.Flow = workflowFlowFromGen(f)
			}
			out = append(out, c)
		case *genWf.VoidConditionOutcome:
			c := &workflows.VoidConditionOutcome{}
			c.ID = model.ID(o.ID())
			if f, ok := o.Flow().(*genWf.Flow); ok && f != nil {
				c.Flow = workflowFlowFromGen(f)
			}
			out = append(out, c)
		}
	}
	return out
}

// userSourceFromGen resolves the polymorphic user source / user targeting part,
// mirroring the legacy parseUserSource.
func userSourceFromGen(source, targeting element.Element) workflows.UserSource {
	el := source
	if el == nil {
		el = targeting
	}
	switch s := el.(type) {
	case *genWf.MicroflowBasedUserSource:
		return &workflows.MicroflowBasedUserSource{Microflow: s.MicroflowQualifiedName()}
	case *genWf.MicroflowUserTargeting:
		return &workflows.MicroflowBasedUserSource{Microflow: s.MicroflowQualifiedName()}
	case *genWf.MicroflowGroupTargeting:
		return &workflows.MicroflowGroupSource{Microflow: s.MicroflowQualifiedName()}
	case *genWf.XPathBasedUserSource:
		return &workflows.XPathBasedUserSource{XPath: s.XPathConstraint()}
	case *genWf.XPathUserTargeting:
		return &workflows.XPathBasedUserSource{XPath: s.XPathConstraint()}
	default:
		return &workflows.NoUserSource{}
	}
}

// microflowEventName extracts the microflow qualified name from an OnCreated
// event part (Workflows$MicroflowBasedEvent).
func microflowEventName(el element.Element) string {
	if ev, ok := el.(*genWf.MicroflowBasedEvent); ok && ev != nil {
		return ev.MicroflowQualifiedName()
	}
	return ""
}

// taskPageName extracts the page qualified name from a TaskPage part
// (Workflows$PageReference).
func taskPageName(el element.Element) string {
	if pr, ok := el.(*genWf.PageReference); ok && pr != nil {
		return pr.PageQualifiedName()
	}
	return ""
}

// annotationText extracts the description text from a Workflows$Annotation part.
func annotationText(el element.Element) string {
	if an, ok := el.(*genWf.Annotation); ok && an != nil {
		return an.Description()
	}
	return ""
}

// workflowTemplateText extracts the Text of a Microflows$StringTemplate part,
// mirroring the legacy extractStringTemplate.
func workflowTemplateText(el element.Element) string {
	if st, ok := el.(*genMf.StringTemplate); ok && st != nil {
		return st.Text()
	}
	return ""
}

// setWfBase fills a semantic BaseWorkflowActivity from common gen accessors.
func setWfBase(a *workflows.BaseWorkflowActivity, id element.ID, name, caption string, annotation element.Element, typeName string) {
	a.ID = model.ID(id)
	a.Name = name
	a.Caption = caption
	a.TypeName = typeName
	a.Annotation = annotationText(annotation)
}

// boundaryEventsFromGen reconstructs an activity's boundary events, including
// each one's handler flow.
//
// Both engines WRITE boundary events; only the legacy parser
// (sdk/mpr/parser_workflow.go, parseBoundaryEvents) read them back. So a boundary
// event mxcli had just written read back as absent on the default engine:
// DESCRIBE rendered nothing, and describe → edit → re-exec silently dropped the
// timer, its handler flow and the jump inside it (issue #948).
//
// The delay lives under FirstExecutionTime for every timer variant — Delay() is
// a separate gen accessor that real documents leave empty — so it is read first
// and Delay() is only a fallback, matching the legacy parser.
func boundaryEventsFromGen(items []element.Element) []*workflows.BoundaryEvent {
	var out []*workflows.BoundaryEvent
	for _, el := range items {
		be := boundaryEventFromGen(el)
		if be != nil {
			out = append(out, be)
		}
	}
	return out
}

func boundaryEventFromGen(el element.Element) *workflows.BoundaryEvent {
	// The three timer variants differ only in $Type, and gen gives each its own
	// concrete type, so the shared shape is read through a small interface rather
	// than repeated three times.
	type timerBoundary interface {
		Caption() string
		FirstExecutionTime() string
		Delay() string
		Flow() element.Element
	}
	t, ok := el.(timerBoundary)
	if !ok {
		return nil
	}
	be := &workflows.BoundaryEvent{Caption: t.Caption()}
	be.ID = model.ID(el.ID())
	be.TimerDelay = t.FirstExecutionTime()
	if be.TimerDelay == "" {
		be.TimerDelay = t.Delay()
	}
	switch el.(type) {
	case *genWf.InterruptingTimerBoundaryEvent:
		be.EventType = "InterruptingTimer"
	case *genWf.NonInterruptingTimerBoundaryEvent:
		be.EventType = "NonInterruptingTimer"
	case *genWf.TimerBoundaryEvent:
		be.EventType = "Timer"
	default:
		return nil // an unknown boundary-event kind is not silently mistyped
	}
	if f, ok := t.Flow().(*genWf.Flow); ok && f != nil {
		be.Flow = workflowFlowFromGen(f)
	}
	return be
}
