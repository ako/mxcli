// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ako/mxcli#1799: a source catalog build described a pluggable widget by its
// generic form at random —
//
//	vegachart chartReplayWin ( spec: …
//
// on one run and
//
//	pluggablewidget 'f1.widget.web.vegachart.VegaChart' chartReplayWin ( spec: …
//
// on the next, same binary, same unchanged project.
//
// Every parallel describe built its own context and so loaded its own widget
// registry from disk, while the same loads rewrote `.mxcli/widgets/*.def.json`
// whenever one definition was unstamped. A describe that read a file another
// worker had just truncated got a registry missing the widget, and
// pluggableWidgetHeader fell back to the id form — correctly, for the registry
// it was given.

// A parallel describe must use the build's registry, not reload one: the test
// removes the definitions from disk after the build loaded them, so a reload
// finds none, which is the state a torn read left a worker in.
func TestParallelDescribeUsesTheBuildsWidgetRegistry(t *testing.T) {
	dir := t.TempDir()
	mprPath := filepath.Join(dir, "App.mpr")
	defsDir := filepath.Join(dir, ".mxcli", "widgets")
	if err := os.MkdirAll(defsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := `{"widgetId": "probe.widget.web.Probe", "mdlName": "probewidget", "generatorVersion": ` +
		jsonInt(WidgetDefGeneratorVersion) + `}`
	if err := os.WriteFile(filepath.Join(defsDir, "probewidget.def.json"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecContext{MprPath: mprPath}
	built := ctx.GetWidgetRegistry()
	if got := pluggableWidgetHeader(built, "probe.widget.web.Probe", "w1"); got != "probewidget w1" {
		t.Fatalf("build registry: header = %q, want %q (fixture does not resolve)", got, "probewidget w1")
	}

	if err := os.RemoveAll(defsDir); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	local := parallelDescribeContext(ctx, &buf, "")
	if got := pluggableWidgetHeader(local.GetWidgetRegistry(), "probe.widget.web.Probe", "w1"); got != "probewidget w1" {
		t.Errorf("parallel describe: header = %q, want %q — the describe reloaded the registry instead of using the build's", got, "probewidget w1")
	}
	if local.GetWidgetRegistry() != built {
		t.Errorf("parallel describe has its own registry; want the build's")
	}
}

// Refreshing the definitions must never let a reader see a torn file: another
// mxcli process, or the language server, reads them while a refresh writes.
func TestWidgetDefinitionRefreshNeverExposesATornFile(t *testing.T) {
	dir := t.TempDir()
	mprPath := filepath.Join(dir, "App.mpr")
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	const fixture = "../../testdata/expr-checker/widgets/Charts.mpk"
	src, err := os.ReadFile(fixture)
	if err != nil {
		t.Skipf("Charts.mpk fixture not available: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widgets", "Charts.mpk"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RefreshWidgetDefinitions(mprPath, false, nil); err != nil {
		t.Fatal(err)
	}
	defsDir := filepath.Join(dir, ".mxcli", "widgets")

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 30 {
			if _, err := RefreshWidgetDefinitions(mprPath, true, nil); err != nil {
				t.Errorf("refresh: %v", err)
				break
			}
		}
		close(stop)
	}()

	var torn []string
	for reading := true; reading; {
		select {
		case <-stop:
			reading = false
		default:
		}
		entries, _ := os.ReadDir(defsDir)
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".def.json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(defsDir, e.Name()))
			if err != nil {
				continue
			}
			var v map[string]any
			if json.Unmarshal(data, &v) != nil && len(torn) < 5 {
				torn = append(torn, e.Name()+" ("+jsonInt(len(data))+" bytes)")
			}
		}
	}
	wg.Wait()
	if len(torn) > 0 {
		t.Errorf("a reader saw a partly written definition during a refresh: %v", torn)
	}
}

func jsonInt(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
