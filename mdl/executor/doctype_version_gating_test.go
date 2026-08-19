// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/sdk/mpr/version"
)

// nightlyMatrix is the Mendix version set .github/workflows/nightly.yml runs the
// doctype scripts against. A script that only parses on the newest of them is a
// nightly failure on the others, reported hours later against whatever landed in
// between — which is how the DecimalScale gating below was found.
var nightlyMatrix = []*version.ProjectVersion{
	{MajorVersion: 10, MinorVersion: 24, ProductVersion: "10.24.24.119349"},
	{MajorVersion: 11, MinorVersion: 6, ProductVersion: "11.6.8"},
	{MajorVersion: 11, MinorVersion: 12, ProductVersion: "11.12.2"},
	{MajorVersion: 11, MinorVersion: 13, ProductVersion: "11.13.0"},
}

// TestDoctypeScriptsParseAfterVersionFiltering guards the whole doctype corpus
// against gating that produces MDL the parser rejects.
//
// A `-- @version:` section is removed by blanking its lines, and what is left
// has to be a valid script on its own. The trap this catches is that a `/** */`
// block is a DOCUMENTATION comment bound to the statement after it: gate the
// statement while leaving its doc comment outside the section and the comment is
// orphaned, so the script dies with "no viable alternative at input '/**...'" —
// at the NEXT statement, tens of lines further down, which reads like an
// unrelated syntax error. Line comments (`--`) are free-standing and safe on
// either side of the directive.
//
// This runs without mxbuild, so a mis-gated script fails in seconds on every
// push rather than in the nightly for one matrix entry.
func TestDoctypeScriptsParseAfterVersionFiltering(t *testing.T) {
	all, err := filepath.Glob("../../mdl-examples/doctype-tests/*.mdl")
	if err != nil {
		t.Fatalf("glob doctype scripts: %v", err)
	}
	// `.test.mdl` / `.tests.mdl` are microflow test SPECS, not MDL scripts: they
	// carry @test/@expect annotations and do not parse with this parser even
	// unfiltered. TestMxCheck_DoctypeScripts excludes them for the same reason.
	var scripts []string
	for _, s := range all {
		name := filepath.Base(s)
		if strings.HasSuffix(name, ".test.mdl") || strings.HasSuffix(name, ".tests.mdl") {
			continue
		}
		scripts = append(scripts, s)
	}
	if len(scripts) == 0 {
		t.Fatal("no doctype scripts found — the glob is wrong, and a passing run here would prove nothing")
	}

	for _, script := range scripts {
		content, err := os.ReadFile(script)
		if err != nil {
			t.Errorf("%s: %v", script, err)
			continue
		}
		name := filepath.Base(script)

		for _, pv := range nightlyMatrix {
			t.Run(name+"/"+pv.ProductVersion, func(t *testing.T) {
				filtered, _ := filterByVersion(string(content), pv)
				if _, errs := visitor.Build(filtered); len(errs) > 0 {
					t.Errorf("filtered for Mendix %s, the script no longer parses: %v\n"+
						"A gated section must contain everything that belongs to it — including any "+
						"`/** */` doc comment, which binds to the statement that follows it.",
						pv.ProductVersion, errs[0])
				}
			})
		}
	}
}
