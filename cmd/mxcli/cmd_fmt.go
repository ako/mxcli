// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	modelsdkbackend "github.com/mendixlabs/mxcli/mdl/backend/modelsdk"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/formatter"
	"github.com/mendixlabs/mxcli/mdl/langver"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/upgrade"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [file.mdl... | -]",
	Short: "Format an MDL file",
	Long: `Format an MDL script file with consistent styling:
  - Lowercase MDL keywords, the canonical case. Only words the parse tree shows
    are keywords change: a name spelled like a keyword (Issue64.User, an
    attribute Title), a property key (Folder:) and a CamelCase value
    (ButtonStyle: Success) keep their case, and so do expressions, XPath, OQL
    and SQL, which are stored as written. Formatting never changes what a
    script builds.
  - Normalize indentation (2-space units)
  - Remove trailing whitespace
  - Normalize blank lines

Pass '-' (or omit the argument) to read from stdin.

Examples:
  # Format to stdout
  mxcli fmt script.mdl

  # Format in-place
  mxcli fmt script.mdl -w

  # Format from stdin (pipe)
  mxcli describe microflow Mod.MF | mxcli fmt
  mxcli describe microflow Mod.MF | mxcli fmt -

Upgrading (--upgrade):
  Rewrites every deprecated spelling registered in the deprecation registry to
  its canonical form (create or replace -> create or modify, show -> list, ...)
  and changes nothing else: comments, layout and keyword case are kept, and the
  heuristic formatting above is not applied. A deprecated use that has no
  mechanical rewrite is reported on stderr and left in place.

  It also adds the language header (mdl 1;), after rewriting every construct
  whose meaning the header would change. A construct without such a rewrite
  blocks the header, and fmt fails rather than change the script's meaning.
  --header=false upgrades the spellings alone and adds no header. Plain fmt
  (without --upgrade) never adds a header: a headerless script is mdl 0, and
  only the upgrade knows how to keep its meaning under mdl 1.

  With -p app.mpr, the project the script runs against answers what the
  script cannot: find(…) / contains(…) over the result of a microflow or
  nanoflow call is the string function when the called flow returns a String
  and the List operation otherwise, and a flow the script does not create
  before the call is looked up in the project. Without a project such a call
  blocks the header and fmt says so. The project is only read.

  The project also says which statements exec would refuse once the header
  is there: a "create or modify" of a stored flow whose change cannot be
  spliced in is refused under mdl 1 (MDL-V1-REBUILD), where the headerless
  script rebuilt the flow. No rewrite keeps that meaning, so fmt names each
  such statement and declines the header for the file, applying the rest of
  the upgrade; --force-header adds it anyway. "check -p" reports the same
  statements, computed by the same code.

  The project also settles a bare commit (with or without --header): since
  #895 "commit $X;" means WITH events, and an older mxcli stored the same
  statement without events. In a "create or modify" flow whose stored flow
  commits the variable without events, --upgrade -p writes
  "commit $X without events;" so that re-running the script keeps what is
  stored. Without -p the script is left as written and fmt prints a note
  (MDL067) for each flow with a bare commit.

  Several files are upgraded as one script set, run in the order given. When
  two of their "create or modify" statements declare the same flow — a
  placeholder ("stub") followed by the real flow — fmt reports the pair
  (MDL-STUB01) and decides the header for the files that share the flow
  together: all of them take it, or none does. Decided file by file, the stub
  could stay mdl 0 (exec would refuse its change to the stored real flow)
  while the real flow's file took the header; run in order, the stub then
  rebuilt the real flow on every run and the mdl 1 real statement was
  refused. Since #843 a self-recursive flow is created in one statement, so
  the better fix is to drop the stub.

  A test file (.test.mdl, .test.md) is upgraded the way check reads it: the
  statements in its blocks are rewritten, and its doc comments (@test,
  @expect, …), separators and prose are kept byte for byte. The header is
  added as for a script — before the first line of a .test.mdl, as the first
  line of each mdl-test block of a .test.md — since check and the runner read
  a test file's header; --header=false upgrades the spellings without it.

  # Upgrade in place
  mxcli fmt --upgrade -w script.mdl
  mxcli fmt --upgrade --header -w script.mdl
  mxcli fmt --upgrade --header -w -p app.mpr script.mdl
  mxcli fmt --upgrade -w -p app.mpr scripts/*.mdl
`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		doUpgrade, _ := cmd.Flags().GetBool("upgrade")
		addHeader, _ := cmd.Flags().GetBool("header")
		if cmd.Flags().Changed("header") && !doUpgrade {
			return fmt.Errorf("--header needs --upgrade")
		}
		if force, _ := cmd.Flags().GetBool("force-header"); force && (!doUpgrade || cmd.Flags().Changed("header") && !addHeader) {
			return fmt.Errorf("--force-header needs --upgrade, with the header")
		}
		if len(args) > 1 {
			return fmtScriptSet(cmd, args)
		}
		return fmtFile(cmd, args, "")
	},
}

