// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// versionGate is one feature a statement needs from the project's Mendix
// version: the sdk/versions registry entry (area, name) that holds its floor,
// how the refusal names the statement, and what to do instead.
type versionGate struct {
	area, name string
	statement  string
	hint       string
}

// statementVersionGates lists the version gates stmt has to pass, in the order
// exec applies them. It is the one place a statement is mapped to the features
// it needs: the handlers refuse through checkVersionGates, and `mxcli check -p`
// and exec's pre-flight report through CheckVersionGates, so the three cannot
// disagree about which statement needs which version (mendixlabs/mxcli#1434 —
// check passed a script exec then refused for the project's version). The
// floors themselves live in sdk/versions/*.yaml.
//
// A gate that depends on what the project stores, not only on the statement
// (requireMessageDefinitionDocuments on a drop/alter of a document that is not
// there), stays with its handler: check cannot know it from the script.
func statementVersionGates(stmt ast.Statement) []versionGate {
	switch s := stmt.(type) {
	case *ast.CreateRestClientStmt:
		return []versionGate{{"integration", "rest_client_basic", "create rest client",
			"upgrade your project to 10.1+"}}
	case *ast.AlterRestClientStmt:
		return []versionGate{{"integration", "rest_client_basic", "alter consumed rest service",
			"upgrade your project to 10.1+"}}
	case *ast.CreateDataTransformerStmt:
		return []versionGate{{"integration", "data_transformer", "create data transformer",
			"upgrade your project to 11.9+"}}
	case *ast.CreatePublishedRestServiceStmt:
		return []versionGate{{"integration", "published_rest_service", "create published rest service",
			"upgrade your project to 10.0+"}}
	case *ast.AlterPublishedRestServiceStmt:
		return []versionGate{{"integration", "published_rest_alter", "alter published rest service",
			"upgrade your project to 10.0+"}}
	case *ast.GrantPublishedRestServiceAccessStmt:
		return []versionGate{{"integration", "published_rest_grant_revoke", "grant access on published rest service",
			"upgrade your project to 10.0+"}}
	case *ast.CreateODataServiceStmt:
		// The document mxcli writes, ODataPublish$PublishedODataService2, is
		// 10.4+; before, a published OData service is a Rest$PublishedODataService,
		// a different document mxcli does not write (ako/mxcli#1163).
		gates := []versionGate{{"integration", "published_odata_service", "create published odata service",
			"Before Mendix 10.4 a published OData service is a different document " +
				"(Rest$PublishedODataService), which mxcli does not write. Publish it in Studio Pro, or upgrade the project."}}
		if s.SupportsGraphQL {
			gates = append(gates, versionGate{"integration", "odata_graphql", "SupportsGraphQL on a published OData service",
				"Publishing a service as GraphQL as well arrived in Studio Pro 10.14. " +
					"Remove SupportsGraphQL to publish OData only."})
		}
		return gates
	case *ast.CreateMessageDefinitionStmt:
		return []versionGate{messageDefinitionDocumentGate("create message definition")}
	case *ast.AlterSettingsStmt:
		if (s.AddGroup || s.ModifyGroup || s.UpsertGroup || s.RemoveGroup) && strings.EqualFold(s.Section, "workflows") {
			return []versionGate{{"workflows", "groups", "alter settings workflows add group '<name>'",
				"Workflow groups (Settings$WorkflowGroup) were introduced in the 11.2 metamodel. " +
					"On an earlier version, model the buckets as your own entity and target user tasks with a microflow."}}
		}
	case *ast.CreateViewEntityStmt:
		gates := []versionGate{{"domain_model", "view_entities", "create view entity",
			"upgrade your project to 10.18+ or use a regular entity with a microflow data source"}}
		// An OQL comment — `--` or `/* */`, anywhere in the query — is CE0174
		// "Error(s) in OQL query: The '-' part is incomplete or incorrect" below
		// the floor. The query now keeps a comment before its first token or
		// after its last (ako/mxcli#1632); one between clauses it always kept, and
		// check passed what mxbuild then rejected. Gated on a COMMENT, not on the
		// characters, so `'--'` in a string literal is not one.
		if q := s.Query.RawQuery; stripOQLComments(q) != q {
			gates = append(gates, versionGate{"oql_functions", "comments",
				"a comment in a view entity's OQL query",
				"remove the comments from the query — Mendix rejects them as CE0174 before 11.7 — or upgrade the project"})
		}
		// STRING_AGG is an OQL aggregate from 11.2, and from 10.24.22 on the
		// 10.24 line (ako/mxcli#1650). Below that floor mxbuild has no such
		// function, and the view fails the build.
		if oqlUsesStringAgg(s.Query.RawQuery) {
			gates = append(gates, versionGate{"oql_functions", "string_agg",
				"STRING_AGG in a view entity's OQL query",
				"Mendix added STRING_AGG to OQL in 11.2 and in 10.24.22 — upgrade the project, " +
					"or build the list in a microflow"})
		}
		return gates
	// A domain-model EXPORT LEVEL is 9.3+ (mendixlabs/mxcli#1498). Gated on the
	// clause being STATED, as the deep-link URL is: a statement without one
	// carries the stored level and writes nothing new.
	case *ast.CreateEntityStmt:
		if s.ExportLevel != nil || attributesStateExportLevel(s.Attributes) {
			return []versionGate{exportLevelGate("export level on an entity or attribute")}
		}
	case *ast.AlterEntityStmt:
		if s.Operation == ast.AlterEntitySetExportLevel || s.ModifyExportLevel != nil ||
			(s.Attribute != nil && s.Attribute.ExportLevel != nil) {
			return []versionGate{exportLevelGate("export level on an entity or attribute")}
		}
	case *ast.CreateAssociationStmt:
		if s.ExportLevel != nil {
			return []versionGate{exportLevelGate("export level on an association")}
		}
	case *ast.AlterAssociationStmt:
		if s.Operation == ast.AlterAssociationSetExportLevel {
			return []versionGate{exportLevelGate("export level on an association")}
		}
	case *ast.CreateEnumerationStmt:
		if s.ExportLevel != nil {
			return []versionGate{exportLevelGate("export level on an enumeration")}
		}
	case *ast.AlterEnumerationStmt:
		if s.Operation == ast.AlterEnumSetExportLevel {
			return []versionGate{exportLevelGate("export level on an enumeration")}
		}
	case *ast.CreateModelStmt:
		return []versionGate{agentDocumentGate("agent_model", "create model")}
	case *ast.CreateConsumedMCPServiceStmt:
		return []versionGate{agentDocumentGate("agent_consumed_mcp_service", "create consumed mcp service")}
	case *ast.CreateKnowledgeBaseStmt:
		return []versionGate{agentDocumentGate("agent_knowledge_base", "create knowledge base")}
	case *ast.CreateAgentStmt:
		return []versionGate{agentDocumentGate("agent", "create agent")}
	case *ast.CreatePageStmtV3:
		var gates []versionGate
		// Pages$PageParameter is 9.4+ (mendixlabs/mxcli#1121): 9.0–9.3 has none.
		if len(s.Parameters) > 0 {
			gates = append(gates, versionGate{"pages", "page_parameters", "create page with parameters",
				"pass data via a non-persistent entity or microflow parameter instead"})
		}
		return append(gates, flowReturnsGates(s)...)
	case *ast.CreateSnippetStmtV3, *ast.AlterPageStmt:
		return flowReturnsGates(s)
	case *ast.CreateMicroflowStmt:
		var gates []versionGate
		// SEND EMAIL writes the 11.13+ shape (StringTemplate subject and
		// bodies); an older project stores a different EmailMessage, or no
		// such activity.
		if bodyContains(s.Body, func(st ast.MicroflowStatement) bool { _, ok := st.(*ast.SendEmailStmt); return ok }) {
			gates = append(gates, versionGate{"microflows", "send_email", "send email",
				"use the Email Connector module, or upgrade the project to 11.13+"})
		}
		// Deep links are Mendix 10.6+. Gated on the clause being STATED, not on
		// the resulting value: a rewrite that carries a stored URL forward on
		// an older project preserves what is already there.
		if s.URL != nil || s.URLSearchParameters != nil {
			gates = append(gates, versionGate{"microflows", "deep_link_url", "a microflow URL (deep link)",
				"upgrade your project to 10.6+, or set the URL in Studio Pro"})
		}
		return gates
	case *ast.CreateNanoflowStmt:
		// SYNCHRONIZE UNSYNCHRONIZED is the one mode with a floor inside
		// Mendix 9: SynchronizationType.Unsynchronized was introduced in 9.4.0,
		// while the activity and its other two modes go back further. Gate the
		// mode, not the statement, so a 9.0-9.3 project keeps `synchronize all`.
		if bodyUsesUnsynchronized(s.Body) {
			return []versionGate{{"microflows", "synchronize_unsynchronized", "synchronize unsynchronized",
				"use `synchronize all` or `synchronize $Var`, or upgrade the project to 9.4+"}}
		}
	case *ast.CreateWorkflowStmt:
		var gates []versionGate
		if len(s.EventHandlers) > 0 {
			gates = append(gates, versionGate{"workflows", "event_handlers", "on workflow events",
				"remove the `on … workflow event` clauses, or upgrade the project"})
		}
		activities := workflowStatementActivities(s)
		if workflowUsesAgentTask(activities) {
			gates = append(gates, agentTaskGate)
		}
		if len(s.EventSubProcesses) > 0 {
			gates = append(gates, versionGate{"workflows", "event_subprocesses", "event subprocess",
				"event sub-processes need Mendix 11.8 or later"})
		}
		for _, esp := range s.EventSubProcesses {
			if esp.Timer {
				gates = append(gates, versionGate{"workflows", "timer_event_subprocesses", "event subprocess … on … timer",
					"a timer-started event sub-process needs Mendix 11.13 or later — start it with a notification on older projects"})
				break
			}
		}
		for _, esp := range s.EventSubProcesses {
			if esp.Recurrence != nil {
				gates = append(gates, versionGate{"workflows", "recurring_timer_event_subprocesses", "event subprocess … repeat every",
					"a repeating event sub-process start needs Mendix 11.15 or later — the 11.13 runtime ignores the recurrence and starts it once; " +
						"repeat a non-interrupting boundary timer instead (`boundary event non interrupting timer … repeat every …`)"})
				break
			}
		}
		if workflowUsesNotificationEvents(activities) {
			gates = append(gates, notificationEventsGate)
		}
		return append(gates, userTaskGates(activities)...)
	case *ast.AlterWorkflowStmt:
		// Workflows require Mendix 9.12+.
		gates := []versionGate{{"workflows", "basic", "alter workflow",
			"upgrade your project to Mendix 9.12+ to use workflows"}}
		added := alterWorkflowAddedActivities(s)
		if workflowUsesAgentTask(added) {
			gates = append(gates, agentTaskGate)
		}
		if workflowUsesNotificationEvents(added) {
			gates = append(gates, notificationEventsGate)
		}
		return append(gates, userTaskGates(added)...)
	}
	return nil
}

