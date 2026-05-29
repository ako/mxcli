// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"strings"
	"testing"
)

// TestNativeMxBuildForSetup_Linux guards issue #608: on Linux (CI, devcontainers)
// `setup mxbuild` should download the CDN binary normally — no guidance, no error.
func TestNativeMxBuildForSetup_Linux(t *testing.T) {
	path, guidance, err := NativeMxBuildForSetup("linux", "11.10.0")
	if err != nil {
		t.Fatalf("Linux must not error, got %v", err)
	}
	if path != "" {
		t.Errorf("Linux must not resolve a native path (download is appropriate), got %q", path)
	}
	if guidance != "" {
		t.Errorf("Linux must not produce guidance, got %q", guidance)
	}
}

// TestNativeMxBuildForSetup_WindowsNoStudioPro guards issue #608: on Windows with
// no Studio Pro installed for the version, `setup mxbuild` must refuse to download
// the Linux CDN binary (which would later fail with "Exec format error") and
// instead return actionable guidance pointing at Studio Pro's bundled mx.exe.
func TestNativeMxBuildForSetup_WindowsNoStudioPro(t *testing.T) {
	// Redirect Windows Program Files lookup at a temp dir with no Mendix install.
	dir := t.TempDir()
	t.Setenv("PROGRAMFILES", dir)
	t.Setenv("PROGRAMW6432", dir)
	t.Setenv("PROGRAMFILES(X86)", dir)
	t.Setenv("SystemDrive", dir)

	path, guidance, err := NativeMxBuildForSetup("windows", "11.10.0")
	if err == nil {
		t.Fatal("Windows with no Studio Pro must return an error, not silently download a Linux binary")
	}
	if path != "" {
		t.Errorf("no native path expected, got %q", path)
	}
	if !strings.Contains(guidance, "Studio Pro") {
		t.Errorf("guidance should mention Studio Pro, got %q", guidance)
	}
	if !strings.Contains(guidance, "--force") {
		t.Errorf("guidance should mention the --force escape hatch, got %q", guidance)
	}
	if !strings.Contains(guidance, "11.10.0") {
		t.Errorf("guidance should name the version, got %q", guidance)
	}
}

// TestNativeMxBuildForSetup_DarwinNoStudioPro mirrors the Windows case for macOS.
func TestNativeMxBuildForSetup_DarwinNoStudioPro(t *testing.T) {
	setTestApplicationsDir(t, t.TempDir())

	_, guidance, err := NativeMxBuildForSetup("darwin", "11.10.0")
	if err == nil {
		t.Fatal("macOS with no Studio Pro must return an error")
	}
	if !strings.Contains(guidance, "Studio Pro") || !strings.Contains(guidance, "--force") {
		t.Errorf("guidance should mention Studio Pro and --force, got %q", guidance)
	}
}
