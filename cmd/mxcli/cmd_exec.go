// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec <file|->",
	Short: "Execute an MDL script file",
	Long: `Execute an MDL script file containing MDL commands.

Before anything is written, the script is put through the same semantic checks
as "mxcli check". If any of them reports an error, nothing is executed: exec
applies statements one at a time and cannot roll back, so running a script with
a known error leaves the model partly updated. Warnings are printed and do not
stop the run. Use --no-check to apply a script anyway.

By default execution stops at the first error. With --continue-on-error, every
statement is attempted; each failure is reported (prefixed with its statement
number) and execution continues, exiting non-zero if any statement failed. This
makes a partially-applied domain script re-runnable — the already-applied
statements (e.g. "attribute already exists") error individually while the not-
yet-applied ones still run — without a failure masking later work.

Pass "-" as the file to read the script from standard input, so MDL can be
piped or written inline as a heredoc without a temporary file.

Example:
  mxcli exec setup.mdl
  mxcli exec -p app.mpr script.mdl
  mxcli exec -p app.mpr script.mdl --continue-on-error
  mxcli exec -p app.mpr script.mdl --no-check
  mxcli exec -p app.mpr - <<'EOF'
  SHOW STRUCTURE DEPTH 1;
  EOF
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		projectPath, _ := cmd.Flags().GetString("project")
		continueOnError, _ := cmd.Flags().GetBool("continue-on-error")
		skipCheck, _ := cmd.Flags().GetBool("no-check")

		// Read the script (a path, or "-" for stdin)
		content, err := readMDLSource(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		exec, logger := newLoggedExecutor("exec")
		defer logger.Close()
		defer exec.Close()

		// A relative path inside the script (a toolbox icon, an image) names a
		// file next to the script, not next to the caller. Empty for stdin.
		if filePath != "-" {
			if abs, absErr := filepath.Abs(filePath); absErr == nil {
				exec.SetScriptDir(filepath.Dir(abs))
			}
		}

		// Auto-connect if project specified
		if projectPath != "" {
			connectCmd := fmt.Sprintf("CONNECT LOCAL '%s';", visitor.QuoteString(projectPath))
			prog, _ := visitor.Build(connectCmd)
			for _, stmt := range prog.Statements {
				if err := exec.Execute(stmt); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
			}
		}

		// Parse and execute the file
		prog, errs := visitor.Build(string(content))
		if len(errs) > 0 {
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "Parse error: %v\n", err)
			}
			os.Exit(1)
		}
		// "Apply this file" that applies nothing is never what was meant, and a
		// silent no-op is the worst outcome for a replayable mdlsource/
		// (ako/mxcli#618).
		if line, bad := unparsableInput(string(content), len(prog.Statements)); bad {
			fmt.Fprintln(os.Stderr, unparsableInputError(filePath, line))
			os.Exit(1)
		}

		// Pre-flight: refuse a script whose semantic checks report an error,
		// rather than writing part of it and leaving the model to mxbuild.
		// exec is not transactional, so "run it and see" means a half-applied
		// model. Warnings are printed and do not stop the run.
		if !skipCheck {
			violations := executor.ValidateProgram(prog, projectPath)
			if len(violations) > 0 {
				formatter := linter.GetFormatter(linter.OutputFormatText, true)
				formatter.Format(violations, os.Stderr)
			}
			if summary := linter.Summarize(violations); summary.Errors > 0 {
				fmt.Fprintf(os.Stderr,
					"\nRefusing to execute: %d error(s) above. Nothing was written.\n"+
						"  exec applies statements one at a time and cannot roll back, so a script\n"+
						"  with a known error would leave the model partly updated.\n"+
						"  Fix them, or re-run with --no-check to apply the script anyway.\n",
					summary.Errors)
				os.Exit(1)
			}
		}

		// Second preflight pass: resolve every NAME against the connected
		// project. The semantic pass above cannot do this — a missing module,
		// entity, page or microflow needs a backend, not a path — so `mxcli
		// check -p` ran it and `exec` did not (#607).
		//
		// MEASURED on the expr-checker fixture, running the pre-fix binary
		// (`--no-check` reproduces it), because the failure mode is not the one
		// the refusal above describes and the difference matters:
		//
		//   create entity "NotAModule"."Thing"     -> exit 0, "Created module:
		//                                             NotAModule". A misspelled
		//                                             module is SILENTLY CREATED.
		//   microflow retrieving a missing entity  -> exit 0, both documents
		//                                             written. The dangling name
		//                                             reaches the model and is
		//                                             not reported until mxbuild
		//                                             rejects it (CE1613).
		//
		// So exec did not half-apply here — it completed, and wrote a model that
		// only a 25s build would reject. That makes this a check-to-build parity
		// fix (moving a build-tier error to the 2s tier) and a fix for the
		// "no silent side effects on typos" rule in CLAUDE.md's checklist, which
		// auto-creating a module on a misspelling violates outright.
		//
		// Safe to refuse on, because the pass skips references to objects the
		// script itself creates: an error from it means the name resolves to
		// nothing in the project AND is not created here, so exec would have
		// failed on it regardless — later, and after writing.
		//
		// Only possible with -p. A script that connects with its own CONNECT
		// statement has no backend until ExecuteProgram runs, which is the same
		// condition `check` gates this on.
		//
		// CheckProjectConflicts is deliberately NOT run here, though `check`
		// runs it alongside this pass: a plain CREATE over an existing document
		// is worth reporting when validating a script, but it is ordinary for a
		// re-run, and refusing it would break scripts that work today.
		if !skipCheck && projectPath != "" {
			if refErrs := exec.ValidateProgram(prog); len(refErrs) > 0 {
				for _, refErr := range refErrs {
					fmt.Fprintf(os.Stderr, "Reference error: %v\n", refErr)
				}
				fmt.Fprintf(os.Stderr,
					"\nRefusing to execute: %d unresolved reference(s) above. Nothing was written.\n"+
						"  A name that resolves to nothing is written into the model as it stands and\n"+
						"  is not reported until mxbuild rejects it (CE1613) — and a misspelled MODULE\n"+
						"  is created rather than refused.\n"+
						"  Fix them, or re-run with --no-check to apply the script anyway.\n",
					len(refErrs))
				os.Exit(1)
			}
		}

		if continueOnError {
			res, err := exec.ExecuteProgramContinueOnError(prog, os.Stderr)
			if err != nil && !errors.Is(err, executor.ErrExit) {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "%d statements: %d succeeded, %d failed\n", res.Total, res.Succeeded, res.Failed)
			if res.Failed > 0 {
				os.Exit(1)
			}
			return
		}

		if err := exec.ExecuteProgram(prog); err != nil {
			if errors.Is(err, executor.ErrExit) {
				return
			}
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	execCmd.Flags().Bool("no-check", false,
		"Skip the pre-flight semantic checks and apply the script even if mxcli check would report errors")
	execCmd.Flags().Bool("continue-on-error", false,
		"Run every statement, reporting each failure instead of halting at the first (exits non-zero if any failed) — makes a partially-applied script re-runnable")
}