// userTaskGates are the gates of what the user tasks among activities state
// that an older user task cannot store, each once (ako/mxcli#1771). Below 11.2
// a task targets users only, and a multi-user task, its later rules and
// `await all users` arrived in 9.22, 10.0, 10.3 and 10.2 (mendixmodelsdk
// 4.117.0). The write refuses them too (canon.LowerToProjectShape); this names
// the statement before anything is written.
func userTaskGates(activities []ast.WorkflowActivityNode) []versionGate {
	var groups, multi, majority, threshold, microflow, await bool
	walkWorkflowActivities(activities, func(a ast.WorkflowActivityNode) {
		ut, ok := a.(*ast.WorkflowUserTaskNode)
		if !ok {
			return
		}
		switch ut.Targeting.Kind {
		case "group_xpath", "group_microflow":
			groups = true
		}
		if !ut.IsMultiUser {
			return
		}
		multi = true
		await = await || ut.AwaitAllUsers
		if ut.Completion != nil {
			switch ut.Completion.Rule {
			case "majority":
				majority = true
			case "threshold":
				threshold = true
			case "microflow":
				microflow = true
			}
		}
	})
	var gates []versionGate
	if groups {
		gates = append(gates, versionGate{"workflows", "group_targeting", "targeting groups on a user task",
			"Before Mendix 11.2 a user task targets users only: `targeting users xpath […]`, or a microflow " +
				"returning users. Workflow groups arrived in 11.2."})
	}
	if multi {
		gates = append(gates, versionGate{"workflows", "multi_user_task", "multi user task",
			"Mendix added multi-user tasks in 9.22: use a user task, or upgrade the project"})
	}
	for _, r := range []struct {
		used bool
		rule string
	}{{majority, "majority"}, {threshold, "threshold"}} {
		if r.used {
			gates = append(gates, versionGate{"workflows", "multi_user_task_majority_threshold", "decide by " + r.rule,
				"Mendix added the majority and threshold rules in 10.0: decide by consensus or veto, or upgrade the project"})
		}
	}
	if microflow {
		gates = append(gates, versionGate{"workflows", "multi_user_task_microflow_rule", "decide by microflow",
			"Mendix added deciding a multi-user task by a microflow in 10.3: decide by consensus or veto, or upgrade the project"})
	}
	if await {
		gates = append(gates, versionGate{"workflows", "multi_user_task_await_all", "await all users",
			"Mendix added `await all users` in 10.2: leave it out, so the task completes once its rule is met, or upgrade the project"})
	}
	return gates
}

