// SPDX-License-Identifier: Apache-2.0

// Package executor - nanoflow-specific validation rules
package executor

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

// validateNanoflowBody checks that a nanoflow body does not contain disallowed
// actions or flow objects. Returns a list of human-readable error messages.
func validateNanoflowBody(body []ast.MicroflowStatement) []string {
	var errors []string
	validateNanoflowStatements(body, &errors)
	return errors
}

func validateNanoflowStatements(stmts []ast.MicroflowStatement, errors *[]string) {
	for _, stmt := range stmts {
		if reason := checkDisallowedNanoflowAction(stmt); reason != "" {
			*errors = append(*errors, reason)
			continue
		}
		if reason := checkNanoflowErrorHandling(stmt); reason != "" {
			*errors = append(*errors, reason)
		}
		// Recurse into compound statements
		switch s := stmt.(type) {
		case *ast.IfStmt:
			validateNanoflowStatements(s.ThenBody, errors)
			validateNanoflowStatements(s.ElseBody, errors)
		case *ast.LoopStmt:
			validateNanoflowStatements(s.Body, errors)
		case *ast.WhileStmt:
			validateNanoflowStatements(s.Body, errors)
		}
		// Also recurse into error handling bodies
		if eh := getErrorHandling(stmt); eh != nil && eh.Body != nil {
			validateNanoflowStatements(eh.Body, errors)
		}
	}
}

// checkDisallowedNanoflowAction returns a human-readable error message if the
// statement is not allowed in nanoflows, or empty string if allowed.
//
// MAINTENANCE: This uses a denylist approach (12 case branches, 22 action types) —
// any action type NOT listed here is implicitly allowed. When adding new action
// AST types, check whether they are available in nanoflows (see Mendix docs
// "Nanoflows" > "Activities") and add a case here if they are server-side only.
// The manual QA test plan (docs/15-testing/nanoflow-test-cases.md §4.1) lists
// all disallowed actions and should be updated in parallel.
func checkDisallowedNanoflowAction(stmt ast.MicroflowStatement) string {
	switch stmt.(type) {
	case *ast.RaiseErrorStmt:
		return "ErrorEvent is not allowed in nanoflows"
	case *ast.CallJavaActionStmt:
		return "Java actions cannot be called from nanoflows"
	case *ast.ExecuteDatabaseQueryStmt:
		return "database queries are not allowed in nanoflows"
	case *ast.CallExternalActionStmt:
		return "external action calls are not allowed in nanoflows"
	case *ast.ShowHomePageStmt:
		return "SHOW HOME PAGE is not allowed in nanoflows"
	case *ast.RestCallStmt:
		return "REST calls are not allowed in nanoflows"
	case *ast.SendRestRequestStmt:
		return "REST requests are not allowed in nanoflows"
	case *ast.ImportFromMappingStmt:
		return "import mapping is not allowed in nanoflows"
	case *ast.ExportToMappingStmt:
		return "export mapping is not allowed in nanoflows"
	case *ast.TransformJsonStmt:
		return "JSON transformation is not allowed in nanoflows"
	// Workflow actions — all server-side only
	case *ast.CallWorkflowStmt,
		*ast.GetWorkflowDataStmt,
		*ast.GetWorkflowsStmt,
		*ast.GetWorkflowActivityRecordsStmt,
		*ast.WorkflowOperationStmt,
		*ast.SetTaskOutcomeStmt,
		*ast.OpenUserTaskStmt,
		*ast.NotifyWorkflowStmt,
		*ast.OpenWorkflowStmt,
		*ast.LockWorkflowStmt,
		*ast.UnlockWorkflowStmt:
		return "workflow actions are not allowed in nanoflows"
	case *ast.DownloadFileStmt:
		return "file downloads are not allowed in nanoflows"
	}
	return ""
}

// getErrorHandling extracts the ErrorHandlingClause from statements that have one.
//
// Only statements reachable in nanoflows (i.e., NOT in the denylist) need coverage
// here. Disallowed actions are rejected by checkDisallowedNanoflowAction before
// this function is called. Statements like ListOperationStmt that have no
// ErrorHandling field are also omitted (they return nil implicitly via default).
func getErrorHandling(stmt ast.MicroflowStatement) *ast.ErrorHandlingClause {
	switch s := stmt.(type) {
	case *ast.CreateObjectStmt:
		return s.ErrorHandling
	case *ast.MfCommitStmt:
		return s.ErrorHandling
	case *ast.DeleteObjectStmt:
		return s.ErrorHandling
	case *ast.RetrieveStmt:
		return s.ErrorHandling
	case *ast.CallMicroflowStmt:
		return s.ErrorHandling
	case *ast.CallNanoflowStmt:
		return s.ErrorHandling
	case *ast.CallJavaScriptActionStmt:
		return s.ErrorHandling
	// The eight statements mendixlabs/mxcli#1078 gave an onErrorClause. None is
	// on the denylist above, so all eight are reachable in a nanoflow — and
	// without them here their handler BODIES are never walked, so a Java action
	// or REST call nested inside `declare … on error { … }` would go unreported.
	case *ast.DeclareStmt:
		return s.ErrorHandling
	case *ast.MfSetStmt:
		return s.ErrorHandling
	case *ast.ChangeObjectStmt:
		return s.ErrorHandling
	case *ast.LogStmt:
		return s.ErrorHandling
	case *ast.ShowPageStmt:
		return s.ErrorHandling
	case *ast.ClosePageStmt:
		return s.ErrorHandling
	case *ast.ShowMessageStmt:
		return s.ErrorHandling
	case *ast.ValidationFeedbackStmt:
		return s.ErrorHandling
	}
	return nil
}