// fmtFile formats or upgrades one script: args holds its path, or nothing or
// "-" for stdin. declineHeader, when set, keeps the language header off the
// file and says why: the header of a script set's stub-then-real pair is
// decided for its files together (ako/mxcli#905).
func fmtFile(cmd *cobra.Command, args []string, declineHeader string) error {
	writeInPlace, _ := cmd.Flags().GetBool("write")
	doUpgrade, _ := cmd.Flags().GetBool("upgrade")
	addHeader, _ := cmd.Flags().GetBool("header")

	// Determine source: stdin when no arg or "-" is passed.
	fromStdin := len(args) == 0 || args[0] == "-"
	filePath := ""
	if !fromStdin {
		filePath = args[0]
	}

	if writeInPlace && fromStdin {
		return fmt.Errorf("-w cannot be used with stdin")
	}

	var data []byte
	var err error
	if fromStdin {
		data, err = readMDLSource(stdinPath)
	} else {
		data, err = readMDLSource(filePath)
	}
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	label := filePath
	if fromStdin {
		label = "<stdin>"
	}

	// A .test.mdl / .test.md file is not top-level MDL: its blocks are
	// microflow bodies behind `/** @test … */` doc comments. --upgrade reads
	// it the way check does (ako/mxcli#837); the layout formatter does not
	// know the format, so it is not let loose on one.
	if !fromStdin && testrunner.IsTestFile(filePath) {
		if !doUpgrade {
			return fmt.Errorf("%s is a test file: fmt formats top-level MDL scripts, and would not keep a test "+
				"file's doc comments and separators; use `mxcli fmt --upgrade` to upgrade its statements", label)
		}
		// The header is added as for a script (ako/mxcli#847): the runner
		// and check read a test file's `mdl <n>;` line now.
		opts := upgrade.DefaultOptions()
		if cmd.Flags().Changed("header") {
			opts.AddHeader = addHeader
		}
		if declineHeader != "" {
			opts.AddHeader = false
		}
		res, err := testrunner.UpgradeSource(string(data), filePath, opts)
		if err != nil {
			var blocked *upgrade.HeaderBlockedError
			if errors.As(err, &blocked) && !cmd.Flags().Changed("header") {
				return fmt.Errorf("%s: %w\n`mxcli fmt --upgrade --header=false` upgrades the spellings without the header", label, err)
			}
			return fmt.Errorf("%s: %w", label, err)
		}
		reportUpgrade(cmd.ErrOrStderr(), label, res)
		if declineHeader != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: no language header added: %s\n", label, declineHeader)
		}
		return writeFmtResult(cmd, filePath, writeInPlace, string(data), res.Source, true)
	}

	// Reject unparseable input so automation scripts can detect failures.
	// Two failure modes:
	//   1. ANTLR reports explicit parse errors (structural violations).
	//   2. ANTLR silently skips unrecognised tokens — detected when no
	//      statements were produced from non-blank, non-comment content.
	prog, errs := visitor.Build(string(data))
	if len(errs) > 0 {
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Error())
		}
		return fmt.Errorf("syntax errors in %s:\n%s", label, strings.Join(msgs, "\n"))
	}
	if prog != nil && len(prog.Statements) == 0 && hasSubstantiveContent(string(data)) {
		return fmt.Errorf("no valid MDL statements found in %s", label)
	}

	var formatted string
	if doUpgrade {
		opts := upgrade.DefaultOptions()
		if cmd.Flags().Changed("header") {
			opts.AddHeader = addHeader
		}
		if declineHeader != "" {
			opts.AddHeader = false
		}
		closeProject, err := openUpgradeProject(cmd, &opts)
		if err != nil {
			return err
		}
		defer closeProject()
		res, err := upgrade.Upgrade(string(data), opts)
		if err != nil {
			// The header is the default since the freeze (ako/mxcli#714):
			// name the way to upgrade the spellings without it, which a
			// plain `fmt --upgrade` did before.
			var blocked *upgrade.HeaderBlockedError
			if errors.As(err, &blocked) && !cmd.Flags().Changed("header") {
				return fmt.Errorf("%s: %w\n`mxcli fmt --upgrade --header=false` upgrades the spellings without the header", label, err)
			}
			return fmt.Errorf("%s: %w", label, err)
		}
		if res.HeaderAdded {
			if res, err = declineRefusedHeader(cmd, label, string(data), opts, res); err != nil {
				return err
			}
		}
		reportUpgrade(cmd.ErrOrStderr(), label, res)
		if declineHeader != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "%s: no language header added: %s\n", label, declineHeader)
		}
		formatted = res.Source
	} else {
		formatted = formatter.Format(string(data))
	}

	return writeFmtResult(cmd, filePath, writeInPlace, string(data), formatted, doUpgrade)
}

