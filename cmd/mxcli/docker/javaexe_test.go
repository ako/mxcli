// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestJavaExePath_UsesHostSuffix pins the bug this helper exists for: every
// consumer built "<jh>/bin/java" without the .exe Windows needs, while detection
// appended it — so a correctly-detected JDK was passed to mxbuild, and exec'd for
// the runtime, in a form that need not resolve.
func TestJavaExePath_UsesHostSuffix(t *testing.T) {
	got := JavaExePath(filepath.Join("C:", "jdk-21"))
	want := "java"
	if runtime.GOOS == "windows" {
		want = "java.exe"
	}
	if filepath.Base(got) != want {
		t.Errorf("JavaExePath = %q, want it to end in %q", got, want)
	}
}

// TestJavaExeName covers the Windows form from any host. The suffix cannot be
// exercised on a Linux runner otherwise, which is why it stayed wrong.
func TestJavaExeName(t *testing.T) {
	if got := javaExeName("windows"); got != "java.exe" {
		t.Errorf("javaExeName(windows) = %q, want java.exe", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := javaExeName(goos); got != "java" {
			t.Errorf("javaExeName(%s) = %q, want java", goos, got)
		}
	}
}

// TestJdkSearchPathsFor_Windows asserts the list a Windows host searches,
// including the per-user install locations that Program Files globs miss.
// Studio Pro contributes no path of its own: it installs Eclipse Temurin, which
// the Adoptium entries already cover.
func TestJdkSearchPathsFor_Windows(t *testing.T) {
	t.Setenv("PROGRAMFILES", `C:\Program Files`)
	t.Setenv("LOCALAPPDATA", `C:\Users\dev\AppData\Local`)

	paths := jdkSearchPathsFor("windows")
	if len(paths) == 0 {
		t.Fatal("no JDK search paths for windows")
	}
	joined := strings.Join(paths, "|")
	// Separator-agnostic: filepath.Join uses "/" on the Linux runner this test
	// normally executes on.
	for _, want := range []string{"Eclipse Adoptium", "Microsoft", "AppData", "Programs"} {
		if !strings.Contains(joined, want) {
			t.Errorf("windows search paths should include %q, got:\n%s", want, strings.Join(paths, "\n"))
		}
	}
	for _, p := range paths {
		if !strings.Contains(p, "jdk-21") {
			t.Errorf("every pattern should pin JDK 21, got %q", p)
		}
	}
}

func TestJdkSearchPathsFor_UnixHosts(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		paths := jdkSearchPathsFor(goos)
		if len(paths) == 0 {
			t.Fatalf("no JDK search paths for %s", goos)
		}
		for _, p := range paths {
			if !strings.Contains(p, "21") {
				t.Errorf("%s: every pattern should pin JDK 21, got %q", goos, p)
			}
		}
	}
}