// The floors of a flow call's `returns ( … )` mappings (ako/mxcli#977). The
// builder applies the same two, for the widgets this walk cannot see (a
// fragment's, expanded at build time).
var (
	flowReturnsGate = versionGate{"pages", "flow_call_output_mappings",
		"returns ( … ) on a flow call", "set the values from inside the flow instead"}
	flowReturnsToAttributeGate = versionGate{"pages", "flow_call_output_mappings_to_attribute",
		"returns ( … ) into an attribute", "map into a page variable ($name = …) instead"}
)

// flowReturnsGates are the gates of the `returns ( … )` mappings a page,
// snippet or alter page statement writes: 11.5 for any, 11.15 for one into an
// attribute — mx 11.13 and 11.14 report CE7006 on it.
func flowReturnsGates(stmt ast.Statement) []versionGate {
	var mapped, toAttribute bool
	visit := func(v any) {
		for a, _ := v.(*ast.ActionV3); a != nil; a = a.ThenAction {
			for _, r := range a.Returns {
				mapped = true
				toAttribute = toAttribute || !r.Variable
			}
		}
	}
	forEachWidget(stmt, func(w *ast.WidgetV3, _ string) {
		for _, v := range w.Properties {
			visit(v)
		}
	})
	if s, ok := stmt.(*ast.AlterPageStmt); ok {
		for _, op := range s.Operations {
			if o, ok := op.(*ast.SetPropertyOp); ok {
				for _, v := range o.Properties {
					visit(v)
				}
			}
		}
	}
	var gates []versionGate
	if mapped {
		gates = append(gates, flowReturnsGate)
	}
	if toAttribute {
		gates = append(gates, flowReturnsToAttributeGate)
	}
	return gates
}