// openUpgradeProject opens the -p project read-only for the upgrade to read
// from: the flow return types a header-gated `find(…)` depends on
// (ako/mxcli#860), and the stored commit flags a bare `commit` is pinned to
// (ako/mxcli#873). With no project it sets neither, and the constructs that
// need one block the header or are reported, as before.
func openUpgradeProject(cmd *cobra.Command, opts *upgrade.Options) (func(), error) {
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath == "" {
		return func() {}, nil
	}
	b := modelsdkbackend.New()
	if err := b.ConnectReadOnly(projectPath); err != nil {
		return nil, fmt.Errorf("cannot read the project %s, which --upgrade reads flows from: %w", projectPath, err)
	}
	if opts.AddHeader {
		opts.Flows = executor.NewFlowReturnTypes(b)
	}
	opts.Commits = executor.NewStoredCommitEvents(b)
	return func() { _ = b.Disconnect() }, nil
}

// declineRefusedHeader keeps the language header off a script that exec would
// stop executing once it has one (ako/mxcli#876). Under mdl 1 a `create or
// modify` of a stored flow whose change cannot be spliced in is refused, where
// mdl 0 rebuilt the flow; no rewrite of the script can say "rebuild", so the
// upgrade cannot keep its meaning. With -p the upgraded script is checked
// against the project the way `check -p` does, by the verdict exec acts on;
// each refusal is reported and the header is declined for the file — the rest
// of the upgrade still applies. --force-header adds it anyway. Without -p the
// project is unknown and nothing is predicted.
func declineRefusedHeader(cmd *cobra.Command, label, src string, opts upgrade.Options, res upgrade.Result) (upgrade.Result, error) {
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath == "" {
		return res, nil
	}
	refusals, err := headerRefusals(projectPath, res.Source)
	if err != nil {
		return res, fmt.Errorf("%s: cannot check the upgraded script against %s: %w", label, projectPath, err)
	}
	if len(refusals) == 0 {
		return res, nil
	}
	w := cmd.ErrOrStderr()
	force, _ := cmd.Flags().GetBool("force-header")
	verb := "no language header added"
	if force {
		verb = "language header added anyway (--force-header)"
	}
	fmt.Fprintf(w, "%s: %s: under mdl 1 exec would refuse %d statement(s), writing nothing, where the script "+
		"without the header rebuilds the flow (MDL-V1-REBUILD):\n", label, verb, len(refusals))
	for _, v := range refusals {
		fmt.Fprintf(w, "  %s %s: %s\n", v.Location.DocumentType, v.Location.QualifiedName(), v.Message)
	}
	if force {
		return res, nil
	}
	fmt.Fprintf(w, "%s: change those flows with `alter`, or drop and create them, then upgrade again; "+
		"--force-header adds the header regardless\n", label)
	opts.AddHeader = false
	return upgrade.Upgrade(src, opts)
}

