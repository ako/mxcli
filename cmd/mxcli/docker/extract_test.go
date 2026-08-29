// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarEntry is one header (plus body, for a regular file) in a synthetic archive.
type tarEntry struct {
	name     string
	typ      byte
	linkname string
	body     string
}

func tarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Typeflag: e.typ, Linkname: e.linkname, Mode: 0o755}
		if e.typ == tar.TypeReg {
			h.Size = int64(len(e.body))
			h.Mode = 0o644
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if e.typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// extractSandbox lays out base/{root,OUTSIDE} and returns both. The extractor is
// pointed at root; anything that appears anywhere else in base has escaped.
func extractSandbox(t *testing.T) (base, root, outside string) {
	t.Helper()
	base = t.TempDir()
	root = filepath.Join(base, "root")
	outside = filepath.Join(base, "OUTSIDE")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return base, root, outside
}

// escapedPaths lists everything under base that is not under root. The walk is
// deliberately over the whole sandbox rather than over a list of expected
// filenames: an escape that lands somewhere unanticipated must still be caught.
func escapedPaths(t *testing.T, base, root, outside string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(base, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // a dangling symlink is not a walk failure
		}
		switch p {
		case base, root, outside:
			return nil
		}
		if rel, relErr := filepath.Rel(root, p); relErr == nil && !strings.HasPrefix(rel, "..") {
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A symlink whose parent is itself a symlink is resolved by the OS against its
// REAL parent, not its lexical one — so a chain of two "up one level" links
// reaches outside the extraction root while every link, checked on its own,
// looks like it stays inside.
//
// Measured against the lexical guard this replaced: "double" wrote pwned into
// the parent of the extraction root, and "triple" wrote into an arbitrary
// sibling directory and left a symlink there too. Both are the reason
// os.Root does the resolving now. CodeQL go/unsafe-unzip-symlink, alerts 4-7.
func TestExtractTarGz_RefusesChainedUpDirSymlinks(t *testing.T) {
	cases := []struct {
		name    string
		entries []tarEntry
	}{
		// sub/up really IS the root, so sub/up/w really is root/w, and its ".."
		// is the root's parent. Nothing in either name contains "..".
		{"two links reach the root's parent", []tarEntry{
			{name: "sub", typ: tar.TypeDir},
			{name: "sub/up", typ: tar.TypeSymlink, linkname: ".."},
			{name: "sub/up/w", typ: tar.TypeSymlink, linkname: ".."},
			{name: "sub/up/w/pwned", typ: tar.TypeReg, body: "x"},
		}},
		// One more link turns "the parent" into any directory beside the root.
		{"three links reach a sibling directory", []tarEntry{
			{name: "sub", typ: tar.TypeDir},
			{name: "sub/up", typ: tar.TypeSymlink, linkname: ".."},
			{name: "sub/up/w", typ: tar.TypeSymlink, linkname: ".."},
			{name: "sub/up/w/v", typ: tar.TypeSymlink, linkname: "OUTSIDE"},
			{name: "sub/up/w/v/pwned", typ: tar.TypeReg, body: "x"},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, strip1 := range []bool{false, true} {
				entries := c.entries
				if strip1 {
					// Same archive under a top-level directory, so the stripping
					// variant is exercised too — it had its own copy of the bug.
					entries = make([]tarEntry, len(c.entries))
					for i, e := range c.entries {
						e.name = "pkg/" + e.name
						entries[i] = e
					}
				}
				base, root, outside := extractSandbox(t)
				// The extractor may or may not error; what matters is where the
				// bytes went, so the error is reported and not asserted on.
				err := extractTarGzInto(bytes.NewReader(tarGz(t, entries)), root, strip1)
				if escaped := escapedPaths(t, base, root, outside); len(escaped) > 0 {
					t.Errorf("strip1=%v: %d path(s) written outside the extraction root: %v (extract err: %v)",
						strip1, len(escaped), escaped, err)
				}
			}
		})
	}
}

// The single-step escapes, which the lexical guard did already refuse. Kept as
// the controls for the ones above: if these ever regress, the chained cases
// stop proving anything specific about parent resolution.
func TestExtractTarGz_RefusesSingleStepEscapes(t *testing.T) {
	cases := []struct {
		name    string
		entries func(outside string) []tarEntry
	}{
		{"absolute symlink target", func(outside string) []tarEntry {
			return []tarEntry{
				{name: "esc", typ: tar.TypeSymlink, linkname: outside},
				{name: "esc/pwned", typ: tar.TypeReg, body: "x"},
			}
		}},
		{"relative symlink target above the root", func(string) []tarEntry {
			return []tarEntry{
				{name: "esc", typ: tar.TypeSymlink, linkname: "../../OUTSIDE"},
				{name: "esc/pwned", typ: tar.TypeReg, body: "x"},
			}
		}},
		{"absolute entry name", func(outside string) []tarEntry {
			return []tarEntry{{name: filepath.Join(outside, "pwned"), typ: tar.TypeReg, body: "x"}}
		}},
		{"dot-dot in the entry name", func(string) []tarEntry {
			return []tarEntry{{name: "../pwned", typ: tar.TypeReg, body: "x"}}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base, root, outside := extractSandbox(t)
			err := extractTarGzInto(bytes.NewReader(tarGz(t, c.entries(outside))), root, false)
			if escaped := escapedPaths(t, base, root, outside); len(escaped) > 0 {
				t.Errorf("wrote outside the extraction root: %v (extract err: %v)", escaped, err)
			}
		})
	}
}

// The control for every refusal above: an ordinary archive must still come out
// whole. Without this, refusing everything would pass the suite.
//
// "foo..bar" is here because the guard this replaced tested for ".." as a
// SUBSTRING and dropped the file silently — a legitimate name, no warning, no
// error, absent from the extracted tree.
func TestExtractTarGz_ExtractsAnOrdinaryArchive(t *testing.T) {
	entries := []tarEntry{
		{name: "pkg", typ: tar.TypeDir},
		{name: "pkg/runtime", typ: tar.TypeDir},
		{name: "pkg/runtime/launcher.jar", typ: tar.TypeReg, body: "JAR"},
		{name: "pkg/lib/libfoo.so.6", typ: tar.TypeReg, body: "SO"},
		{name: "pkg/foo..bar", typ: tar.TypeReg, body: "DOTS"},
		{name: "pkg/lib/libfoo.so", typ: tar.TypeSymlink, linkname: "libfoo.so.6"},
	}
	for _, strip1 := range []bool{false, true} {
		_, root, _ := extractSandbox(t)
		if err := extractTarGzInto(bytes.NewReader(tarGz(t, entries)), root, strip1); err != nil {
			t.Fatalf("strip1=%v: %v", strip1, err)
		}
		prefix := "pkg"
		if strip1 {
			prefix = ""
		}
		for name, want := range map[string]string{
			"runtime/launcher.jar": "JAR",
			"lib/libfoo.so.6":      "SO",
			"foo..bar":             "DOTS",
			"lib/libfoo.so":        "SO", // read through the symlink
		} {
			got, err := os.ReadFile(filepath.Join(root, prefix, filepath.FromSlash(name)))
			if err != nil {
				t.Errorf("strip1=%v: %s: %v", strip1, name, err)
				continue
			}
			if string(got) != want {
				t.Errorf("strip1=%v: %s = %q, want %q", strip1, name, got, want)
			}
		}
	}
}

// symlinkStaysInside is the half os.Root cannot do: os.Root stops mxcli
// following an outward link, but the extracted tree is handed to mxbuild and a
// JVM, which will. Asserted directly because the chain that reaches it is three
// archive entries deep.
func TestSymlinkStaysInside_ResolvesTheRealParent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	// sub/up -> .. is the root itself: allowed, and genuinely inside.
	if !symlinkStaysInside(root, filepath.Join("sub", "up"), "..") {
		t.Fatal("control failed: a link resolving to the root was refused")
	}
	if err := os.Symlink("..", filepath.Join(root, "sub", "up")); err != nil {
		t.Fatal(err)
	}
	// sub/up/w -> .. looks like root/sub lexically. Its real parent is the root,
	// so it points at the root's parent and must be refused.
	if symlinkStaysInside(root, filepath.Join("sub", "up", "w"), "..") {
		t.Error("a link whose real parent is the root was allowed to point above it")
	}
	if symlinkStaysInside(root, "link", string(filepath.Separator)+"etc") {
		t.Error("an absolute link target outside the root was allowed")
	}
	if !symlinkStaysInside(root, filepath.Join("sub", "rel"), "../sub") {
		t.Error("control failed: a relative link inside the root was refused")
	}
}

// A bare prefix test puts /cache/runtime-evil inside /cache/runtime. The
// separator is the whole difference.
func TestPathWithin_RequiresASeparator(t *testing.T) {
	sep := string(os.PathSeparator)
	root := sep + "cache" + sep + "runtime"
	if !pathWithin(root, root) {
		t.Error("the root is not within itself")
	}
	if !pathWithin(root, root+sep+"lib") {
		t.Error("a child is not within the root")
	}
	if pathWithin(root, root+"-evil") {
		t.Error("a sibling sharing the root's name prefix was treated as inside it")
	}
}

// Names that must be dropped, and names that must not. The substring test this
// replaced got the last two wrong.
func TestTarEntryPath(t *testing.T) {
	cases := []struct {
		in     string
		strip1 bool
		want   string
		ok     bool
	}{
		{"pkg/runtime/a.jar", true, filepath.FromSlash("runtime/a.jar"), true},
		{"pkg/runtime/a.jar", false, filepath.FromSlash("pkg/runtime/a.jar"), true},
		{"pkg", true, "", false},  // the top-level entry itself
		{"pkg/", true, "", false}, // ditto, with a trailing slash
		{"../escape", false, "", false},
		{"pkg/../../escape", true, "", false},
		{"/etc/passwd", false, "", false},
		{"./pkg/a", true, filepath.FromSlash("a"), true},
		{"foo..bar", false, "foo..bar", true},
		{"pkg/lib/libfoo.so.6", true, filepath.FromSlash("lib/libfoo.so.6"), true},
	}
	for _, c := range cases {
		got, ok := tarEntryPath(c.in, c.strip1)
		if got != c.want || ok != c.ok {
			t.Errorf("tarEntryPath(%q, %v) = %q,%v want %q,%v", c.in, c.strip1, got, ok, c.want, c.ok)
		}
	}
}
