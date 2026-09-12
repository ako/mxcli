// SPDX-License-Identifier: Apache-2.0

// Can a default-engine run still hit errUnimplemented?
//
// That is the question standing between the modelsdk engine and dropping
// `legacy` from nightly, and counting unimplemented methods answers it wrongly:
// 19 of FullBackend's 276 methods were not declared on *Backend, but 17 of them
// are interface surface nothing calls through a backend value, so their stub
// could never fire. Measured with scripts/backend-reachability.sh, which removes
// one method from its interface at a time and rebuilds — grep cannot tell
// `b.reader.X()` inside the MPR backend from `ctx.Backend.X()` in the executor.
//
// This test does not repeat that measurement (a build per method takes minutes).
// It pins its OUTPUT: the set of methods *Backend leaves to the stub must be
// exactly the set measured unreachable. A new stub, or a rename that drops an
// implementation, fails here and is a prompt to re-run the script rather than to
// extend the list on faith.
package modelsdkbackend

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend"
)

// unreachableUnimplemented are the FullBackend methods *Backend does not
// implement, each measured to have no caller through a backend value.
//
// The reason column is what the probe found, not a guess: for all but one it is
// that only the MPR backend's own delegation mentions the name, plus callers
// that hold a concrete *mpr.Reader / *mpr.Writer (the api/ package, examples/,
// and cmd/mxcli commands that open a reader directly) — none of which route
// through this engine.
var unreachableUnimplemented = map[string]string{
	"AddAttribute":             "api/ and examples/ call it on the sdk writer; ALTER ENTITY goes through the mutator",
	"UpdateAttribute":          "same as AddAttribute",
	"GetDomainModelByID":       "the MPR and MCP backends call it on their own reader",
	"ExportJSON":               "examples/read_project calls it on the sdk reader",
	"FindCustomWidgetType":     "cmd/mxcli/cmd_extract_templates.go holds a concrete reader",
	"FindAllCustomWidgetTypes": "reached only via the reader, inside modelsdk/mpr itself",
	"GetProjectRootID":         "callers hold a reader; this package uses b.reader.GetProjectRootID directly",
	"GetUnitTypes":             "no caller at all outside the MPR delegation",
	"ListAllUnitIDs":           "cmd/mxcli/diag.go holds a concrete reader; infrastructure_write.go uses b.reader",
	"ListRawUnits":             "the bson dump/discover/describe commands hold a concrete reader",
	"ListNavigationDocuments":  "the MCP backend and sdk/mpr call it on their own reader",
	"GetWorkflow":              "the MCP backend calls it on its own reader",
	"UpdateLayout":             "ALTER LAYOUT goes through the page mutator, not this method",
	"SerializeWidget":          "the child serializer is a separate type (codecChildSerializer), not Backend",
	"SerializeClientAction":    "same as SerializeWidget",
	"SerializeDataSource":      "no caller at all, on any type",
}

func TestNoReachableUnimplementedBackendMethods(t *testing.T) {
	declared := methodsDeclaredOnBackend(t)
	iface := reflect.TypeOf((*backend.FullBackend)(nil)).Elem()

	var missing []string
	for i := 0; i < iface.NumMethod(); i++ {
		if name := iface.Method(i).Name; !declared[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 && len(unreachableUnimplemented) > 0 {
		t.Fatalf("*Backend now implements everything, but %d methods are still listed as "+
			"unreachable — delete unreachableUnimplemented rather than leave it stale",
			len(unreachableUnimplemented))
	}
	sort.Strings(missing)

	var unexpected []string
	for _, name := range missing {
		if unreachableUnimplemented[name] == "" {
			unexpected = append(unexpected, name)
		}
	}
	if len(unexpected) > 0 {
		t.Errorf("these FullBackend methods fall through to the errUnimplemented stub and are not\n"+
			"recorded as unreachable:\n  %s\n"+
			"Run `scripts/backend-reachability.sh %s`. If it says LIVE, implement the method —\n"+
			"a default-engine run can reach it and will be told to rerun on an engine that is\n"+
			"being retired. If it says DEAD, add it to unreachableUnimplemented with that reason.",
			strings.Join(unexpected, "\n  "), strings.Join(unexpected, " "))
	}

	present := map[string]bool{}
	for _, name := range missing {
		present[name] = true
	}
	var stale []string
	for name := range unreachableUnimplemented {
		if !present[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Errorf("unreachableUnimplemented lists %v, which *Backend now implements — "+
			"strike them off, or the list stops meaning anything", stale)
	}
}

// TestTheTwoReachableMethodsAreImplemented is the control for the list above.
//
// Without it the test would pass just as well against a build where nothing is
// implemented and everything is listed as unreachable. These two are the ones
// the probe found LIVE, called from mdl/executor/cmd_microflows_builder.go, and
// they must never go back on the list.
func TestTheTwoReachableMethodsAreImplemented(t *testing.T) {
	declared := methodsDeclaredOnBackend(t)
	for _, name := range []string{"GetRawUnitByName", "ParseMicroflowBSON"} {
		if !declared[name] {
			t.Errorf("%s is reachable from the executor but not implemented — a default-engine "+
				"run hits errUnimplemented and falls back to an O(n) module walk", name)
		}
		if unreachableUnimplemented[name] != "" {
			t.Errorf("%s is listed as unreachable; the probe found four call sites in "+
				"mdl/executor/cmd_microflows_builder.go", name)
		}
	}
}
