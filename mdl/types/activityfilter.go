// SPDX-License-Identifier: Apache-2.0

package types

import (
	"fmt"
	"sort"
	"strings"
)

// ActivityFilter selects the activities an `ALTER MICROFLOW … DISABLE|ENABLE
// ACTIVITIES WHERE …` statement acts on.
//
// Conditions are ANDed. There is no OR and no nesting: `IN (a, b)` covers the
// case an OR would be written for ("debug or trace"), and a boolean expression
// tree would need precedence rules that nothing else in MDL has. A filter that
// needs more than this is two statements.
//
// The type lives in mdl/types because `mxcli check` and the executor must apply
// the IDENTICAL rules — check reports the problem with a source position, exec
// refuses the write — and two resolvers in two currencies is how a resolver
// drifts (CLAUDE.md).
type ActivityFilter struct {
	Conditions []ActivityCondition
}

// ActivityCondition is one `column <op> value` term.
type ActivityCondition struct {
	Column string   // lower-cased; one of ActivityColumns
	Negate bool     // `!=` / `NOT IN`
	Like   bool     // `LIKE 'pattern'` — text columns only
	Values []string // one value for `=`/`!=`/`LIKE`, several for `IN (…)`
}

// The columns a WHERE may name. Deliberately short: each one is a property
// mxcli can read off a stored activity without interpreting it.
const (
	// ActivityColAction is the activity's action kind — `log`, `commit`,
	// `call microflow`, … or the Mendix storage name (`LogMessageAction`),
	// which is what CATALOG.ACTIVITIES.ActionType prints.
	ActivityColAction = "action"
	// ActivityColLevel is a log activity's level. Meaningful on `log` only;
	// on any other activity it matches nothing, which is why a filter naming
	// it alongside a non-log action is refused rather than run.
	ActivityColLevel = "level"
	// ActivityColCaption is the activity's caption as DESCRIBE shows it.
	ActivityColCaption = "caption"
	// ActivityColDisabled is the activity's CURRENT state, so a statement can
	// be written to act only on what it would change.
	ActivityColDisabled = "disabled"
)

// ActivityColumns is the accepted set, in the order the error message lists them.
var ActivityColumns = []string{
	ActivityColAction, ActivityColLevel, ActivityColCaption, ActivityColDisabled,
}

// ActivityLogLevels are Mendix's Microflows$LogLevel members, lower-cased.
// Validated against generated/metamodel's MicroflowsLogLevel by
// TestActivityLogLevelsMatchTheMetamodel.
var ActivityLogLevels = []string{"critical", "error", "warning", "info", "debug", "trace"}

