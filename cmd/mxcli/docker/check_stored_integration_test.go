// SPDX-License-Identifier: Apache-2.0

//go:build integration

package docker

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCheck_ReportsStaleWidgetCE0463OfTheStoredProject is ako/mxcli#568 and
// #646 end to end, with real mx. A widget package that gained a property after
// its widgets were authored — what any widget upgrade does — leaves every
// stored instance stale: CE0463, which fails MxBuild and `mxcli run --local`,
// and which `mx update-widgets` clears. Until #646 the default docker check ran
// update-widgets on its copy first and passed such a project.
//
// Measured on 11.13.0 with Data grid 2 3.4.0 plus one boolean property: mx check
// on the stored project reports 11 CE0463, the old default 0 errors.
func TestCheck_ReportsStaleWidgetCE0463OfTheStoredProject(t *testing.T) {
	mxPath, err := ResolveMx("")
	if err != nil {
		t.Skipf("mx not resolvable: %v", err)
	}
	// Not t.TempDir(): create-project's template extraction hits
	// PathTooLongException under a long test name.
	dir, err := os.MkdirTemp("", "stl")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	scaffold := exec.Command(mxPath, "create-project")
	scaffold.Dir = dir
	PrepareMxCommand(scaffold)
	if out, err := scaffold.CombinedOutput(); err != nil {
		t.Skipf("mx create-project failed: %v\n%s", err, out)
	}
	mprPath := filepath.Join(dir, "App.mpr")
	mpk := filepath.Join(dir, "widgets", "com.mendix.widget.web.Datagrid.mpk")
	if _, err := os.Stat(mpk); err != nil {
		t.Skipf("the scaffolded project has no Data grid 2 package to make stale: %v", err)
	}
	addWidgetProperty(t, mpk, "Datagrid.xml")
	before := treeState(t, dir)

	check := func(opts CheckOptions) (string, error) {
		var out bytes.Buffer
		opts.ProjectPath, opts.Stdout, opts.Stderr = mprPath, &out, &out
		err := Check(opts)
		return out.String(), err
	}

	// Precondition, and the ground truth: the stored project has CE0463.
	stored, err := check(CheckOptions{SkipUpdateWidgets: true})
	n := len(ce0463Lines(stored))
	if n == 0 {
		t.Skipf("no CE0463 after making Data grid 2 stale (no instance in this template?):\n%s", stored)
	}
	if err == nil {
		t.Fatalf("--no-update-widgets passed a project with %d stored CE0463:\n%s", n, stored)
	}
	if strings.Contains(stored, staleCE0463Label) {
		t.Errorf("--no-update-widgets labelled the CE0463s; it must not run update-widgets:\n%s", stored)
	}

	// The default reports them, labels each as stale, and fails.
	got, err := check(CheckOptions{})
	if err == nil {
		t.Fatalf("docker check passed a project whose stored model has %d CE0463 (#568):\n%s", n, got)
	}
	if c := len(ce0463Lines(got)); c != n {
		t.Errorf("default reported %d CE0463, the stored project has %d:\n%s", c, n, got)
	}
	if !strings.Contains(got, staleCE0463Label) || strings.Contains(got, persistingCE0463Label) {
		t.Errorf("the CE0463s are not all labelled %q:\n%s", staleCE0463Label, got)
	}
	if !strings.Contains(got, "After update-widgets: 0 errors.") {
		t.Errorf("the re-check does not report the resynced copy clean:\n%s", got)
	}

	// --update-widgets is the old behaviour: it checks the normalised copy only.
	old, err := check(CheckOptions{UpdateWidgetsFirst: true})
	if err != nil {
		t.Errorf("--update-widgets failed on a project whose only errors a resync clears: %v\n%s", err, old)
	}

	if d := diffStates(before, treeState(t, dir)); len(d) > 0 {
		t.Errorf("docker check modified the project:\n  %v", d)
	}
}

// addWidgetProperty adds a boolean property to the widget definition xmlName in
// the package at mpkPath, the way a newer package version does: stored
// instances of the widget lack it from then on.
func addWidgetProperty(t *testing.T, mpkPath, xmlName string) {
	t.Helper()
	zr, err := zip.OpenReader(mpkPath)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	found := false
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if f.Name == xmlName {
			i := bytes.Index(data, []byte(`<property key="`))
			if i < 0 {
				t.Fatalf("%s has no property to insert before", xmlName)
			}
			probe := []byte(`<property key="mxcliStaleProbe" type="boolean" defaultValue="false"><caption>Stale probe</caption><description /></property>`)
			data = append(append(append([]byte{}, data[:i]...), probe...), data[i:]...)
			found = true
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: f.Modified})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	zr.Close()
	if !found {
		t.Fatalf("%s not found in %s", xmlName, mpkPath)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mpkPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
