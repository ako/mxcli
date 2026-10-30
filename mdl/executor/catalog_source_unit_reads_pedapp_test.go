// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1789: `refresh catalog full source` describes every document, and
// each microflow describe found its microflow — and every flow it calls, for
// the void-call warning — by listing every microflow of the project from disk.
// One refresh of a 4,753-unit project opened 7,532,649 unit files: minutes on
// macOS, hours on Windows, where Defender scans each open. The build now holds
// the units it reads in memory, and a describe reads the document it was
// pinned to and the microflows it calls by ID rather than by listing.

func TestSourceCatalogBuild_ReadsUnitFilesLinearly(t *testing.T) {
	exec, out, dir := openPedAppCopy(t)
	// Every microflow calls the one before it, so each describe also resolves
	// a called flow: both of the issue's lookups are exercised.
	const n = 40
	if err := agreeExec(t, exec, redeclareChain(n, "create")); err != nil {
		t.Fatalf("setup: %v\n%s", err, out.String())
	}
	units := 0
	_ = filepath.Walk(filepath.Join(dir, "mprcontents"), func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".mxunit") {
			units++
		}
		return nil
	})

	before := unitFileReads(t, exec)
	if err := agreeExec(t, exec, "refresh catalog full source force;"); err != nil {
		t.Fatalf("source build: %v\n%s", err, out.String())
	}
	// Each unit file is read from disk at most once per build, however often
	// the describes look a document up. Before the fix this build read 11,017
	// files of these 409.
	if reads := unitFileReads(t, exec) - before; reads > int64(units) {
		t.Errorf("a source catalog build read %d unit files; the project has %d", reads, units)
	}

	// The narrowing must not change the text: the pinned describe still
	// renders the body, call included.
	out.Reset()
	if err := agreeExec(t, exec, "select SourceText from catalog.source where QualifiedName = 'MyFirstModule.MF_Chain040';"); err != nil {
		t.Fatalf("query source: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "call microflow MyFirstModule.MF_Chain039") {
		t.Errorf("the source row of MF_Chain040 lacks its call:\n%s", out.String())
	}
}