// headerRefusals returns what exec would refuse in the upgraded script src
// because of its header: the MDL-V1-REBUILD errors check reports for it
// against the project (executor.CheckFlowVerdicts).
func headerRefusals(projectPath, src string) ([]linter.Violation, error) {
	prog, errs := visitor.Build(src)
	if len(errs) > 0 {
		return nil, errs[0]
	}
	exec := executor.New(io.Discard)
	exec.SetBackendFactory(newBackendFactory())
	defer exec.Close()
	connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
	for _, stmt := range connectProg.Statements {
		if err := exec.Execute(stmt); err != nil {
			return nil, err
		}
	}
	var out []linter.Violation
	for _, v := range exec.CheckFlowVerdicts(prog) {
		if v.RuleID == executor.FlowRebuildRule && v.Severity == linter.SeverityError {
			out = append(out, v)
		}
	}
	return out, nil
}

// writeFmtResult writes fmt's output: in place with -w, else to stdout. An
// upgrade that changed nothing leaves the file untouched.
func writeFmtResult(cmd *cobra.Command, filePath string, writeInPlace bool, original, formatted string, upgraded bool) error {
	if !writeInPlace {
		fmt.Print(formatted)
		return nil
	}
	if upgraded && formatted == original {
		return nil // nothing to upgrade: leave the file untouched
	}
	if err := os.WriteFile(filePath, []byte(formatted), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	verb := "Formatted"
	if upgraded {
		verb = "Upgraded"
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "%s %s\n", verb, filePath)
	return nil
}

func init() {
	fmtCmd.Flags().BoolP("write", "w", false, "Write result to source file instead of stdout")
	fmtCmd.Flags().Bool("upgrade", false, "Rewrite deprecated spellings to their canonical form, changing nothing else")
	fmtCmd.Flags().Bool("header", true, "With --upgrade: add the mdl 1 language header (the default; --header=false declines it)")
	fmtCmd.Flags().Bool("force-header", false, "With --header -p: add the header even where exec would then refuse a statement")
}

// reportUpgrade prints what an upgrade did, and what it left, on w.
func reportUpgrade(w io.Writer, label string, res upgrade.Result) {
	codes := func(m map[string]int) string {
		var parts []string
		for c, n := range m {
			parts = append(parts, fmt.Sprintf("%s x%d", c, n))
		}
		sort.Strings(parts)
		return strings.Join(parts, ", ")
	}
	if len(res.Rewritten) > 0 {
		fmt.Fprintf(w, "%s: rewrote %s\n", label, codes(res.Rewritten))
	}
	if len(res.GatedRewritten) > 0 {
		fmt.Fprintf(w, "%s: rewrote for the header %s\n", label, codes(res.GatedRewritten))
	}
	if res.HeaderAdded {
		fmt.Fprintf(w, "%s: added the language header\n", label)
	}
	if res.CommitsPinned > 0 {
		fmt.Fprintf(w, "%s: stated `without events` on %d bare commit(s), as the project's stored flows have them (MDL067)\n",
			label, res.CommitsPinned)
	}
	for _, n := range res.Notes {
		fmt.Fprintf(w, "%s:%d: note: %s\n", label, n.Line, n.Message)
	}
	for _, d := range res.Unrewritten {
		msg := d.Code
		if e, ok := deprecation.Lookup(d.Code); ok {
			msg = fmt.Sprintf("%s (%s -> %s)", d.Code, e.Old, e.Canonical)
		}
		if d.NoFix != "" {
			msg += ": " + d.NoFix
		}
		msg += " " + langver.HelpHint(d.Code)
		fmt.Fprintf(w, "%s:%d:%d: not upgraded, no mechanical rewrite: %s\n", label, d.Line, d.Column+1, msg)
	}
}

// hasSubstantiveContent reports whether s contains at least one non-blank,
// non-comment line — used to distinguish empty/comment-only files (which
// produce zero statements legitimately) from garbage input.
func hasSubstantiveContent(s string) bool {
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "--") && !langver.IsHeaderLine(t) {
			return true
		}
	}
	return false
}

