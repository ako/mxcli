// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The message below is the real one, captured from mxbuild 11.14.0 serving a
// blank app: a clean deployment, one `ALTER PAGE … SET Content` edit, and the
// second Deploy build against the same serve process.
const real1114IncrementalFailure = "One or more errors occurred. " +
	"(Could not find a part of the path '/w/W1114/deployment/web/layouts/Atlas_Core.Atlas_TopBar.js'.) " +
	"(Could not find a part of the path '/w/W1114/deployment/web/pages/MyFirstModule.Home_Web.js'.) " +
	"(Could not find a part of the path '/w/W1114/deployment/web/layouts/Atlas_Core.Atlas_Default.js'.) " +
	"(Could not find a part of the path '/w/W1114/deployment/web/layouts/Atlas_Core.Atlas_SideBar.js'.)"

// deploy1114 is the shape a 11.14 cold build leaves: web/dist and nothing else.
func deploy1114(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLegacyClientBuildHintExplainsThe1114IncrementalFailure(t *testing.T) {
	hint := legacyClientBuildHint(deploy1114(t), real1114IncrementalFailure)
	if hint == "" {
		t.Fatal("the real 11.14 incremental failure must be explained")
	}
	// The three things a user acts on: it is not their model, both directories
	// are named, and there is a command that works today.
	for _, want := range []string{"not a problem with your model", "web/pages/", "web/layouts/", "run --local --screenshot"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint missing %q:\n%s", want, hint)
		}
	}
	// Creating the directories is the obvious wrong move, so the hint must say
	// so — measured, it advances the same build to a widget-export failure.
	if !strings.Contains(hint, "does not help") {
		t.Errorf("hint must warn that creating the directories is not a workaround:\n%s", hint)
	}
}

func TestLegacyClientBuildHintStaysSilentOnOtherFailures(t *testing.T) {
	dir := deploy1114(t)
	// Every one of these is a real failure with a different cause, and a hint
	// on any of them would send the user chasing a Mendix-version story that
	// has nothing to do with their build.
	for name, msg := range map[string]string{
		"model errors":  "1 error occurred. CE0142: The microflow must return a Boolean.",
		"scss":          "Expected expression. _custom.scss 180:35",
		"widget export": "Deployment failed during export of pluggable widgets.",
		"other path":    "Could not find a part of the path '/w/W1114/userlib/missing.jar'.",
		"empty":         "",
	} {
		if hint := legacyClientBuildHint(dir, msg); hint != "" {
			t.Errorf("%s must not be explained as the 11.14 client shape:\n%s", name, hint)
		}
	}
}

func TestLegacyClientBuildHintStaysSilentWhenTheDirectoriesExist(t *testing.T) {
	// The pre-11.14 shape: the cold build made these, so a missing file inside
	// one is a real failure and not this. Without the stat the hint would fire
	// on any 11.13 build that lost a page file, which is a different bug.
	dir := deploy1114(t)
	for _, d := range legacyClientDirs {
		if err := os.MkdirAll(filepath.Join(dir, "web", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if hint := legacyClientBuildHint(dir, real1114IncrementalFailure); hint != "" {
		t.Errorf("must not fire when the directories exist:\n%s", hint)
	}
}
