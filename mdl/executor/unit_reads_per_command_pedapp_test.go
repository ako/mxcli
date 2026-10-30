// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#1791: on MPR v2 nearly every command opened most of the project's
// unit files — `show modules` 4,511 of a 4,753-unit app's, `describe
// microflow` 2,996 — to read names (every folder for the hierarchy, every
// microflow for the name map and the by-name lookup) or to count documents.
// Each command runs in a fresh process, and an open costs a scan on Windows,
// so this was a floor under every call an agent makes. A fresh process now
// names what the persisted unit index has seen without opening it.

// openPedAppAgain connects a second executor to dir, as the next command's
// process would: nothing is shared but what is on disk.
func openPedAppAgain(t *testing.T, dir string) (*Executor, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	exec := New(out)
	exec.SetBackendFactory(func() backend.FullBackend { return modelsdkbackend.New() })
	if err := exec.Execute(&ast.ConnectStmt{Path: filepath.Join(dir, "PedApp.mpr")}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = exec.Execute(&ast.DisconnectStmt{}) })
	return exec, out
}

func TestFreshProcess_CommandsOpenOnlyWhatTheyShow(t *testing.T) {
	first, out, dir := openPedAppCopy(t)
	// The first process learns every unit and persists the index.
	if err := agreeExec(t, first, "show modules;"); err != nil {
		t.Fatalf("warm: %v\n%s", err, out.String())
	}
	hs, err := microflowHeaders(first.newExecContext(context.Background()))
	if err != nil || len(hs) < 10 {
		t.Fatalf("control: the fixture has %d microflows (%v); the bounds below would prove nothing", len(hs), err)
	}
	h, err := getHierarchy(first.newExecContext(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	target := h.GetQualifiedName(hs[0].ContainerID, hs[0].Name)

	// Measured on this fixture (375 units, 16 microflows): before the fix
	// `show modules` opened 196 files and the describe 113; after, 56 and 18.
	// The bounds sit between, so a regression to either listing fails.
	for _, c := range []struct {
		stmt string
		max  int64
	}{
		// The modules, their settings and domain models: no folder, no flow,
		// no page is opened to name or count it.
		{"show modules;", 80},
		// The microflow itself and the domain models its names resolve
		// against: not every microflow to find it by name.
		{"describe microflow " + target + ";", 30},
	} {
		exec, out := openPedAppAgain(t, dir)
		before := unitFileReads(t, exec)
		if err := agreeExec(t, exec, c.stmt); err != nil {
			t.Fatalf("%s: %v\n%s", c.stmt, err, out.String())
		}
		if reads := unitFileReads(t, exec) - before; reads > c.max {
			t.Errorf("%s opened %d unit files in a fresh process, want at most %d; the project has %d microflows", c.stmt, reads, c.max, len(hs))
		}
	}
}

// `show modules` counts documents from the unit types instead of decoding
// every document; the counts must be the ones the full listings give,
// including the System module's built-in enumerations and Java actions, which
// have no unit.
func TestShowModules_CountsAgreeWithTheListings(t *testing.T) {
	exec, out, _ := openPedAppCopy(t)
	exec.SetFormat(FormatJSON)
	out.Reset() // the connect message
	if err := agreeExec(t, exec, "show modules;"); err != nil {
		t.Fatalf("show modules: %v\n%s", err, out.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("parse: %v\n%s", err, out.String())
	}
	shown := map[string]map[string]int{}
	for _, r := range rows {
		m := map[string]int{}
		for k, v := range r {
			if n, ok := v.(float64); ok {
				m[k] = int(n)
			}
		}
		shown[r["Module"].(string)] = m
	}

	ctx := exec.newExecContext(context.Background())
	h, err := getHierarchy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b := ctx.Backend
	want := map[string]map[string]int{}
	add := func(col string, container model.ID) {
		mod := h.GetModuleName(h.FindModuleID(container))
		if want[mod] == nil {
			want[mod] = map[string]int{}
		}
		want[mod][col]++
	}
	enums, _ := b.ListEnumerations()
	for _, e := range enums {
		add("Enums", e.ContainerID)
	}
	pages, _ := b.ListPages()
	for _, p := range pages {
		add("Pages", p.ContainerID)
	}
	snippets, _ := b.ListSnippets()
	for _, s := range snippets {
		add("Snippets", s.ContainerID)
	}
	mfs, _ := b.ListMicroflows()
	for _, mf := range mfs {
		add("Microflows", mf.ContainerID)
	}
	nfs, _ := b.ListNanoflows()
	for _, nf := range nfs {
		add("Nanoflows", nf.ContainerID)
	}
	constants, _ := b.ListConstants()
	for _, c := range constants {
		add("Constants", c.ContainerID)
	}
	jas, _ := b.ListJavaActions()
	for _, ja := range jas {
		add("JavaActions", ja.ContainerID)
	}
	if want["System"]["Enums"] == 0 || want["System"]["JavaActions"] == 0 {
		t.Fatalf("control: the listings give System no built-ins; the System case is not exercised")
	}

	checked := 0
	for mod, cols := range want {
		for col, n := range cols {
			if shown[mod][col] != n {
				t.Errorf("show modules: %s %s = %d, the listing has %d", mod, col, shown[mod][col], n)
			}
			checked++
		}
	}
	if checked < 10 {
		t.Fatalf("control: only %d module/column counts compared", checked)
	}
}