// activityActionAliases maps the word a script writes to the Mendix action's
// storage name without the `Microflows$` prefix — the same label
// CATALOG.ACTIVITIES.ActionType carries, so a value found by querying the
// catalog can be pasted into a filter unchanged.
//
// Hand-written, and that is the risk: the catalog's own comment records eight
// action types that a parallel switch silently bucketed under a generic label.
// Two things keep this from repeating. An alias that names a type the codec
// does not register fails TestActivityActionAliasesAreRegisteredTypes, so a
// renamed type breaks the build rather than a user's script. And a word that is
// neither an alias nor a registered type is an ERROR (MDL088), never a filter
// that quietly matches nothing — which is the failure this table could
// otherwise cause.
var activityActionAliases = map[string]string{
	// Every value here was MEASURED, not read off a type name: the statement
	// was built and its action run through microflowActionToGen, and the
	// storage name printed. Nine of them differ from the SDK's qualified name
	// (CLAUDE.md's storage-name table) and a first pass got three more wrong by
	// reading the Go type — `create` stores CreateChangeAction, `commit` stores
	// CommitAction, `show page` stores ShowFormAction.
	"log":                           "Microflows$LogMessageAction",
	"create":                        "Microflows$CreateChangeAction",
	"change":                        "Microflows$ChangeAction",
	"delete":                        "Microflows$DeleteAction",
	"commit":                        "Microflows$CommitAction",
	"rollback":                      "Microflows$RollbackAction",
	"retrieve":                      "Microflows$RetrieveAction",
	"aggregate":                     "Microflows$AggregateAction",
	"list operation":                "Microflows$ListOperationsAction",
	"change list":                   "Microflows$ChangeListAction",
	"add to list":                   "Microflows$ChangeListAction",
	"remove from list":              "Microflows$ChangeListAction",
	"create list":                   "Microflows$CreateListAction",
	"change variable":               "Microflows$ChangeVariableAction",
	"create variable":               "Microflows$CreateVariableAction",
	"cast":                          "Microflows$CastAction",
	"call microflow":                "Microflows$MicroflowCallAction",
	"call nanoflow":                 "Microflows$NanoflowCallAction",
	"call java action":              "Microflows$JavaActionCallAction",
	"call javascript action":        "Microflows$JavaScriptActionCallAction",
	"call rest":                     "Microflows$RestCallAction",
	"rest operation":                "Microflows$RestOperationCallAction",
	"call external action":          "Microflows$CallExternalAction",
	"execute database query":        "DatabaseConnector$ExecuteDatabaseQueryAction",
	"import from mapping":           "Microflows$ImportXmlAction",
	"export to mapping":             "Microflows$ExportXmlAction",
	"transform json":                "Microflows$TransformJsonAction",
	"show page":                     "Microflows$ShowFormAction",
	"close page":                    "Microflows$CloseFormAction",
	"show home page":                "Microflows$ShowHomePageAction",
	"show message":                  "Microflows$ShowMessageAction",
	"download file":                 "Microflows$DownloadFileAction",
	"validation feedback":           "Microflows$ValidationFeedbackAction",
	"synchronize":                   "Microflows$SynchronizeAction",
	"call workflow":                 "Microflows$WorkflowCallAction",
	"set task outcome":              "Microflows$SetTaskOutcomeAction",
	"open user task":                "Microflows$OpenUserTaskAction",
	"notify workflow":               "Microflows$NotifyWorkflowAction",
	"open workflow":                 "Microflows$OpenWorkflowAction",
	"lock workflow":                 "Microflows$LockWorkflowAction",
	"unlock workflow":               "Microflows$UnlockWorkflowAction",
	"workflow operation":            "Microflows$WorkflowOperationAction",
	"get workflow data":             "Microflows$GetWorkflowDataAction",
	"get workflows":                 "Microflows$GetWorkflowsAction",
	"get workflow activity records": "Microflows$GetWorkflowActivityRecordsAction",
}

// ActivityActionAliases returns the alias table. Exported for the drift test
// and for the error message that lists what a filter may name.
func ActivityActionAliases() map[string]string {
	out := make(map[string]string, len(activityActionAliases))
	for k, v := range activityActionAliases {
		out[k] = v
	}
	return out
}

// ResolveActivityAction turns the word a script wrote into the Mendix `$Type`
// a stored activity carries, and reports whether it resolved.
//
// Three spellings resolve. An alias (`log`, `call javascript action`) — the MDL
// keyword for the statement. A bare storage label (`LogMessageAction`), which
// is what `select ActionType from CATALOG.ACTIVITIES` prints, so a value found
// by querying the catalog pastes in unchanged. And a full `$Type`
// (`Microflows$LogMessageAction`), which is what `mxcli bson dump` shows.
//
// knownTypes is the set of full `$Type` strings the engine can decode; pass nil
// to accept only aliases, which is what `check` does without a project.
//
// Matching is case-insensitive and collapses internal whitespace, so
// `Call  Microflow` and `call microflow` are the same word.
func ResolveActivityAction(word string, knownTypes map[string]bool) (string, bool) {
	w := strings.ToLower(strings.Join(strings.Fields(word), " "))
	if w == "" {
		return "", false
	}
	if typ, ok := activityActionAliases[w]; ok {
		return typ, true
	}
	for typ := range knownTypes {
		// Full $Type, or the label after the namespace separator. An action can
		// live outside the Microflows namespace — `execute database query` is
		// DatabaseConnector$ExecuteDatabaseQueryAction — so the namespace is
		// split off rather than a fixed prefix being trimmed.
		if strings.EqualFold(typ, w) {
			return typ, true
		}
		if i := strings.IndexByte(typ, '$'); i >= 0 && strings.EqualFold(typ[i+1:], w) {
			return typ, true
		}
	}
	return "", false
}

