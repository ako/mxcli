// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// ako/mxcli#1618 (mendixlabs/mxcli#1634): `create java action … as $$ … $$`
// stores the action in the model AND writes its body to
// javasource/<module>/actions/<Name>.java, and `exec` said only "Created java
// action: Probe.JA_Hello". A workflow that commits the model (.mpr and
// mprcontents/) left the .java behind, and a clone got an action with no body.
// Every file a statement changes beside the model is now named on its own line,
// and a run that changed any ends with one line saying to stage them.

// sourceFileMock records the files its java-action writer "writes", the way the
// modelsdk backend does, and hands them over on TakeSourceFileChanges.
type sourceFileMock struct {
	*mock.MockBackend
	pending []backend.SourceFileChange
}

func (m *sourceFileMock) TakeSourceFileChanges() []backend.SourceFileChange {
	c := m.pending
	m.pending = nil
	return c
}

// newSourceFileExecutor returns an executor whose backend creates Java actions
// in module Probe. A body equal to unchangedBody is "already on disk" and
// records nothing, as WriteSourceIfChanged does for an identical rewrite.
func newSourceFileExecutor(t *testing.T, out *bytes.Buffer, unchangedBody string) *Executor {
	t.Helper()
	mod := mkModule("Probe")
	m := &sourceFileMock{}
	m.MockBackend = &mock.MockBackend{
		IsConnectedFunc:          func() bool { return true },
		ListModulesFunc:          func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListJavaActionsFunc:      func() ([]*types.JavaAction, error) { return nil, nil },
		ReadJavaActionByNameFunc: func(string) (*javaactions.JavaAction, error) { return nil, nil },
		CreateJavaActionFunc:     func(*javaactions.JavaAction) error { return nil },
		WriteJavaSourceFileFunc: func(module, action, code string, _ []*javaactions.JavaActionParameter,
			_ javaactions.CodeActionReturnType, _ []string, _ *string, _ string) error {
			if strings.TrimSpace(code) != unchangedBody {
				m.pending = append(m.pending, backend.SourceFileChange{
					Path: "javasource/" + strings.ToLower(module) + "/actions/" + action + ".java",
					What: "Java action body",
				})
			}
			return nil
		},
	}
	e := New(out)
	e.backend = m
	return e
}

func buildProgram(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		t.Fatalf("parse: %v", errs[0])
	}
	return prog
}

const twoJavaActions = `create java action Probe.JA_Hello () returns String as $$
return "hi";
$$;
create java action Probe.JA_Bye () returns String as $$
return "bye";
$$;
`

func TestExecNamesEveryFileItWritesBesideTheModel(t *testing.T) {
	var out bytes.Buffer
	e := newSourceFileExecutor(t, &out, "")
	if err := e.ExecuteProgram(buildProgram(t, twoJavaActions)); err != nil {
		t.Fatal(err)
	}
	want := "Created java action: Probe.JA_Hello\n" +
		"Wrote javasource/probe/actions/JA_Hello.java (Java action body; commit it with the model)\n" +
		"Created java action: Probe.JA_Bye\n" +
		"Wrote javasource/probe/actions/JA_Bye.java (Java action body; commit it with the model)\n" +
		"Files changed outside the model: 2 (javasource/). Stage them with the .mpr.\n"
	if got := out.String(); got != want {
		t.Errorf("exec output:\n%s\nwant:\n%s", got, want)
	}
}