// fmtScriptSet formats or upgrades several scripts, each as fmtFile would one,
// and returns every file's error. With --upgrade the files are read as one set
// first (ako/mxcli#905): a flow two of its `create or modify` statements
// declare — the stub-then-real pattern — is reported (MDL-STUB01), and the
// language header of the files sharing such a flow is decided for them
// together: added to all of them or to none. Decided file by file, the stub's
// file was declined the header (exec would refuse its change to the stored
// real flow) while the real flow's file took it; run in order, the mdl 0 stub
// then rebuilt the real flow and the mdl 1 real statement was refused, on
// every run, with mx check clean.
func fmtScriptSet(cmd *cobra.Command, files []string) error {
	for _, f := range files {
		if f == stdinPath {
			return fmt.Errorf("'-' (stdin) is one script; it cannot be one of several files")
		}
	}
	decline := map[string]string{}
	if doUpgrade, _ := cmd.Flags().GetBool("upgrade"); doUpgrade {
		scripts := parseScriptSet(files)
		redecls := findFlowRedeclarations(scripts)
		w := cmd.ErrOrStderr()
		for _, v := range stubThenRealViolations(redecls) {
			fmt.Fprintf(w, "warning: %s [%s]\n", v.Message, v.RuleID)
		}
		if addHeader, _ := cmd.Flags().GetBool("header"); addHeader && len(redecls) > 0 {
			decline = decideSetHeaders(cmd, scripts, redecls)
		}
	}
	var errs []error
	for _, f := range files {
		if err := fmtFile(cmd, []string{f}, decline[f]); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// decideSetHeaders returns, for each file that has to stay off the header
// because a file it shares a redeclared flow with cannot take it, the reason
// to print. A group whose files can all take the header, or none of them, is
// left to the per-file decision, which then agrees.
func decideSetHeaders(cmd *cobra.Command, scripts []setScript, redecls []flowRedeclaration) map[string]string {
	src := map[string]string{}
	var files []string
	for _, sc := range scripts {
		src[sc.Path] = sc.Source
		files = append(files, sc.Path)
	}
	decline := map[string]string{}
	for _, group := range fileGroups(files, redecls) {
		var can, cannot []string
		for _, f := range group {
			if canTakeHeader(cmd, src[f]) {
				can = append(can, f)
			} else {
				cannot = append(cannot, f)
			}
		}
		if len(can) == 0 || len(cannot) == 0 {
			continue
		}
		for _, f := range can {
			// fmt never removes a written header: a file already under it
			// cannot be held back, and the pair stays split until the stub
			// goes or the header is taken off by hand.
			if v, written := langver.ScanWrittenHeader(src[f]); written {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: already has the %s header, which fmt does not remove, while %s "+
					"cannot take it: the stub-then-real pair stays under different headers (%s); drop the stub, "+
					"or take the header off this file\n", f, v, strings.Join(cannot, ", "), StubThenRealRule)
				continue
			}
			decline[f] = fmt.Sprintf("it declares a flow that %s also declares, and that file cannot take the "+
				"header; a stub-then-real pair under different headers lets the mdl 0 statement rebuild the flow "+
				"on every run while the mdl 1 one is refused, so the header is decided for the files together "+
				"(%s)", strings.Join(cannot, ", "), StubThenRealRule)
		}
	}
	return decline
}

// canTakeHeader reports whether fmt --upgrade, run on src alone, would leave
// it under the language header: it has it written already, or the upgrade can add
// one (no construct blocks it) and, with -p, exec would refuse none of its
// statements under it (or --force-header overrides that).
func canTakeHeader(cmd *cobra.Command, src string) bool {
	// A written header is the file's version: `mdl 0;` pins it, and
	// --upgrade leaves it there.
	if v, written := langver.ScanWrittenHeader(src); written {
		return v == langver.Latest
	}
	opts := upgrade.DefaultOptions()
	opts.AddHeader = true
	closeProject, err := openUpgradeProject(cmd, &opts)
	if err != nil {
		return false
	}
	defer closeProject()
	res, err := upgrade.Upgrade(src, opts)
	if err != nil {
		return false
	}
	projectPath, _ := cmd.Flags().GetString("project")
	if !res.HeaderAdded || projectPath == "" {
		return true
	}
	if force, _ := cmd.Flags().GetBool("force-header"); force {
		return true
	}
	refusals, err := headerRefusals(projectPath, res.Source)
	return err == nil && len(refusals) == 0
}
