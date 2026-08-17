// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"

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
