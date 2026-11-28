// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#2644 (integration of mendixlabs/mxcli#1977): the version check on
// git merge's mx, and the two holes left in it.

// An installed mx of the project's exact release wins over an mx on PATH.
// ResolveMxForVersion tries PATH first, so with 11.13.0 cached and 11.6.3 on
// PATH the merge was refused although the right engine was there.
func TestGitMergeMx_ExactInstallBeatsPath(t *testing.T) {
	_, mprPath := modelConflictRepo(t) // Mendix 11.13.0
	stubMergeMx(t, "/usr/local/bin/mx", "11.6.3")
	localMxForMerge = func(v string) string {
		if v == "11.13.0" {
			return "/cache/mxbuild/11.13.0/modeler/mx"
		}
		return ""
	}
	var stderr bytes.Buffer
	got, err := mergeMxLookup("", &stderr)(mprPath)
	if err != nil {
		t.Fatalf("refused although Mendix 11.13.0's mx is installed: %v", err)
	}
	if got != "/cache/mxbuild/11.13.0/modeler/mx" {
		t.Errorf("merge engine = %s, want the exact-version install", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("warned about an exact match: %s", stderr.String())
	}
}

// --mxbuild-path overrides the check only for an mx under that path. A folder
// without one made ResolveMxForVersion fall back to PATH, and the override then
// ran that unnamed mx with only a warning.
func TestGitMergeMx_MxbuildPathMustHoldTheMx(t *testing.T) {
	_, mprPath := modelConflictRepo(t)
	empty := t.TempDir()
	stubMergeMx(t, "/usr/local/bin/mx", "11.6.3")
	var stderr bytes.Buffer
	_, err := mergeMxLookup(empty, &stderr)(mprPath)
	if err == nil || !strings.Contains(err.Error(), "no mx in "+empty) {
		t.Fatalf("want a refusal naming %s, got %v (stderr %q)", empty, err, stderr.String())
	}

	// Control: an mx under the named path is used, the mismatch warned about.
	dir := t.TempDir()
	mx := filepath.Join(dir, "modeler", "mx")
	if err := os.MkdirAll(filepath.Dir(mx), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mx, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	stubMergeMx(t, mx, "11.6.3")
	stderr.Reset()
	got, err := mergeMxLookup(dir, &stderr)(mprPath)
	if err != nil || got != mx {
		t.Fatalf("an mx under --mxbuild-path was refused: %v (%s)", err, got)
	}
	if !strings.Contains(stderr.String(), "going on because --mxbuild-path was given") {
		t.Errorf("mismatch not warned about: %q", stderr.String())
	}
}

// The mx diff behind the conflict review is held to the same check.
func TestGitMergeMx_DiffIsRefusedToo(t *testing.T) {
	_, mprPath := modelConflictRepo(t)
	stubMergeMx(t, "/cache/mxbuild/11.6.3/modeler/mx", "11.6.3")
	var stderr bytes.Buffer
	err := mxRunnersFor("", &stderr).diff(mprPath, mprPath, filepath.Join(t.TempDir(), "d.json"))
	if err == nil || !strings.Contains(err.Error(), "is Mendix 11.6.3, but the project is Mendix 11.13.0") {
		t.Fatalf("want the diff refused, got %v", err)
	}
}

// A project whose version cannot be read is not checked, and says so, as
// marketplace package does.
func TestGitMergeMx_UnknownProjectVersionWarns(t *testing.T) {
	stubMergeMx(t, "/usr/local/bin/mx", "11.6.3")
	var stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "none.mpr")
	if _, err := mergeMxLookup("", &stderr)(missing); err != nil {
		t.Fatalf("refused a project of unknown version: %v", err)
	}
	if !strings.Contains(stderr.String(), "version is unknown, so the version of /usr/local/bin/mx was not checked") {
		t.Errorf("no warning for an unchecked mx: %q", stderr.String())
	}
}

// --mxbuild-path may name the mxbuild binary itself; its mx sits beside it.
func TestPathWithin(t *testing.T) {
	dir := t.TempDir()
	modeler := filepath.Join(dir, "modeler")
	if err := os.MkdirAll(modeler, 0o755); err != nil {
		t.Fatal(err)
	}
	mxbuild := filepath.Join(modeler, "mxbuild")
	if err := os.WriteFile(mxbuild, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	mx := filepath.Join(modeler, "mx")
	if err := os.WriteFile(mx, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		dir  string
		want bool
	}{{dir, true}, {modeler, true}, {mxbuild, true}, {t.TempDir(), false}, {filepath.Join(dir, "modeler2"), false}} {
		if got := pathWithin(mx, c.dir); got != c.want {
			t.Errorf("pathWithin(%s, %s) = %v, want %v", mx, c.dir, got, c.want)
		}
	}
}