// ActivityLogActionType is the `$Type` a log activity's action carries. Named
// because the `level` column is a property of this action and of no other, and
// two checks need to say so.
const ActivityLogActionType = "Microflows$LogMessageAction"

// CheckActivityFilter returns one message per problem with the filter, empty
// when it is sound. Called by `mxcli check` and by the executor, so the two
// cannot disagree about what a filter means.
//
// knownTypes may be nil — `check` without a project has no registry to consult,
// and an action word is then validated against the aliases alone. A storage
// label is accepted there rather than refused, because refusing it would make
// `check` reject a script `exec` accepts.
func CheckActivityFilter(f ActivityFilter, knownTypes map[string]bool) []string {
	var problems []string
	if len(f.Conditions) == 0 {
		return []string{
			"the WHERE clause selects nothing — a filter with no condition would " +
				"act on every activity in scope, which is never what a script means to say. " +
				"Name at least one of " + strings.Join(ActivityColumns, ", "),
		}
	}
	seenAction, seenActionWord := "", ""
	usesLevel := false

	for _, c := range f.Conditions {
		col := strings.ToLower(c.Column)
		if !containsString(ActivityColumns, col) {
			problems = append(problems, fmt.Sprintf(
				"unknown column `%s` — a WHERE on activities may name %s",
				c.Column, strings.Join(ActivityColumns, ", ")))
			continue
		}
		if c.Like && col != ActivityColCaption {
			problems = append(problems, fmt.Sprintf(
				"`LIKE` on `%s` is not supported — it matches text, and only `caption` is text. "+
					"Use `=` or `IN (…)` for `%s`", col, col))
			continue
		}
		switch col {
		case ActivityColAction:
			for _, v := range c.Values {
				label, ok := ResolveActivityAction(v, knownTypes)
				if !ok {
					problems = append(problems, fmt.Sprintf(
						"unknown action `%s` — it would match no activity at all, so the "+
							"statement would report a success that changed nothing.%s",
						v, suggestActions(v)))
					continue
				}
				if !c.Negate && seenAction == "" {
					seenAction, seenActionWord = label, v
				}
			}
		case ActivityColLevel:
			usesLevel = true
			for _, v := range c.Values {
				if !containsString(ActivityLogLevels, strings.ToLower(v)) {
					problems = append(problems, fmt.Sprintf(
						"unknown log level `%s` — Mendix's levels are %s",
						v, strings.Join(ActivityLogLevels, ", ")))
				}
			}
		case ActivityColDisabled:
			for _, v := range c.Values {
				if lv := strings.ToLower(v); lv != "true" && lv != "false" {
					problems = append(problems, fmt.Sprintf(
						"`disabled` takes true or false, not `%s`", v))
				}
			}
		}
	}

	// `level` is a property of a log activity and of nothing else, so pairing it
	// with a different action selects the empty set. Reported rather than run:
	// the statement would print "0 activities" and look like the model had none
	// to change, which is indistinguishable from the filter being wrong.
	if usesLevel && seenAction != "" && seenAction != ActivityLogActionType {
		problems = append(problems, fmt.Sprintf(
			"`level` is a property of a log activity, and this filter also requires "+
				"`action = %s` — together they match nothing. Drop one of the two",
			seenActionWord))
	}
	return problems
}

// suggestActions offers the closest accepted words for an unknown action.
func suggestActions(word string) string {
	w := strings.ToLower(strings.Join(strings.Fields(word), " "))
	var near []string
	for alias := range activityActionAliases {
		if strings.Contains(alias, w) || strings.Contains(w, alias) {
			near = append(near, alias)
		}
	}
	sort.Strings(near)
	if len(near) > 0 {
		if len(near) > 6 {
			near = near[:6]
		}
		return " Did you mean " + strings.Join(near, ", ") + "?"
	}
	return " Accepted words are the MDL activity keywords (log, commit, retrieve, " +
		"call microflow, call javascript action, …) and the Mendix storage names that " +
		"`select ActionType from CATALOG.ACTIVITIES` prints."
}
