// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// ako/mxcli#1618 (mendixlabs/mxcli#1634): `exec` of `create java action … as
// $$ … $$` wrote javasource/<module>/actions/<Name>.java and named only the
// model element, so a commit of the model left the body behind. Through the
// real backend, on an MPR v1 and a v2 project: the file is named on its own
// line, project-relative, and the run ends with the summary.
func TestExecNamesTheSourceFilesItWritesBesideTheModel(t *testing.T) {
	const script = "create module Probe1618;\n" +
		"create or modify java action Probe1618.JA_Hello () returns String as $$\nreturn \"hi\";\n$$;\n" +
		"create or modify javascript action Probe1618.JS_Hello () returns String as $$\nreturn \"hi\";\n$$;\n"
	for _, fx := range atomicFixtures {
		t.Run(fx.name, func(t *testing.T) {
			dir := copyFixtureTree(t, fx.src)
			out := runScriptCapturing(t, filepath.Join(dir, fx.mpr), script)
			for _, want := range []string{
				"Created java action: Probe1618.JA_Hello\n" +
					"Wrote javasource/probe1618/actions/JA_Hello.java (Java action body; commit it with the model)\n",
				"Created javascript action: Probe1618.JS_Hello\n" +
					"Wrote javascriptsource/probe1618/actions/JS_Hello.js (JavaScript action body; commit it with the model)\n",
			} {
				if !strings.Contains(out, want) {
					t.Errorf("exec output lacks\n%s\ngot:\n%s", want, out)
				}
			}
			if !strings.HasSuffix(out, "Files changed outside the model: 2 (javasource/, javascriptsource/). Stage them with the .mpr.\n") {
				t.Errorf("exec output does not end with the summary:\n%s", out)
			}
			for _, p := range []string{"javasource/probe1618/actions/JA_Hello.java", "javascriptsource/probe1618/actions/JS_Hello.js"} {
				if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(p))); err != nil {
					t.Errorf("the reported file is not there: %v", err)
				}
			}

			// The control: the same script again rewrites nothing beside the
			// model, and says nothing about it.
			again := runScriptCapturing(t, filepath.Join(dir, fx.mpr), script)
			if strings.Contains(again, "Wrote ") || strings.Contains(again, "outside the model") {
				t.Errorf("a re-run with unchanged bodies reported a file:\n%s", again)
			}
		})
	}
}

// `mxcli test` passes --no-source-file-report to the exec that injects its
// endpoint, whose Java source it deletes again at cleanup.
func TestExecHasAHiddenNoSourceFileReportFlag(t *testing.T) {
	f := execCmd.Flags().Lookup("no-source-file-report")
	if f == nil {
		t.Fatal("exec has no --no-source-file-report flag; mxcli test's injection passes it")
	}
	if !f.Hidden {
		t.Error("--no-source-file-report is for mxcli test's own injection runs and should be hidden")
	}
}

func runScriptCapturing(t *testing.T, mprPath, script string) string {
	t.Helper()
	var out bytes.Buffer
	exec := executor.New(&out)
	exec.SetBackendFactory(newBackendFactory())
	conn, _ := visitor.Build("connect local '" + visitor.QuoteString(mprPath) + "';")
	if err := exec.ExecuteProgram(conn); err != nil {
		t.Fatalf("connect: %v", err)
	}
	out.Reset()
	prog, errs := visitor.Build(script)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs)
	}
	if err := runScriptAtomically(exec, prog); err != nil {
		t.Fatalf("exec: %v", err)
	}
	return out.String()
}