// The control: a body already on disk is not rewritten, so there is nothing to
// stage and nothing is said — not a "Wrote" line, not a summary.
func TestExecSaysNothingAboutAnUnchangedSourceFile(t *testing.T) {
	var out bytes.Buffer
	e := newSourceFileExecutor(t, &out, `return "hi";`)
	if err := e.ExecuteProgram(buildProgram(t, twoJavaActions)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.Contains(got, "JA_Hello.java") {
		t.Errorf("an unchanged file was reported:\n%s", got)
	}
	if !strings.Contains(got, "Wrote javasource/probe/actions/JA_Bye.java") {
		t.Errorf("control: the changed file was not reported:\n%s", got)
	}
	if !strings.Contains(got, "Files changed outside the model: 1 (javasource/)") {
		t.Errorf("the summary does not count only the changed file:\n%s", got)
	}

	out.Reset()
	e = newSourceFileExecutor(t, &out, `return "hi";`)
	if err := e.ExecuteProgram(buildProgram(t, "create java action Probe.JA_Hello () returns String as $$\nreturn \"hi\";\n$$;\n")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "Wrote") || strings.Contains(got, "outside the model") {
		t.Errorf("a run that changed no file beside the model mentioned one:\n%s", got)
	}
}

// One statement run on its own (the REPL) names its file; the summary belongs to
// a program run, where the per-file lines may have scrolled away.
func TestASingleStatementNamesItsFileWithoutASummary(t *testing.T) {
	var out bytes.Buffer
	e := newSourceFileExecutor(t, &out, "")
	prog := buildProgram(t, "create java action Probe.JA_Hello () returns String as $$\nreturn \"hi\";\n$$;\n")
	if err := e.Execute(prog.Statements[0]); err != nil {
		t.Fatal(err)
	}
	want := "Created java action: Probe.JA_Hello\n" +
		"Wrote javasource/probe/actions/JA_Hello.java (Java action body; commit it with the model)\n"
	if got := out.String(); got != want {
		t.Errorf("output:\n%s\nwant:\n%s", got, want)
	}
}

// `mxcli test` injects a Java action into the project and deletes it again at
// cleanup; telling its user to commit that file would be wrong. A caller whose
// writes are temporary turns the report off — and what the backend recorded is
// still taken, so it is not reported by a later statement either.
func TestSourceFileReportCanBeTurnedOff(t *testing.T) {
	var out bytes.Buffer
	e := newSourceFileExecutor(t, &out, "")
	e.SetSourceFileReport(false)
	if err := e.ExecuteProgram(buildProgram(t, twoJavaActions)); err != nil {
		t.Fatal(err)
	}
	want := "Created java action: Probe.JA_Hello\nCreated java action: Probe.JA_Bye\n"
	if got := out.String(); got != want {
		t.Errorf("output with the report off:\n%s\nwant:\n%s", got, want)
	}

	// Turned back on, the next run reports only its own writes.
	out.Reset()
	e.SetSourceFileReport(true)
	if err := e.ExecuteProgram(buildProgram(t, "create java action Probe.JA_Third () returns String as $$\nreturn \"3\";\n$$;\n")); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); strings.Contains(got, "JA_Hello") || strings.Contains(got, "JA_Bye") ||
		!strings.Contains(got, "Files changed outside the model: 1 (javasource/)") {
		t.Errorf("a later run reported writes the silenced run made:\n%s", got)
	}
}

func TestSourceFileChangeLines(t *testing.T) {
	for _, c := range []struct {
		change backend.SourceFileChange
		want   string
	}{
		{backend.SourceFileChange{Path: "javascriptsource/probe/actions/JS_Hello.js", What: "JavaScript action body"},
			"Wrote javascriptsource/probe/actions/JS_Hello.js (JavaScript action body; commit it with the model)"},
		{backend.SourceFileChange{Path: "javasource/probe/actions/JA_New.java", From: "javasource/probe/actions/JA_Old.java", What: "Java action body"},
			"Moved javasource/probe/actions/JA_Old.java to javasource/probe/actions/JA_New.java (Java action body; commit both paths with the model)"},
		{backend.SourceFileChange{Path: "javasource/probe/", Removed: true, What: "module Java source"},
			"Removed javasource/probe/ (module Java source; commit the deletion with the model)"},
	} {
		if got := sourceFileChangeLine(c.change); got != c.want {
			t.Errorf("%+v:\n got %q\nwant %q", c.change, got, c.want)
		}
	}
}

func TestSourceFileSummaryCountsPathsOnceAndNamesEveryFolder(t *testing.T) {
	var log sourceFileLog
	log.begin()
	log.add([]backend.SourceFileChange{
		{Path: "javasource/probe/actions/JA_Hello.java"},
		{Path: "javascriptsource/probe/actions/JS_Hello.js"},
		{Path: "javasource/probe/actions/JA_Hello.java"}, // written twice in one run
		{Path: "themesource/probe/", Removed: true},
	})
	var out bytes.Buffer
	log.flush(&out)
	want := "Files changed outside the model: 3 (javasource/, javascriptsource/, themesource/). Stage them with the .mpr.\n"
	if got := out.String(); got != want {
		t.Errorf("summary %q, want %q", got, want)
	}
	out.Reset()
	log.flush(&out)
	if out.Len() != 0 {
		t.Errorf("a second flush repeated the summary: %q", out.String())
	}
}