// oqlStringAggCallRe is a STRING_AGG call in query text as written.
var oqlStringAggCallRe = regexp.MustCompile(`(?i)(^|[^A-Za-z0-9_."` + "`" + `])string_agg\s*\(`)

// oqlUsesStringAgg reports whether a view's query calls STRING_AGG — outside
// comments and string literals, so `'string_agg('` as a label is not a call.
func oqlUsesStringAgg(oql string) bool {
	return oqlStringAggCallRe.MatchString(oqlStringLiteralRe.ReplaceAllString(stripOQLComments(oql), "''"))
}

func exportLevelGate(statement string) versionGate {
	return versionGate{"domain_model", "export_levels", statement,
		"Mendix stores no export level on domain-model elements before 9.3 — remove the clause, or upgrade the project"}
}

func attributesStateExportLevel(attrs []ast.Attribute) bool {
	for _, a := range attrs {
		if a.ExportLevel != nil {
			return true
		}
	}
	return false
}

var (
	agentTaskGate = versionGate{"workflows", "ai_agent_task", "call agent microflow",
		"AI agent tasks need Mendix 11.9 or later — use `call microflow` on older projects"}
	notificationEventsGate = versionGate{"workflows", "notification_events",
		"notification activities and notification boundary events",
		"these need Mendix 11.11 or later — use `wait for notification` on older projects"}
)