// validateNanoflowReturnType checks that the return type is allowed for nanoflows.
// Binary return type is not supported in nanoflows.
func validateNanoflowReturnType(retType *ast.MicroflowReturnType) string {
	if retType == nil {
		return ""
	}
	switch retType.Type.Kind {
	case ast.TypeBinary:
		return "Binary return type is not allowed in nanoflows"
	}
	return ""
}

// validateNanoflow runs all nanoflow-specific validations and returns a combined
// error message, or empty string if valid.
func validateNanoflow(name string, body []ast.MicroflowStatement, retType *ast.MicroflowReturnType) string {
	var allErrors []string

	if msg := validateNanoflowReturnType(retType); msg != "" {
		allErrors = append(allErrors, msg)
	}

	allErrors = append(allErrors, validateNanoflowBody(body)...)

	if len(allErrors) == 0 {
		return ""
	}

	var errMsg strings.Builder
	errMsg.WriteString(fmt.Sprintf("nanoflow '%s' has validation errors:\n", name))
	for _, e := range allErrors {
		errMsg.WriteString(fmt.Sprintf("  - %s\n", e))
	}
	return errMsg.String()
}

// nanoflowErrorHandlingUnsupported names the activities that accept NO error
// handling at all inside a nanoflow, by the caption mxbuild uses.
//
// MEASURED on Mendix 11.14.0, one nanoflow per cell, not inferred. A dash is a
// cell that was NOT measured, not one that passed — every activity listed in the
// map below has at least one measured CE6035, and none has a measured pass:
//
//	activity (in a NANOFLOW)   continue   rollback   custom { }
//	CreateVariable (declare)      ok          -          ok
//	ChangeVariable (set)           -          -          ok
//	ChangeObject                   -       CE6035      CE6035
//	Log                         CE6035     CE6035      CE6035
//	ShowPage                       -          -        CE6035
//	ClosePage                      -       CE6035      CE6035
//	ShowMessage                    -          -        CE6035
//	ValidationFeedback             -       CE6035      CE6035
//
// The rollback column comes from #1078's own regression: writing "Rollback"
// instead of the nanoflow default Abort was CE6035 on exactly ChangeObject,
// ClosePage and ValidationFeedback in the doctype scripts. Log is the one
// activity measured in all three forms, and rejects all three — which is why the
// rule refuses any clause rather than one form: the accepted value is Abort, and
// no MDL syntax writes it.
//
// The two VARIABLE activities are the permissive pair here, exactly as they are
// for `continue` in a microflow (see continueUnsupportedOn) — the split is by
// activity, not by "client-side vs server-side".
//
// This is a nanoflow-only rule and cannot live in MDL076: that one runs on the
// microflow validator, which has no flow flavour. Reaching this at all is new —
// mendixlabs/mxcli#1078 gave these six statements an onErrorClause so a Studio
// Pro error handler could survive DESCRIBE, and a nanoflow accepts none of them.
var nanoflowErrorHandlingUnsupported = map[string]string{
	"change":              "Change object activity",
	"log":                 "Log message activity",
	"show page":           "Show page activity",
	"close page":          "Close page activity",
	"show message":        "Show message activity",
	"validation feedback": "Validation feedback activity",
}

// checkNanoflowErrorHandling reports an ON ERROR clause on a nanoflow activity
// that cannot carry one.
//
// Refused rather than dropped or downgraded: the alternatives are a nanoflow
// mxbuild rejects (CE6035) or one that silently does something the script does
// not say. `mxcli check` passed the rejected form until this rule existed.
func checkNanoflowErrorHandling(stmt ast.MicroflowStatement) string {
	if getErrorHandling(stmt) == nil {
		return ""
	}
	var keyword string
	switch stmt.(type) {
	case *ast.ChangeObjectStmt:
		keyword = "change"
	case *ast.LogStmt:
		keyword = "log"
	case *ast.ShowPageStmt:
		keyword = "show page"
	case *ast.ClosePageStmt:
		keyword = "close page"
	case *ast.ShowMessageStmt:
		keyword = "show message"
	case *ast.ValidationFeedbackStmt:
		keyword = "validation feedback"
	default:
		// declare and set are deliberately absent: both variable activities accept
		// every form in a nanoflow. So do the statements that could already carry
		// the clause (commit, create, retrieve, the calls) — unmeasured here, and
		// refusing them would reject nanoflows that build today.
		return ""
	}
	return "`" + keyword + " ... on error` is not supported in a nanoflow — Mendix rejects " +
		"error handling on a " + nanoflowErrorHandlingUnsupported[keyword] +
		" there with CE6035 \"Error handling type is not supported\". Drop the clause " +
		"(a nanoflow activity aborts the flow on error by default)"
}
