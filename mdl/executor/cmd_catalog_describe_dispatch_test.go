// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
)

// funcName returns the unqualified name of a describe function, so the dispatch
// table can be asserted by identity rather than by behaviour (which would need a
// project). reflect+runtime is the only way to compare func values in Go.
func funcName(fn describeDocFunc) string {
	if fn == nil {
		return ""
	}
	full := runtime.FuncForPC(reflect.ValueOf(fn).Pointer()).Name()
	if i := strings.LastIndex(full, "."); i >= 0 {
		return full[i+1:]
	}
	return full
}

// The catalog collects one describe per document into CATALOG.SOURCE, and the
// executor turns each into MDL. Those two lists are maintained in different
// packages, and #912 is what happens when they drift: the collector was taught
// to enumerate nanoflows and rules, the dispatch was not, and every one of those
// describes failed into a swallowed error. The reporter's project had 13
// nanoflows and a rule; CATALOG.SOURCE had 0 rows for both, and `search` could
// not find them.
//
// Asserting the *identity* of each mapping (not merely that one exists) is what
// makes this test detect the nanoflow half: "nanoflow" was dispatched to
// describeMicroflow, which searches ListMicroflows() only and returns NotFound
// for every nanoflow. A presence-only check would have passed.
func TestDescribeDispatchCoversEverySourceObjectType(t *testing.T) {
	want := map[string]string{
		catalog.SourceEntity:        "describeEntity",
		catalog.SourceMicroflow:     "describeMicroflow",
		catalog.SourceNanoflow:      "describeNanoflow",
		catalog.SourceRule:          "describeRule",
		catalog.SourcePage:          "describePage",
		catalog.SourceSnippet:       "describeSnippet",
		catalog.SourceEnumeration:   "describeEnumeration",
		catalog.SourceWorkflow:      "describeWorkflow",
		catalog.SourceJsonStructure: "describeJsonStructure",
		catalog.SourceImportMapping: "describeImportMapping",
		catalog.SourceExportMapping: "describeExportMapping",
	}

	// Every type the catalog collects must be dispatchable. This is the
	// invariant #912 broke; it is asserted against the exported list so adding
	// a collector without a dispatch case fails here rather than silently
	// producing zero rows.
	for _, objType := range catalog.SourceObjectTypes {
		wantFn, listed := want[objType]
		if !listed {
			t.Errorf("catalog.SourceObjectTypes has %q but this test does not pin its describe function — add it here and to the dispatch", objType)
			continue
		}
		got := funcName(describeDispatch(objType))
		if got == "" {
			t.Errorf("%s: no describe function — the catalog collects this type, so every document of it would be dropped from CATALOG.SOURCE", objType)
			continue
		}
		if got != wantFn {
			t.Errorf("%s dispatches to %s, want %s", objType, got, wantFn)
		}
	}

	// The reverse direction: a pinned mapping that is no longer collected means
	// this test has gone stale.
	for objType := range want {
		if !listsObjectType(catalog.SourceObjectTypes, objType) {
			t.Errorf("this test pins %q but catalog.SourceObjectTypes no longer lists it", objType)
		}
	}
}

func TestDescribeDispatchRejectsUnknownTypes(t *testing.T) {
	for _, objType := range []string{"", "LAYOUT", "not-a-type"} {
		if fn := describeDispatch(objType); fn != nil {
			t.Errorf("describeDispatch(%q) = %s, want nil", objType, funcName(fn))
		}
	}
}

// The dispatch is case-insensitive, because the collector emits upper-case type
// names and the original switch lower-cased before matching.
func TestDescribeDispatchIsCaseInsensitive(t *testing.T) {
	for _, objType := range []string{"nanoflow", "NANOFLOW", "NanoFlow"} {
		if got := funcName(describeDispatch(objType)); got != "describeNanoflow" {
			t.Errorf("describeDispatch(%q) = %q, want describeNanoflow", objType, got)
		}
	}
}

func listsObjectType(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
