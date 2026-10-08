// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
)

// The project-side existence checks behind `check -p` list documents to learn
// which names the project already holds. Each listing is a scan of the stored
// model — on a 140 MB MPR v1 project, ~0.1 s for a document kind and ~5 s for
// the module roles, which read every module's security — and CheckProjectConflicts
// used to issue all ~40 of them before looking at the script, although it only
// ever consults the kinds a plain CREATE names. A one-flow `create or modify`
// paid ~9 s for listings whose answers nothing read.
//
// These tests count the listings instead of timing them: a listing that is not
// needed is a listing that must not happen.

// countListings wraps every List*, GetProjectSecurity, GetModuleSecurity and
// GetProjectSettings function field of mb so each call is counted by name. A field the test left nil
// is filled with a function returning zero values, so an unconfigured listing
// is still counted rather than erroring out of sight.
type listingCounter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *listingCounter) names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for n := range c.calls {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// hierarchyListings are what resolving a container's qualified name reads; they
// are not a question about which names a kind holds.
var hierarchyListings = map[string]bool{"ListModules": true, "ListFolders": true, "ListUnits": true}

func countListings(mb *mock.MockBackend) *listingCounter {
	c := &listingCounter{calls: map[string]int{}}
	v := reflect.ValueOf(mb).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.Type.Kind() != reflect.Func || !strings.HasSuffix(f.Name, "Func") {
			continue
		}
		name := strings.TrimSuffix(f.Name, "Func")
		if !strings.HasPrefix(name, "List") && name != "GetProjectSecurity" &&
			name != "GetModuleSecurity" && name != "GetProjectSettings" {
			continue
		}
		if hierarchyListings[name] {
			continue
		}
		ft := f.Type
		inner := reflect.New(ft).Elem() // a copy: the field itself is replaced below
		inner.Set(v.Field(i))
		wrapped := reflect.MakeFunc(ft, func(args []reflect.Value) []reflect.Value {
			c.mu.Lock()
			c.calls[name]++
			c.mu.Unlock()
			if !inner.IsNil() {
				return inner.Call(args)
			}
			out := make([]reflect.Value, ft.NumOut())
			for j := range out {
				out[j] = reflect.Zero(ft.Out(j))
			}
			return out
		})
		v.Field(i).Set(wrapped)
	}
	return c
}

func TestCheckProjectConflicts_ListsOnlyWhatTheScriptAsks(t *testing.T) {
	cases := []struct {
		name   string
		script string
		// want is exactly the set of listings the check may issue.
		want []string
	}{
		{
			// `or modify` is never a same-kind conflict, and a regular
			// expression shares no name space with another kind: nothing to ask.
			name: "or-modify of a kind with no shared name space",
			script: `create or modify regular expression M.Pattern (
  Expression: '^[a-z]+$'
);`,
			want: nil,
		},
		{
			// `or modify` is still a cross-kind clash if a nanoflow or rule
			// holds the name (CE0122), so the flow name space is read — and
			// nothing else.
			name: "or-modify of one microflow",
			script: `create or modify microflow M.Probe ()
begin
  log info node 'Probe' 'hello';
end;`,
			want: []string{"ListMicroflows", "ListNanoflows", "ListRules"},
		},
		{
			name:   "plain create of a workflow",
			script: `create workflow M.BrandNewWF begin end workflow;`,
			want:   []string{"ListWorkflows"},
		},
		{
			// A drop of a kind no plain create follows needs no same-kind set.
			name: "drop then or-modify of a kind with no shared name space",
			script: `drop workflow M.ExistingWF;
create or modify regular expression M.Pattern (
  Expression: '^[a-z]+$'
);`,
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := setupProjectConflictCtx(t)
			counter := countListings(ctx.Backend.(*mock.MockBackend))
			if msgs := conflictErrorMessages(ctx, tc.script, t); len(msgs) != 0 {
				t.Fatalf("expected no conflicts, got %v", msgs)
			}
			got := counter.names()
			want := append([]string(nil), tc.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
				t.Errorf("listings issued:\n  got  %v\n  want %v", got, want)
			}
		})
	}
}

// The control: narrowing what is listed must not narrow what is found. A plain
// create of a stored name is still flagged, and a cross-kind clash in the flow
// name space is still found by a script that names only a microflow.
func TestCheckProjectConflicts_ListingNarrowedStillFlags(t *testing.T) {
	ctx, _ := setupProjectConflictCtx(t)
	counter := countListings(ctx.Backend.(*mock.MockBackend))
	assertHasConflict(t, ctx, `create workflow M.ExistingWF begin end workflow;`, "M.ExistingWF")
	if got := counter.names(); !reflect.DeepEqual(got, []string{"ListWorkflows"}) {
		t.Errorf("listings issued: got %v, want [ListWorkflows]", got)
	}

	ctx, _ = setupProjectConflictCtx(t)
	// M.ExistingRule is a rule; a microflow of the same name is CE0122.
	assertHasConflict(t, ctx, `create or modify microflow M.ExistingRule ()
begin
  log info node 'Probe' 'hello';
end;`, "M.ExistingRule")

	ctx, _ = setupProjectConflictCtx(t)
	// Drop, then a plain create of the same name: not a conflict, because the
	// drop removed it — which needs the dropped kind's set to be read.
	assertNoConflicts(t, ctx, `drop workflow M.ExistingWF;
create workflow M.ExistingWF begin end workflow;`)
}
