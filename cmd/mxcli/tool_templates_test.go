// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGenerateDevcontainerJSON_ValidJSON ensures both runtimes produce valid JSON
// and that the podman variant no longer references the non-existent
// podman-in-podman devcontainer feature (issue #653).
func TestGenerateDevcontainerJSON_ValidJSON(t *testing.T) {
	for _, runtime := range []string{"docker", "podman"} {
		t.Run(runtime, func(t *testing.T) {
			out := generateDevcontainerJSON("MyApp", "App.mpr", runtime)
			var v any
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				t.Fatalf("invalid JSON for %s runtime: %v\n%s", runtime, err, out)
			}
			if strings.Contains(out, "podman-in-podman") {
				t.Errorf("%s devcontainer.json still references the non-existent podman-in-podman feature", runtime)
			}
		})
	}
}

func TestGenerateDevcontainerJSON_Docker(t *testing.T) {
	out := generateDevcontainerJSON("MyApp", "App.mpr", "docker")
	if !strings.Contains(out, "docker-in-docker") {
		t.Error("docker variant should use the docker-in-docker feature")
	}
	if strings.Contains(out, "MXCLI_CONTAINER_CLI") {
		t.Error("docker variant should not set MXCLI_CONTAINER_CLI")
	}
}

func TestGenerateDevcontainerJSON_Podman(t *testing.T) {
	out := generateDevcontainerJSON("MyApp", "App.mpr", "podman")
	if !strings.Contains(out, `"MXCLI_CONTAINER_CLI": "podman"`) {
		t.Error("podman variant should set MXCLI_CONTAINER_CLI=podman")
	}
	if strings.Contains(out, "docker-in-docker") {
		t.Error("podman variant should not use the docker-in-docker feature")
	}
	if !strings.Contains(out, `"features": {}`) {
		t.Error("podman variant should have an empty features block (podman is installed via the Dockerfile)")
	}
}

// TestGenerateDockerfile_Podman verifies the podman runtime adds a Podman install
// to the generated Dockerfile, while docker does not.
func TestGenerateDockerfile_Podman(t *testing.T) {
	podman := generateDockerfile("MyApp", "App.mpr", "podman")
	if !strings.Contains(podman, "install") || !strings.Contains(podman, "podman") {
		t.Error("podman runtime should install podman in the Dockerfile")
	}
	// The systemd-less gotchas must be configured or rootless podman won't run.
	for _, want := range []string{"cgroup_manager", "events_logger", "subuid", "subgid"} {
		if !strings.Contains(podman, want) {
			t.Errorf("podman Dockerfile missing required config %q", want)
		}
	}

	docker := generateDockerfile("MyApp", "App.mpr", "docker")
	if strings.Contains(docker, "/etc/containers") {
		t.Error("docker runtime should not include podman configuration")
	}
}

// TestGenerateDockerfile_PlaywrightArm64 guards the arm64 Playwright provisioning
// fix: browsers must be installed via @playwright/cli's bundled playwright-core
// (not a transient "npx playwright"), into a world-readable shared cache, with a
// stable headless-shell symlink that the generated cli.config.json pins.
func TestGenerateDockerfile_PlaywrightArm64(t *testing.T) {
	df := generateDockerfile("MyApp", "App.mpr", "docker")

	if strings.Contains(df, "npx playwright install") {
		t.Error("Dockerfile must not use 'npx playwright install' — it resolves a different playwright than the CLI's bundled core (wrong revision + wrong cache)")
	}
	if strings.Contains(df, "@playwright/cli@latest") {
		t.Error("@playwright/cli must be pinned to a known-good version, not @latest (its CLI surface shifts between releases)")
	}
	if !strings.Contains(df, "@playwright/cli@0.1.") {
		t.Error("Dockerfile should install a pinned @playwright/cli@0.1.x version")
	}
	for _, want := range []string{
		"@playwright/cli/node_modules/playwright-core/cli.js", // install via bundled core
		"chromium-headless-shell",                             // headless needs the shell build
		"PLAYWRIGHT_BROWSERS_PATH",                            // shared, non-root-only cache
		"/usr/local/bin/mx-headless-shell",                    // stable symlink for the config pin
	} {
		if !strings.Contains(df, want) {
			t.Errorf("Dockerfile missing expected Playwright provisioning fragment %q", want)
		}
	}
}

// TestGeneratePlaywrightConfig_PinsHeadlessShell ensures the generated config is
// valid JSON and pins executablePath to the stable symlink the Dockerfile
// creates — without it, the alpha CLI falls back to the unavailable chrome
// channel on arm64.
func TestGeneratePlaywrightConfig_PinsHeadlessShell(t *testing.T) {
	cfg := generatePlaywrightConfig()

	var v struct {
		Browser struct {
			BrowserName   string `json:"browserName"`
			LaunchOptions struct {
				Headless       bool   `json:"headless"`
				ExecutablePath string `json:"executablePath"`
			} `json:"launchOptions"`
		} `json:"browser"`
	}
	if err := json.Unmarshal([]byte(cfg), &v); err != nil {
		t.Fatalf("playwright config is not valid JSON: %v\n%s", err, cfg)
	}
	if v.Browser.BrowserName != "chromium" {
		t.Errorf("browserName = %q, want chromium", v.Browser.BrowserName)
	}
	if got := v.Browser.LaunchOptions.ExecutablePath; got != "/usr/local/bin/mx-headless-shell" {
		t.Errorf("executablePath = %q, want the stable Dockerfile symlink /usr/local/bin/mx-headless-shell", got)
	}
}