// agentDocumentGate is the gate of an agent-editor document. Nothing downstream
// catches an older project: the documents are custom blobs, so mxbuild does not
// validate them and the build stays green while Studio Pro cannot open the
// result.
func agentDocumentGate(name, statement string) versionGate {
	return versionGate{"agent_documents", name, statement,
		"upgrade your project to Mendix 11.9+ and install the AgentEditorCommons module"}
}

// messageDefinitionDocumentGate refuses the per-document message definition
// form below 11.15.
func messageDefinitionDocumentGate(statement string) versionGate {
	return versionGate{"integration", "message_definition_document", statement,
		"below Mendix 11.15 a message definition is an entry of a collection: " +
			"create message definition collection Module.Name { definition Name for Module.Entity { … } };"}
}

// requireMessageDefinitionDocuments refuses the per-document form below 11.15.
func requireMessageDefinitionDocuments(ctx *ExecContext, statement string) error {
	return checkGate(ctx, messageDefinitionDocumentGate(statement))
}

func checkGate(ctx *ExecContext, g versionGate) error {
	return checkFeature(ctx, g.area, g.name, g.statement, g.hint)
}

// checkVersionGates refuses stmt when the connected project's version lacks a
// feature it needs: the first of statementVersionGates that fails. Nil when not
// connected, as checkFeature is.
func checkVersionGates(ctx *ExecContext, stmt ast.Statement) error {
	for _, g := range statementVersionGates(stmt) {
		if err := checkGate(ctx, g); err != nil {
			return err
		}
	}
	return nil
}

// CheckVersionGates reports every statement of prog the connected project's
// Mendix version cannot store — the refusals exec's handlers would raise when
// they reach it, after the statements before it. `mxcli check -p` and exec's
// pre-flight run it, so both refuse the script before anything is written.
func (e *Executor) CheckVersionGates(prog *ast.Program) []linter.Violation {
	return versionGateViolations(e.newExecContext(context.Background()), prog)
}

func versionGateViolations(ctx *ExecContext, prog *ast.Program) []linter.Violation {
	var out []linter.Violation
	for i, stmt := range prog.Statements {
		if err := checkVersionGates(ctx, stmt); err != nil {
			msg, hint, _ := strings.Cut(err.Error(), "\n  hint: ")
			out = append(out, linter.Violation{
				RuleID:     "MDL-VERSION01",
				Severity:   linter.SeverityError,
				Message:    fmt.Sprintf("statement %d: %s", i+1, msg),
				Suggestion: hint,
			})
		}
	}
	return out
}
