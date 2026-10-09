// SPDX-License-Identifier: Apache-2.0

// Package scriptdiff answers "what would `mxcli exec` write?" by running exec.
//
// `mxcli diff` used to answer it a second way: render each statement as MDL,
// render the stored document as MDL, and compare the text. Every place the two
// renderings disagreed without exec disagreeing was a phantom change (`Boolean`
// against the stored `Boolean default false`, `String` against
// `String(unlimited)`, a position, a re-laid-out flow), every document kind it
// had no renderer for was not compared at all (pages, translations, a layout
// repointed), and it never ran a statement, so it neither saw what earlier
// statements create nor refused what exec refuses (ako/mxcli#907, #807, #856).
//
// Here the script is executed — by exec's own code, under its own language
// header — against a scratch copy of the project, and the copy is compared with
// the project unit by unit. The units that differ are, by construction, the
// units exec would write: there is no second verdict to keep in step with the
// first. DESCRIBE renders each one on both sides so the change can be read as
// MDL.
package scriptdiff

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/langver"
)

// Options configures a run.
type Options struct {
	// NewBackend opens the scratch copy. It must be a file engine: the script is
	// executed for real, and a backend that writes somewhere other than the
	// copy (Studio Pro over MCP) would apply it.
	NewBackend func() backend.FullBackend
	// ScriptDir is the directory of the script, which relative paths inside it
	// name (exec's SetScriptDir).
	ScriptDir string
	// DescribeLanguage is the MDL the changes are rendered in; nil is describe's
	// default.
	DescribeLanguage *langver.Version
	// ContinueOnError runs every statement as `exec --continue-on-error` does,
	// instead of stopping at the first error.
	ContinueOnError bool
	// Preflight, when set, runs exec's pre-flight checks against the scratch
	// executor before anything is executed, writing its report to w, and
	// returns why exec would refuse the script ("" when it would run it).
	Preflight func(scratch *executor.Executor, w io.Writer) string
}

// Report is what exec would do.
type Report struct {
	// Refused is why exec's pre-flight would refuse the script, writing
	// nothing; "" when it would run it.
	Refused string
	// PreflightOutput is what the pre-flight printed.
	PreflightOutput string
	// ExecErr is the error exec would stop on (or, with ContinueOnError, the
	// error that ended the run). Units still lists what it wrote before it.
	ExecErr error
	// Failures are the statement failures a ContinueOnError run reports.
	Failures string
	// Output is what exec printed.
	Output string
	// Units are the units exec would add, rewrite, move or remove.
	Units []UnitChange
	// Files are the files next to the model it would write or delete.
	Files []FileChange
	// Results render Units as MDL, an entry per document (per entity and
	// association for a domain model).
	Results []executor.DiffResult
}

// Run executes prog against a scratch copy of the project at mprPath and
// reports what it wrote there. The project at mprPath is only read.
func Run(mprPath string, prog *ast.Program, opts Options) (*Report, error) {
	if opts.NewBackend == nil {
		return nil, errors.New("scriptdiff: no backend to open the scratch copy with")
	}
	scratch, err := NewScratch(mprPath)
	if err != nil {
		return nil, err
	}
	defer scratch.Close()
	src, copyMpr := scratch.src, scratch.Mpr

	before, err := TakeSnapshot(copyMpr)
	if err != nil {
		return nil, err
	}

	rep := &Report{}
	var out bytes.Buffer
	x := newExecutor(&out, opts)
	if err := x.Execute(&ast.ConnectStmt{Path: copyMpr}); err != nil {
		x.Close()
		return nil, fmt.Errorf("connect to the scratch copy: %w", err)
	}
	g := &outsideGuard{project: src, copyMpr: copyMpr}
	x.SetStatementGuard(g.check)
	if opts.Preflight != nil {
		var pf bytes.Buffer
		rep.Refused = opts.Preflight(x, &pf)
		rep.PreflightOutput = pf.String()
	}
	if rep.Refused == "" {
		if opts.ContinueOnError {
			var fails bytes.Buffer
			_, rep.ExecErr = x.ExecuteProgramContinueOnError(prog, &fails)
			rep.Failures = fails.String()
		} else {
			rep.ExecErr = x.ExecuteProgram(prog)
		}
		if errors.Is(rep.ExecErr, executor.ErrExit) {
			rep.ExecErr = nil
		}
	}
	_ = x.Execute(&ast.DisconnectStmt{})
	x.Close()
	rep.Output = out.String()
	if g.refused != nil {
		return nil, g.refused
	}

	after, err := TakeSnapshot(copyMpr)
	if err != nil {
		return nil, err
	}
	rep.Units, rep.Files = before.Compare(after)
	if len(rep.Units) > 0 {
		r, err := newRenderer(src, copyMpr, before, after, opts)
		if err != nil {
			return nil, err
		}
		rep.Results = r.render(rep.Units)
		r.close()
	}
	return rep, nil
}

