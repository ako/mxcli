// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A project with Settings > Web UI > OptimizedClient = No deploys Mendix's
// classic (Dojo) client, which has no bundling step at all: no
// web/rollup.config.mjs and no web/dist. The gate assumed one of those two must
// exist and failed the run outright, claiming "the build did not produce a
// client" about a deployment that had one. (issue #1123)

// classicDeployment builds a deployment directory shaped like a real
// OptimizedClient=No build: web/index.html from mxbuild, no rollup config, no
// dist, and the React client parked in react-web/.
func classicDeployment(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	webDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(filepath.Join(dir, "react-web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("testdata", "webclient", "classic-index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	// The React client's rollup config lives in the PARKED directory, not in
	// web/. Planting it proves the plan reads web/ and is not fooled by it.
	if err := os.WriteFile(filepath.Join(dir, "react-web", "rollup.config.mjs"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// optimizedDeployment is the control: the same app with OptimizedClient = Yes,
// where web/ holds the React client and its rollup config.
func optimizedDeployment(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	webDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(webDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(filepath.Join("testdata", "webclient", "optimized-index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "rollup.config.mjs"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestPlanWebClientReadsRealEntryPoints pins the decision against the actual
// index.html files mxbuild writes for both settings of the same 11.12.2 app —
// the classic one is the deployment the reporter had.
func TestPlanWebClientReadsRealEntryPoints(t *testing.T) {
	if got := planWebClient(classicDeployment(t)); got != webClientClassic {
		t.Errorf("a deployment whose web/index.html loads mxclientsystem is the classic client, got %v", got)
	}
	if got := planWebClient(optimizedDeployment(t)); got != webClientRollup {
		t.Errorf("a React deployment with a rollup config needs the rollup step, got %v", got)
	}
}

// TestPlanWebClientPrebuiltAndMissing keeps the two pre-existing outcomes intact:
// Mendix 11.14+ writes web/dist itself, and a deployment with none of the three
// shapes is still the genuine "no client" failure this gate exists for.
func TestPlanWebClientPrebuiltAndMissing(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "web", "dist", "index.js"), []byte("console.log(1)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := planWebClient(dir); got != webClientPrebuilt {
		t.Errorf("11.14+ writes the bundle itself, got %v", got)
	}

	if got := planWebClient(t.TempDir()); got != webClientMissing {
		t.Errorf("an empty deployment really has no client, got %v", got)
	}
}

// TestBuildWebClientSkipsClassic is the reporter's failure. With no rollup config
// and no dist, BuildWebClient errored — so `mxcli run --local` exited 1 after a
// ~154s cold build, saying the build produced no client about a deployment whose
// client was sitting in the same directory.
func TestBuildWebClientSkipsClassic(t *testing.T) {
	var out bytes.Buffer
	// No MxBuildPath: reaching the node tooling would be a failure in itself,
	// since there is nothing for rollup to do.
	if err := BuildWebClient(WebClientOptions{DeployDir: classicDeployment(t), Stdout: &out}); err != nil {
		t.Fatalf("a classic-client deployment must not be treated as a failed build: %v", err)
	}
	if !strings.Contains(strings.ToLower(out.String()), "classic") {
		t.Errorf("the skip should say why it did nothing, got: %q", out.String())
	}
}

// TestBuildWebClientStillRejectsAClientlessDeployment is the control for the one
// above: the gate must keep failing where it was right. Without it, "skip when
// there is nothing to bundle" degenerates into "never report a broken build".
func TestBuildWebClientStillRejectsAClientlessDeployment(t *testing.T) {
	err := BuildWebClient(WebClientOptions{DeployDir: t.TempDir(), Stdout: io.Discard})
	if err == nil {
		t.Fatal("a deployment with no client at all must still be reported")
	}
	if !strings.Contains(err.Error(), "did not produce a client") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestStartWebClientWatchSkipsClassic — under --watch the second copy of the gate
// applied, so `run --local --watch` failed on a classic app even when the boot
// path was fixed. A nil watcher is the existing "no bundler to keep hot" signal
// and every method on it is nil-safe.
func TestStartWebClientWatchSkipsClassic(t *testing.T) {
	var out bytes.Buffer
	w, err := StartWebClientWatch(WebClientOptions{DeployDir: classicDeployment(t), Stdout: &out})
	if err != nil {
		t.Fatalf("classic deployment must not fail the watcher: %v", err)
	}
	if w != nil {
		t.Fatal("there is no incremental bundler for the classic client")
	}
	w.Stop() // nil-safe, as the loop relies on
}

// TestEnsureWebClientBundleSkipsClassic — the post-boot guard re-bundles when
// web/dist is missing, which for a classic app is always, so it would have
// re-run the bundler on every boot.
func TestEnsureWebClientBundleSkipsClassic(t *testing.T) {
	called := false
	rebuilt, err := ensureWebClientBundle(classicDeployment(t), io.Discard, func() error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("classic deployment: %v", err)
	}
	if called || rebuilt {
		t.Error("nothing to re-bundle: the classic client has no bundle")
	}
}

// TestEnsureClientServedSkipsClassic — the per-apply guard asserts /dist/index.js
// is being SERVED, which a classic app never serves. Under --watch this fails on
// every applied change, so fixing only the boot path would have moved the failure
// rather than removed it. appURL is deliberately unreachable: reaching the probe
// at all is the defect.
func TestEnsureClientServedSkipsClassic(t *testing.T) {
	if err := ensureClientServed(classicDeployment(t), "http://127.0.0.1:1", "", io.Discard); err != nil {
		t.Fatalf("classic deployment must not be probed for a bundle it has no concept of: %v", err)
	}
}
