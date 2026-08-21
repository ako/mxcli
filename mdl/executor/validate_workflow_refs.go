// SPDX-License-Identifier: Apache-2.0

// Reference-based (project-connected) validation for workflows. Runs under
// `check --references`, where the target microflows can be introspected. See
// validate_workflow.go for the syntax-only (no-project) checks.
package executor

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateWorkflowParameterMappings checks that each workflow "call microflow"
// activity maps every parameter of its target microflow. Mendix rejects an
// unmapped parameter (CE6677 — "should accept parameters ...") and the workflow
// would fail at the activity, but mxcli check passed it (FINDINGS #40). Microflows
// created in the same script are skipped (not yet queryable); a target microflow
// that isn't in the project is left to the missing-reference check.
func validateWorkflowParameterMappings(ctx *ExecContext, s *ast.CreateWorkflowStmt, sc *scriptContext) []string {
	if ctx == nil || ctx.Backend == nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	mfs, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil
	}
	paramsByMF := make(map[string][]string, len(mfs))
	for _, mf := range mfs {
		names := make([]string, 0, len(mf.Parameters))
		for _, p := range mf.Parameters {
			names = append(names, p.Name)
		}
		paramsByMF[h.GetQualifiedName(mf.ContainerID, mf.Name)] = names
	}

	var errs []string
	walkWorkflowActivities(s.Activities, func(act ast.WorkflowActivityNode) {
		cm, ok := act.(*ast.WorkflowCallMicroflowNode)
		if !ok {
			return
		}
		mfQN := cm.Microflow.String()
		if sc != nil && sc.microflows[mfQN] {
			return // created in the same script — cannot introspect its parameters
		}
		want, known := paramsByMF[mfQN]
		if !known {
			return // target microflow not in project; the missing-ref check covers it
		}
		mapped := make(map[string]bool, len(cm.ParameterMappings))
		for _, pm := range cm.ParameterMappings {
			mapped[pm.Parameter] = true
		}
		for _, name := range want {
			if !mapped[name] {
				errs = append(errs, fmt.Sprintf(
					"call microflow '%s': parameter '%s' is not mapped — Mendix requires every parameter of a workflow call-microflow to be mapped (add `with (%s = ...)`)",
					mfQN, name, name))
			}
		}
	})
	return errs
}

// validateWorkflowReferences checks every qualified name a workflow's activities
// point at against the project (and against what the same script creates).
//
// This is the "missing-reference check" validateWorkflowParameterMappings above
// says it defers to, and which did not exist: a workflow calling a microflow
// that was nowhere in the project passed `check --references` with "All
// references valid" and was written by exec, leaving the Mendix validator
// (CE1613) as the only thing that noticed. The identical mistake inside a plain
// microflow body was caught, because validateFlowBodyReferences runs for
// microflows and nanoflows only (issue #943).
//
// The lookups and message wording deliberately match validateFlowBodyReferences,
// so the same mistake reads the same way wherever it is made.
func validateWorkflowReferences(ctx *ExecContext, activities []ast.WorkflowActivityNode, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() || len(activities) == 0 {
		return nil
	}

	// The lookups are built lazily: most workflows reference only microflows, and
	// each build is a full backend list.
	var microflowNames, workflowNames, pageNames map[string]bool
	knownMicroflow := func(qn string) bool {
		if microflowNames == nil {
			microflowNames = buildMicroflowQualifiedNames(ctx)
		}
		return microflowNames[qn] || (sc != nil && sc.microflows[qn])
	}
	knownWorkflow := func(qn string) bool {
		if workflowNames == nil {
			workflowNames = buildWorkflowQualifiedNames(ctx)
		}
		return workflowNames[qn] || (sc != nil && sc.workflows[qn])
	}
	knownPage := func(qn string) bool {
		if pageNames == nil {
			pageNames = buildPageQualifiedNames(ctx)
		}
		return pageNames[qn] || (sc != nil && sc.pages[qn])
	}

	var errs []string
	seen := map[string]bool{} // one report per distinct reference
	report := func(kind, qn, via string) {
		if qn == "" || seen[kind+qn] {
			return
		}
		// A System.* target is provided by the runtime and never appears in the
		// project, the same exemption validateFlowBodyReferences makes for Java
		// actions. Reporting it would be a guaranteed false positive.
		if isBuiltinModuleEntity(qualifiedNameModule(qn)) {
			return
		}
		seen[kind+qn] = true
		errs = append(errs, fmt.Sprintf("%s not found: %s (referenced by %s)", kind, qn, via))
	}

	walkWorkflowActivities(activities, func(act ast.WorkflowActivityNode) {
		switch n := act.(type) {
		case *ast.WorkflowCallMicroflowNode:
			if qn := n.Microflow.String(); qn != "." && !knownMicroflow(qn) {
				report("microflow", qn, "call microflow")
			}
		case *ast.WorkflowCallWorkflowNode:
			if qn := n.Workflow.String(); qn != "." && !knownWorkflow(qn) {
				report("workflow", qn, "call workflow")
			}
		case *ast.WorkflowUserTaskNode:
			if qn := n.Page.String(); qn != "." && qn != "" && !knownPage(qn) {
				report("page", qn, "user task page")
			}
			// Targeting by microflow, for users or for groups; the XPath variants
			// carry no qualified name.
			if qn := n.Targeting.Microflow.String(); qn != "." && qn != "" && !knownMicroflow(qn) {
				report("microflow", qn, "user task targeting")
			}
		}
	})
	return errs
}

