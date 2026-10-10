// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDownloadMxcliBinary_HTTP404ReturnsError verifies that a 404 from the
// release server is surfaced as an error. This exercises the path in
// cmd_new.go step 7 that warns when the download fails.
func TestDownloadMxcliBinary_HTTP404ReturnsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Temporarily override the GitHub releases URL by using a repo path that
	// maps to our test server. We test the underlying helper directly.
	outPath := filepath.Join(t.TempDir(), "mxcli")
	err := downloadMxcliBinaryFromURL(ts.URL+"/mxcli-linux-amd64", outPath, os.Stdout)
	if err == nil {
		t.Fatal("expected error on HTTP 404, got nil")
	}
}

// TestDownloadMxcliBinary_SuccessWritesBinary verifies that a successful
// download writes the binary to the output path.
func TestDownloadMxcliBinary_SuccessWritesBinary(t *testing.T) {
	content := []byte("fake-binary-content")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	}))
	defer ts.Close()

	outPath := filepath.Join(t.TempDir(), "mxcli")
	err := downloadMxcliBinaryFromURL(ts.URL+"/mxcli-linux-amd64", outPath, os.Stdout)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("output file not written: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("file content mismatch: got %q, want %q", got, content)
	}
}

// TestProvisionDevcontainerMxcli_DownloadFailureIsAWarning is mendixlabs/mxcli#1365:
// "`mxcli new` exits 1 at step 7 when the Linux devcontainer binary can't be
// downloaded, although the project is complete". By step 7 the project is usable,
// so a failed download must warn and return — the helper used to os.Exit(1),
// which ends this test binary with "exit status 1" instead of reaching the asserts.
func TestProvisionDevcontainerMxcli_DownloadFailureIsAWarning(t *testing.T) {
	var out, errOut bytes.Buffer
	failing := func(tag, outPath string, w io.Writer) error {
		return errors.New("HTTP 404")
	}
	provisionDevcontainerMxcli("windows", "v0.5.0", filepath.Join(t.TempDir(), "mxcli"), failing, &out, &errOut)

	got := errOut.String()
	if !strings.Contains(got, "Warning: could not download Linux mxcli") {
		t.Errorf("download failure should be reported as a warning, stderr:\n%s", got)
	}
	if !strings.Contains(got, "The project is usable") {
		t.Errorf("warning should say the project is usable, stderr:\n%s", got)
	}
	if !strings.Contains(got, "mxcli setup mxcli") {
		t.Errorf("warning should keep the 'mxcli setup mxcli' hint, stderr:\n%s", got)
	}
}

// A build with no matching release (plain `go build` reports 0.1.0; `make build`
// in a tagless fork reports a commit hash) cannot be downloaded. Asking anyway is
// a guaranteed 404, so it is skipped with a note instead.
func TestProvisionDevcontainerMxcli_DevBuildSkipsDownload(t *testing.T) {
	var out, errOut bytes.Buffer
	called := false
	download := func(tag, outPath string, w io.Writer) error {
		called = true
		return nil
	}
	provisionDevcontainerMxcli("windows", "", filepath.Join(t.TempDir(), "mxcli"), download, &out, &errOut)
	if called {
		t.Error("a dev build has no matching release; the download should not be attempted")
	}
	if !strings.Contains(errOut.String(), "--tag") {
		t.Errorf("skip note should point at 'mxcli setup mxcli --tag', stderr:\n%s", errOut.String())
	}
}

func TestProvisionDevcontainerMxcli_ReleaseBuildDownloads(t *testing.T) {
	var out, errOut bytes.Buffer
	var gotTag string
	download := func(tag, outPath string, w io.Writer) error {
		gotTag = tag
		return nil
	}
	provisionDevcontainerMxcli("darwin", "v0.5.0", filepath.Join(t.TempDir(), "mxcli"), download, &out, &errOut)
	if gotTag != "v0.5.0" {
		t.Errorf("download tag = %q, want v0.5.0", gotTag)
	}
	if errOut.Len() != 0 {
		t.Errorf("successful download should not warn, stderr:\n%s", errOut.String())
	}
}

func TestDevcontainerReleaseTag(t *testing.T) {
	cases := []struct {
		ldflagsVersion, want string
	}{
		{"", ""},                       // plain go build: version stays 0.1.0
		{"dev", ""},                    // make build outside git
		{"abc1234", ""},                // make build in a tagless fork
		{"abc1234-dirty", ""},          // same, dirty tree
		{"v0.5.0", "v0.5.0"},           // release
		{"v0.5.0-3-gabcdef", "v0.5.0"}, // past a tag
		{"v0.5.0-3-gabcdef-dirty", "v0.5.0"},
		{"nightly-2026-10-01", "nightly"},
	}
	for _, c := range cases {
		if got := devcontainerReleaseTag(c.ldflagsVersion); got != c.want {
			t.Errorf("devcontainerReleaseTag(%q) = %q, want %q", c.ldflagsVersion, got, c.want)
		}
	}
}
