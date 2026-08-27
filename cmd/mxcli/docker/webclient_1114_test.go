// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-ledger #146: mxcli could not start ANY Mendix 11.14 app.
//
// BuildWebClient exists because mxbuild's serve Deploy target used to write the
// client source and a rollup config but never run the bundler. 11.14 closed that
// gap upstream. Measured on a blank app, same build target, both engines of the
// toolchain untouched:
//
//	11.13.0   rollup.config.mjs PRESENT   dist/index.js ABSENT
//	11.14.0   rollup.config.mjs ABSENT    dist/index.js PRESENT
//
// The gate tested for the config, so on 11.14 it failed on the absence of a file
// whose purpose had been served — and both call sites are fatal.
package docker

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// deployWith builds a deployment dir with the given web/ contents. An empty
// value means "do not create this file".
func deployWith(t *testing.T, config, bundle string) string {
	t.Helper()
	dir := t.TempDir()
	webDir := filepath.Join(dir, "web")
	if err := os.MkdirAll(filepath.Join(webDir, "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(webDir, "rollup.config.mjs"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if bundle != "" {
		if err := os.WriteFile(filepath.Join(webDir, "dist", "index.js"), []byte(bundle), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestBuildWebClientSkipsWhenMxbuildAlreadyBundled(t *testing.T) {
	// The 11.14 shape: no config, bundle already written by mxbuild.
	var out bytes.Buffer
	err := BuildWebClient(WebClientOptions{
		DeployDir:   deployWith(t, "", "console.log(1)"),
		MxBuildPath: "/nonexistent/modeler/mxbuild", // must never be reached
		Stdout:      &out,
	})
	if err != nil {
		t.Fatalf("11.14 shape must succeed without running rollup, got: %v", err)
	}
	if !strings.Contains(out.String(), "already bundled") {
		t.Errorf("expected a skip line explaining why, got %q", out.String())
	}
	// The bogus MxBuildPath is the control: if the rollup step had run at all it
	// would have failed resolving the node tooling, so a nil error proves the
	// skip rather than a lucky build.
}

func TestBuildWebClientStillRefusesWhenNothingToServe(t *testing.T) {
	// Neither a config nor a bundle: the build produced no client, and that is
	// still an error — silently continuing would serve a blank page.
	err := BuildWebClient(WebClientOptions{
		DeployDir:   deployWith(t, "", ""),
		MxBuildPath: "/nonexistent/modeler/mxbuild",
	})
	if err == nil {
		t.Fatal("expected an error when there is neither a bundle nor a config")
	}
	// The message must name both halves, or a user on 11.14 reads it as the old
	// "run a serve Deploy build first" and looks in the wrong place.
	for _, want := range []string{"11.14", "rollup config", "deployment/"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q: %v", want, err)
		}
	}
}

func TestBuildWebClientStillRunsRollupWhenConfigPresent(t *testing.T) {
	// The 11.13 shape: a config and no bundle. The rollup step must still run —
	// proven here by it failing on the bogus tooling path rather than skipping.
	err := BuildWebClient(WebClientOptions{
		DeployDir:   deployWith(t, "export default {}", ""),
		MxBuildPath: "/nonexistent/modeler/mxbuild",
	})
	if err == nil {
		t.Fatal("expected the rollup step to run (and fail on the tooling path)")
	}
	if strings.Contains(err.Error(), "no rollup.config.mjs") {
		t.Errorf("must not report a missing config when one is present: %v", err)
	}
}