// validateWorkflowStatementRefs is the CREATE WORKFLOW entry point: the context
// entity and the module the workflow is being created in, plus every activity
// reference.
//
// The module check matters more than it looks. exec creates a module on demand,
// so without it a typo'd module name silently produced a new module rather than
// an error — `check` is the only thing standing in the way, and it did this for
// a microflow but not for a workflow.
func validateWorkflowStatementRefs(ctx *ExecContext, s *ast.CreateWorkflowStmt, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	var errs []string
	if s.Name.Module != "" && (sc == nil || !sc.modules[s.Name.Module]) {
		if _, err := findModule(ctx, s.Name.Module); err != nil {
			errs = append(errs, fmt.Sprintf("module not found: %s", s.Name.Module))
		}
	}
	if qn := s.ParameterEntity.String(); qn != "" && qn != "." {
		if !isBuiltinModuleEntity(s.ParameterEntity.Module) {
			known := buildEntityQualifiedNames(ctx)
			if !known[qn] && (sc == nil || !sc.entities[qn]) {
				errs = append(errs, fmt.Sprintf("entity not found: %s (referenced by workflow parameter)", qn))
			}
		}
	}
	return append(errs, validateWorkflowReferences(ctx, s.Activities, sc)...)
}

// validateAlterWorkflowRefs validates ALTER WORKFLOW, which had no case in the
// validation switch at all and so fell through to "skip validation" — it got
// nothing, not even a check that the workflow it targets exists.
func validateAlterWorkflowRefs(ctx *ExecContext, s *ast.AlterWorkflowStmt, sc *scriptContext) []string {
	if ctx == nil || !ctx.Connected() {
		return nil
	}
	var errs []string
	qn := s.Name.String()
	if qn != "" && qn != "." && !isBuiltinModuleEntity(s.Name.Module) {
		known := buildWorkflowQualifiedNames(ctx)
		if !known[qn] && (sc == nil || !sc.workflows[qn]) {
			errs = append(errs, fmt.Sprintf("workflow not found: %s", qn))
		}
	}

	// Every op that introduces activities gets the same reference check a CREATE
	// body gets. Ops that only remove or rename something introduce no reference.
	var added []ast.WorkflowActivityNode
	for _, op := range s.Operations {
		switch o := op.(type) {
		case *ast.InsertAfterOp:
			added = append(added, o.NewActivity)
		case *ast.ReplaceActivityOp:
			added = append(added, o.NewActivity)
		case *ast.InsertOutcomeOp:
			added = append(added, o.Activities...)
		case *ast.InsertPathOp:
			added = append(added, o.Activities...)
		case *ast.InsertBranchOp:
			added = append(added, o.Activities...)
		case *ast.InsertBoundaryEventOp:
			added = append(added, o.Activities...)
		case *ast.SetActivityPropertyOp:
			// SET PAGE / SET TARGETING MICROFLOW name something that must exist.
			if p := o.PageName.String(); p != "" && p != "." && !isBuiltinModuleEntity(o.PageName.Module) {
				known := buildPageQualifiedNames(ctx)
				if !known[p] && (sc == nil || !sc.pages[p]) {
					errs = append(errs, fmt.Sprintf("page not found: %s (referenced by set activity page)", p))
				}
			}
			if m := o.Microflow.String(); m != "" && m != "." && !isBuiltinModuleEntity(o.Microflow.Module) {
				known := buildMicroflowQualifiedNames(ctx)
				if !known[m] && (sc == nil || !sc.microflows[m]) {
					errs = append(errs, fmt.Sprintf("microflow not found: %s (referenced by set activity targeting)", m))
				}
			}
		}
	}
	return append(errs, validateWorkflowReferences(ctx, added, sc)...)
}
