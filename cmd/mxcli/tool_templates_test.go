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
