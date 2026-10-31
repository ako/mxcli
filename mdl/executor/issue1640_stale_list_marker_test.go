// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1640: a Data grid 2 that an older mxcli stored with
// SortBar.SortItems marker 3 kept it through every rewrite, where Studio Pro
// and today's writer both write 2:
//
//	Home_Web   mxcli=3 SP=2   <-- the old page
//	ZZ_A       mxcli=2 SP=2
//
// "drop page + re-run the same create or replace page MDL" gave 2, so only an
// existing document kept it — also when the rewrite changed the page for real
// (a dynamictext added). An unchanged pluggable widget passes its stored Type
// and Object through byte for byte, and the stored list with them.

const issue1640Setup = `create persistent entity MyFirstModule.Thing (Name: String(100));`

const issue1640Page = `create or modify page MyFirstModule.Things1640 (Title: 'Things', Layout: Atlas_Core.Atlas_Default) {
  container ctMain {
    datagrid dgThings (DataSource: database from MyFirstModule.Thing sort by Name asc) {
      column (Attribute: Name, Caption: 'Name')
    }
  }
};`

// listMarkers returns the marker of every stored list under key.
func listMarkers(doc bson.D, key string) []int32 {
	var got []int32
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			for _, e := range n {
				if a, ok := e.Value.(bson.A); ok && e.Key == key && len(a) > 0 {
					if m, ok := a[0].(int32); ok {
						got = append(got, m)
					}
				}
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	return got
}

// setListMarkers sets the marker of every list under key to m.
func setListMarkers(doc bson.D, key string, m int32) int {
	n := 0
	var walk func(any)
	walk = func(v any) {
		switch d := v.(type) {
		case bson.D:
			for i := range d {
				if a, ok := d[i].Value.(bson.A); ok && d[i].Key == key && len(a) > 0 {
					a[0] = m
					n++
				}
				walk(d[i].Value)
			}
		case bson.A:
			for _, e := range d {
				walk(e)
			}
		}
	}
	walk(doc)
	return n
}

// markerFixture is pageFixtureExecutor keeping the project path, so a test can
// change a stored unit behind the writer's back.
func markerFixture(t *testing.T) (*Executor, *bytes.Buffer, string) {
	t.Helper()
	var out bytes.Buffer
	path := projectFixture(t)
	exec := New(&out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	t.Cleanup(func() { exec.Close() })
	run(t, exec, "CONNECT LOCAL '"+visitor.QuoteString(path)+"'")
	out.Reset()
	return exec, &out, path
}

// plantStoredUnit replaces a unit's file with doc, as an older mxcli left it.
// Every write gives a registered list its marker, so the write path cannot
// plant a stale one; the file is written directly, then the project reopened.
func plantStoredUnit(t *testing.T, exec *Executor, path, unitID string, doc bson.D) {
	t.Helper()
	stored, err := exec.backend.GetRawUnitBytes(model.ID(unitID))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	var file string
	err = filepath.WalkDir(filepath.Join(filepath.Dir(path), "mprcontents"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".mxunit") {
			return err
		}
		if b, err := os.ReadFile(p); err == nil && bytes.Equal(b, stored) {
			file = p
		}
		return nil
	})
	if err != nil || file == "" {
		t.Fatalf("no file holds unit %s (%v)", unitID, err)
	}
	if err := os.WriteFile(file, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Execute(&ast.DisconnectStmt{}); err != nil {
		t.Fatalf("disconnect: %v", err)
	}
	if err := exec.Execute(&ast.ConnectStmt{Path: path}); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
}

// plantStaleSortItems stores the page's SortItems with marker 3.
func plantStaleSortItems(t *testing.T, exec *Executor, path, id string) {
	t.Helper()
	doc := storedUnitDoc(t, exec, id)
	if got := listMarkers(doc, "SortItems"); len(got) != 1 || got[0] != 2 {
		t.Fatalf("a fresh page stores SortItems %v, want [2]", got)
	}
	setListMarkers(doc, "SortItems", 3)
	plantStoredUnit(t, exec, path, id, doc)
	if got := listMarkers(storedUnitDoc(t, exec, id), "SortItems"); len(got) != 1 || got[0] != 3 {
		t.Fatalf("the planted page stores SortItems %v, want [3]", got)
	}
}

// The report's scenario: the page is changed for real, the grid is not.
func TestIssue1640_ARewriteGivesAStoredListStudioProsMarker(t *testing.T) {
	exec, _, path := markerFixture(t)
	run(t, exec, issue1640Setup)
	run(t, exec, issue1640Page)
	id := unitIDByName(t, exec, "page", "Things1640")
	plantStaleSortItems(t, exec, path, id)

	run(t, exec, strings.Replace(issue1640Page, "  container ctMain {", "  dynamictext txtNew (Content: 'new')\n  container ctMain {", 1))
	doc := storedUnitDoc(t, exec, id)
	namedDoc(t, doc, "txtNew") // the rewrite landed
	if got := listMarkers(doc, "SortItems"); len(got) != 1 || got[0] != 2 {
		t.Errorf("after the rewrite SortItems is stored %v, want [2] as Studio Pro writes", got)
	}
}

// Executing describe's output corrects it once, then is unchanged.
func TestIssue1640_ReExecutingDescribeCorrectsTheMarkerOnce(t *testing.T) {
	exec, out, path := markerFixture(t)
	run(t, exec, issue1640Setup)
	run(t, exec, issue1640Page)
	id := unitIDByName(t, exec, "page", "Things1640")
	plantStaleSortItems(t, exec, path, id)

	out.Reset()
	run(t, exec, "describe page MyFirstModule.Things1640")
	described := out.String()
	run(t, exec, described)
	if got := listMarkers(storedUnitDoc(t, exec, id), "SortItems"); len(got) != 1 || got[0] != 2 {
		t.Errorf("after describe -> exec SortItems is stored %v, want [2]", got)
	}
	out.Reset()
	run(t, exec, described)
	if !strings.Contains(out.String(), "Unchanged page") {
		t.Errorf("the second run wrote the page again:\n%s", out.String())
	}
}

// The comment on the issue: a populated nanoflow parameter list is marker 2
// in every Studio Pro document (78 of 78 call nanoflow actions and 21 of 21
// nanoflow data sources across ako/TestApp and PedApp); the writer used the
// child type's default, 3.
func TestIssue1640_NanoflowParameterListsAreWrittenAsStudioProWritesThem(t *testing.T) {
	exec, _ := pageFixtureExecutor(t)
	run(t, exec, issue1640Setup)
	run(t, exec, `create nanoflow MyFirstModule.NF_Do1640 ($Thing: MyFirstModule.Thing)
begin
end;`)
	run(t, exec, `create nanoflow MyFirstModule.NF_Get1640 ($Thing: MyFirstModule.Thing) returns MyFirstModule.Thing
begin
  return $Thing;
end;`)
	run(t, exec, `create or modify page MyFirstModule.Thing1640 (Title: 'Thing', Layout: Atlas_Core.Atlas_Default, Params: ($Thing: MyFirstModule.Thing)) {
  dataview dvThing (DataSource: $Thing) {
    actionbutton btnDo (Caption: 'Do', Action: call nanoflow MyFirstModule.NF_Do1640(Thing = $currentObject))
  }
  dataview dvGot (DataSource: nanoflow MyFirstModule.NF_Get1640(Thing = $Thing)) {
    textbox tbName (Attribute: Name)
  }
};`)
	doc := storedUnitDoc(t, exec, unitIDByName(t, exec, "page", "Thing1640"))
	for _, c := range []struct{ widget, owner string }{
		{"btnDo", "Forms$CallNanoflowClientAction"},
		{"dvGot", "Forms$NanoflowSource"},
	} {
		owner := firstOfType(t, namedDoc(t, doc, c.widget), c.owner)
		list, _ := bsonValue(owner, "ParameterMappings").(bson.A)
		if len(list) != 2 {
			t.Fatalf("%s: %s.ParameterMappings is %v, want a marker and one mapping", c.widget, c.owner, list)
		}
		if m, _ := list[0].(int32); m != 2 {
			t.Errorf("%s: %s.ParameterMappings is stored with marker %v, want 2 as Studio Pro writes", c.widget, c.owner, list[0])
		}
	}
}

// firstOfType returns the first element of $Type typ inside doc.
func firstOfType(t *testing.T, doc bson.D, typ string) bson.D {
	t.Helper()
	var found bson.D
	var walk func(any)
	walk = func(v any) {
		switch n := v.(type) {
		case bson.D:
			if found == nil && bsonString(n, "$Type") == typ {
				found = n
				return
			}
			for _, e := range n {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range n {
				walk(e)
			}
		}
	}
	walk(doc)
	if found == nil {
		t.Fatalf("no %s", typ)
	}
	return found
}
