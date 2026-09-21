// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The `action =` vocabulary is a hand-written table in mdl/types. That is the
// shape the catalog's own getMicroflowActionType comment warns about — a
// parallel switch silently buckets what it forgets, and eight action types went
// unlabelled that way — so it gets two guards, not one.
//
// This is the strong one: every alias is asserted against what the codec
// ACTUALLY writes for that action. Reading the Go type name is not enough, and
// that is not hypothetical — the first draft of the table did exactly that and
// got `create`, `commit`, `show page`, `close page`, `delete`, `rollback`,
// `aggregate` and `list operation` wrong, because those eight are the
// storage-name split in CLAUDE.md's table. A filter written with any of them
// would have matched nothing and reported a clean success.
//
// It lives in the backend package because mdl/types may not import codec or
// gen: they are the MPR backend's internal storage adapter (ADR-0005), so the
// vocabulary is plain data there and the pin is applied from the one layer
// allowed to see both currencies.
func TestActivityActionAliasesMatchWhatTheCodecWrites(t *testing.T) {
	// One representative semantic action per alias. Constructed as zero values:
	// the $Type does not depend on any field, and a fuller fixture would only
	// add ways for this to fail for an unrelated reason.
	byAlias := map[string]microflows.MicroflowAction{
		"log":                           &microflows.LogMessageAction{},
		"create":                        &microflows.CreateObjectAction{},
		"change":                        &microflows.ChangeObjectAction{},
		"delete":                        &microflows.DeleteObjectAction{},
		"commit":                        &microflows.CommitObjectsAction{},
		"rollback":                      &microflows.RollbackObjectAction{},
		"retrieve":                      &microflows.RetrieveAction{},
		"aggregate":                     &microflows.AggregateListAction{},
		"list operation":                &microflows.ListOperationAction{},
		"change list":                   &microflows.ChangeListAction{},
		"add to list":                   &microflows.ChangeListAction{},
		"remove from list":              &microflows.ChangeListAction{},
		"create list":                   &microflows.CreateListAction{},
		"change variable":               &microflows.ChangeVariableAction{},
		"create variable":               &microflows.CreateVariableAction{},
		"cast":                          &microflows.CastAction{},
		"call microflow":                &microflows.MicroflowCallAction{},
		"call nanoflow":                 &microflows.NanoflowCallAction{},
		"call java action":              &microflows.JavaActionCallAction{},
		"call javascript action":        &microflows.JavaScriptActionCallAction{},
		"call rest":                     &microflows.RestCallAction{},
		"rest operation":                &microflows.RestOperationCallAction{},
		"call external action":          &microflows.CallExternalAction{},
		"execute database query":        &microflows.ExecuteDatabaseQueryAction{},
		"import from mapping":           &microflows.ImportXmlAction{},
		"export to mapping":             &microflows.ExportXmlAction{},
		"transform json":                &microflows.TransformJsonAction{},
		"show page":                     &microflows.ShowPageAction{},
		"close page":                    &microflows.ClosePageAction{},
		"show home page":                &microflows.ShowHomePageAction{},
		"show message":                  &microflows.ShowMessageAction{},
		"download file":                 &microflows.DownloadFileAction{},
		"validation feedback":           &microflows.ValidationFeedbackAction{},
		"synchronize":                   &microflows.SynchronizeAction{},
		"call workflow":                 &microflows.WorkflowCallAction{},
		"set task outcome":              &microflows.SetTaskOutcomeAction{},
		"open user task":                &microflows.OpenUserTaskAction{},
		"notify workflow":               &microflows.NotifyWorkflowAction{},
		"open workflow":                 &microflows.OpenWorkflowAction{},
		"lock workflow":                 &microflows.LockWorkflowAction{},
		"unlock workflow":               &microflows.UnlockWorkflowAction{},
		"workflow operation":            &microflows.WorkflowOperationAction{},
		"get workflow data":             &microflows.GetWorkflowDataAction{},
		"get workflows":                 &microflows.GetWorkflowsAction{},
		"get workflow activity records": &microflows.GetWorkflowActivityRecordsAction{},
	}

	aliases := types.ActivityActionAliases()
	if len(aliases) == 0 {
		t.Fatal("the alias table is empty; every assertion below would be vacuous")
	}
	// Every alias must be covered here, or a new one could be added without
	// ever being measured — which is the whole failure this test exists for.
	for word := range aliases {
		if _, ok := byAlias[word]; !ok {
			t.Errorf("alias %q has no action in this test, so its storage name has "+
				"never been measured; add one", word)
		}
	}

	for word, act := range byAlias {
		want, ok := aliases[word]
		if !ok {
			t.Errorf("%q is not in the alias table", word)
			continue
		}
		g := microflowActionToGen(act)
		if g == nil {
			t.Errorf("alias %q: the codec has no gen type for %T, so the alias names "+
				"something mxcli never writes", word, act)
			continue
		}
		if got := g.TypeName(); got != want {
			t.Errorf("alias %q maps to %q but the codec writes %q — a filter using it "+
				"would match nothing and report a clean success", word, want, got)
		}
	}
}

// The second guard, and the cheaper one: every alias names a $Type the engine
// can decode. It catches a type renamed upstream even where the semantic action
// above still compiles.
func TestActivityActionAliasesAreDecodableTypes(t *testing.T) {
	registered := map[string]bool{}
	for _, name := range codec.DefaultRegistry.TypeNames() {
		registered[name] = true
	}
	// Positive control: an empty registry would make the loop pass vacuously.
	if !registered[types.ActivityLogActionType] {
		t.Fatalf("the codec registry does not know %s — gen is not linked in and "+
			"this test proves nothing", types.ActivityLogActionType)
	}
	for word, typ := range types.ActivityActionAliases() {
		if !registered[typ] {
			t.Errorf("alias %q names %q, which the codec cannot decode", word, typ)
		}
	}
}

// ResolveActivityAction accepts three spellings. The catalog label matters most
// of the three: it is how someone finds an action they cannot name.
func TestResolveActivityActionAcceptsAllThreeSpellings(t *testing.T) {
	known := map[string]bool{}
	for _, name := range codec.DefaultRegistry.TypeNames() {
		known[name] = true
	}

	for _, tc := range []struct{ in, want string }{
		{"log", types.ActivityLogActionType},                                           // alias
		{"LogMessageAction", types.ActivityLogActionType},                              // CATALOG.ACTIVITIES label
		{"logmessageaction", types.ActivityLogActionType},                              // case-insensitive
		{"Microflows$LogMessageAction", types.ActivityLogActionType},                   // full $Type, as bson dump prints
		{"  call   microflow ", "Microflows$MicroflowCallAction"},                      // whitespace collapsed
		{"ExecuteDatabaseQueryAction", "DatabaseConnector$ExecuteDatabaseQueryAction"}, // non-Microflows namespace
	} {
		got, ok := types.ResolveActivityAction(tc.in, known)
		if !ok || got != tc.want {
			t.Errorf("ResolveActivityAction(%q) = %q, %v; want %q, true", tc.in, got, ok, tc.want)
		}
	}

	// Control: an unknown word must be REFUSED, not defaulted. A resolver that
	// fell back to some type would make every bad filter match the same thing.
	for _, bad := range []string{"frobnicate", "LogMesageAction", "", "   "} {
		if got, ok := types.ResolveActivityAction(bad, known); ok {
			t.Errorf("ResolveActivityAction(%q) resolved to %q; it must be refused so "+
				"MDL088 can report it", bad, got)
		}
	}
}