// Scratch is a scratch copy of a project that scripts can be executed against
// in turn, each seeing what the ones before it wrote — the state exec leaves
// for the next file of a script set. The project it was copied from is only
// read.
type Scratch struct {
	dir string // the scratch folder, removed by Close
	src string // the project's .mpr, absolute
	// Mpr is the copy's .mpr.
	Mpr string
}

// NewScratch copies the project at mprPath into a new scratch folder.
func NewScratch(mprPath string) (*Scratch, error) {
	src, err := filepath.Abs(mprPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(src); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "mxcli-diff-")
	if err != nil {
		return nil, fmt.Errorf("create scratch folder: %w", err)
	}
	copyDir := filepath.Join(dir, "project")
	if err := copyProject(filepath.Dir(src), copyDir); err != nil {
		os.RemoveAll(dir)
		return nil, fmt.Errorf("copy the project to %s: %w", copyDir, err)
	}
	return &Scratch{dir: dir, src: src, Mpr: filepath.Join(copyDir, filepath.Base(src))}, nil
}

// Close removes the scratch folder.
func (s *Scratch) Close() error { return os.RemoveAll(s.dir) }

// Apply executes prog against the copy as exec would — stopping at the first
// error — and returns what exec printed. A statement that would act outside
// the copy (a CONNECT elsewhere, SQL, IMPORT) is refused, and the refusal
// returned, as in Run. Options.Preflight and ContinueOnError are not used.
func (s *Scratch) Apply(prog *ast.Program, opts Options) (string, error) {
	if opts.NewBackend == nil {
		return "", errors.New("scriptdiff: no backend to open the scratch copy with")
	}
	var out bytes.Buffer
	x := newExecutor(&out, opts)
	defer x.Close()
	if err := x.Execute(&ast.ConnectStmt{Path: s.Mpr}); err != nil {
		return out.String(), fmt.Errorf("connect to the scratch copy: %w", err)
	}
	g := &outsideGuard{project: s.src, copyMpr: s.Mpr}
	x.SetStatementGuard(g.check)
	err := x.ExecuteProgram(prog)
	_ = x.Execute(&ast.DisconnectStmt{})
	if g.refused != nil {
		return out.String(), g.refused
	}
	if errors.Is(err, executor.ErrExit) {
		err = nil
	}
	return out.String(), err
}

func newExecutor(w io.Writer, opts Options) *executor.Executor {
	x := executor.New(w)
	x.SetBackendFactory(opts.NewBackend)
	if opts.ScriptDir != "" {
		x.SetScriptDir(opts.ScriptDir)
	}
	if opts.DescribeLanguage != nil {
		x.SetDescribeLanguage(*opts.DescribeLanguage)
	}
	return x
}

// outsideGuard keeps the script diff runs on the scratch copy from acting on
// anything but the copy. exec follows a CONNECT (a headerless script may hold
// one, and so may a script it executes) and runs SQL and IMPORT against real
// databases; run for real by diff, the first would write to the project being
// diffed — or another one — while the copy, and so the report, saw no write,
// and the others would change a database. A connect to the project itself is
// followed on the copy; anything else is refused, and Run reports it as an
// error rather than as a verdict of exec's.
type outsideGuard struct {
	project, copyMpr string
	refused          error
}

func (g *outsideGuard) check(stmt ast.Statement) (ast.Statement, error) {
	var err error
	switch s := stmt.(type) {
	case *ast.ConnectStmt:
		switch {
		case sameFile(s.Path, g.copyMpr):
			return stmt, nil
		case sameFile(s.Path, g.project):
			return &ast.ConnectStmt{Path: g.copyMpr}, nil
		}
		err = fmt.Errorf("the script connects to %s, outside the scratch copy of %s that diff runs it on; diff cannot show what exec would write there", s.Path, g.project)
	case *ast.SQLQueryStmt:
		err = fmt.Errorf("the script runs a SQL query on %q, a database outside the scratch copy diff runs it on; diff does not run it for real", s.Alias)
	case *ast.ImportStmt:
		err = fmt.Errorf("the script imports into %s's database, outside the scratch copy diff runs it on; diff does not run it for real", s.TargetEntity)
	default:
		return stmt, nil
	}
	if g.refused == nil {
		g.refused = err
	}
	return nil, err
}

func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(fa, fb)
}
