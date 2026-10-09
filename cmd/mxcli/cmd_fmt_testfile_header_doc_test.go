// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mendixlabs/mxcli#1356: "fmt --upgrade adds `mdl 1;` to a .test.mdl, but fmt
// --help and the test-microflows skill say test files take no header". The
// behaviour is the intended one (ako/mxcli#847: the runner and check read a
// test file's header, and --upgrade adds it by default); the two texts an
// agent reads first were left describing the pre-#847 behaviour. This pins
// the behaviour on the issue's own file, then holds both texts to it.
func TestFmtUpgrade_TestFileHeaderDocsMatchBehaviour(t *testing.T) {
	path := filepath.Join(t.TempDir(), "suite.test.mdl")
	src := "/**\n * @test Finds a customer\n * @expect $Found = true\n */\n" +
		"retrieve $Customer from MyModule.Customer where Name = 'Ann' limit 1;\n$Found = $Customer != empty;\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runFmt(t, "--upgrade", "-w", path); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !strings.HasPrefix(string(got), "mdl 1;\n") {
		t.Fatalf("fmt --upgrade no longer adds the header to a test file; this test's premise is gone:\n%s", got)
	}

	skill, err := os.ReadFile(filepath.Join("..", "..", ".claude", "skills", "mendix", "test-microflows", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	docs := map[string]string{
		"fmt --help":               fmtCmd.Long,
		"test-microflows/SKILL.md": string(skill),
	}
	// "takes no language header", "takes **no `mdl 1;` header**", "adds none to it".
	stale := regexp.MustCompile("(?i)takes\\s+(\\*\\*)?no\\b|adds\\s+none\\s+to\\s+it")
	for name, body := range docs {
		var said bool
		for _, para := range regexp.MustCompile(`\n\s*\n`).Split(body, -1) {
			p := strings.Join(strings.Fields(para), " ")
			if !strings.Contains(strings.ToLower(p), "test file") || !strings.Contains(p, "header") {
				continue
			}
			if m := stale.FindString(p); m != "" {
				t.Errorf("%s says a test file takes no header (%q), but fmt --upgrade adds one:\n%s", name, m, p)
			}
			if strings.Contains(p, "--header=false") {
				said = true
			}
		}
		if !said {
			t.Errorf("%s: no paragraph about a test file's header names --header=false, the way to decline it", name)
		}
	}
}
