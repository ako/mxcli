// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check <file|->",
	Short: "Check an MDL script for errors without executing it",
	Long: `Check an MDL script file for syntax errors and optionally validate references.

Without a project it checks syntax and the semantic rules that need no model.
Pass -p and it also resolves every reference — modules, entities, pages,
microflows and icons — against that project; --references is implied by -p and
is kept only for compatibility.

Reference validation is smart: it automatically skips references to objects
that are created within the script itself. For example, if your script creates
a module "MyModule" and then creates entities in it, no error will be reported
for the module reference.

Output includes structured rule IDs (MDL prefix) for each validation issue.

Use --post-migration to scan an existing project (independent of the script)
for legacy native widgets that have pluggable replacements — Studio Pro does
not auto-migrate these on a Mendix major-version upgrade.

Examples:
  # Check syntax only (no project needed)
  mxcli check script.mdl

  # Check syntax and resolve references against a project
  mxcli check script.mdl -p app.mpr

  # Scan the project for legacy native widgets after a Mendix upgrade
  mxcli check script.mdl -p app.mpr --post-migration

  # Output as JSON or SARIF
  mxcli check script.mdl --format json
  mxcli check script.mdl --format sarif

  # Read the script from stdin
  cat script.mdl | mxcli check -
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		filePath := args[0]
		projectPath, _ := cmd.Flags().GetString("project")
		// A project makes reference resolution possible, so it runs. It used to
		// need --references as well, which meant `mxcli check script.mdl -p
		// app.mpr` printed an unqualified "Check passed!" having resolved
		// nothing — icons, entity and page references all silently unchecked.
		// Someone who hands the command a project has said what they want; the
		// flag stays accepted so existing invocations and scripts keep working.
		checkRefs, _ := cmd.Flags().GetBool("references")
		checkRefs = checkRefs || projectPath != ""
		postMigration, _ := cmd.Flags().GetBool("post-migration")
		format := resolveFormat(cmd, "text")
		isStructured := format != "" && format != "text"

		outputFormat := linter.OutputFormat(format)
		formatter := linter.GetFormatter(outputFormat, !isStructured)

		// Read the script (a path, or "-" for stdin)
		content, err := readMDLSource(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}

		// Parse the script
		if !isStructured {
			fmt.Printf("Checking syntax: %s\n", mdlSourceLabel(filePath))
		}
		prog, errs := visitor.Build(string(content))
		if len(errs) > 0 {
			if isStructured {
				var parseViolations []linter.Violation
				for _, parseErr := range errs {
					parseViolations = append(parseViolations, linter.Violation{
						RuleID:   "MDL-SYNTAX",
						Severity: linter.SeverityError,
						Message:  parseErr.Error(),
					})
				}
				formatter.Format(parseViolations, os.Stderr)
			} else {
				fmt.Fprintf(os.Stderr, "Syntax errors found:\n")
				for _, err := range errs {
					fmt.Fprintf(os.Stderr, "  - %v\n", err)
				}
				// Hint: if script contains IMPORT/QUERY with single $ but not $$, suggest dollar-quoting
				src := string(content)
				if (strings.Contains(src, "IMPORT") || strings.Contains(src, "import")) &&
					(strings.Contains(src, "QUERY") || strings.Contains(src, "query")) &&
					strings.Contains(src, "$") && !strings.Contains(src, "$$") {
					fmt.Fprintf(os.Stderr, "\nHint: SQL queries in IMPORT statements should use dollar-quoting ($$...$$) instead of single quotes.\n")
					fmt.Fprintf(os.Stderr, "  Example: IMPORT FROM alias QUERY $$SELECT * FROM table$$ INTO Module.Entity MAP (...)\n")
				}
			}
			os.Exit(1)
		}
		if !isStructured {
			fmt.Printf("✓ Syntax OK (%d statements)\n", len(prog.Statements))
		}

		// Every semantic check lives in executor.ValidateProgram, so `mxcli exec`
		// refuses exactly what `mxcli check` reports. Adding a check there gives
		// both commands it at once.
		violations := executor.ValidateProgram(prog, projectPath)

		if isStructured {
			// Always emit structured output (even when clean)
			formatter.Format(violations, os.Stderr)
		} else if len(violations) > 0 {
			fmt.Fprintln(os.Stderr)
			formatter.Format(violations, os.Stderr)
		}

		if len(violations) > 0 {
			summary := linter.Summarize(violations)
			if summary.Errors > 0 {
				os.Exit(1)
			}
		}

		// If reference checking requested
		if checkRefs {
			if projectPath == "" {
				fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for reference checking")
				os.Exit(1)
			}

			if !isStructured {
				fmt.Printf("\nValidating references against: %s\n", projectPath)
				fmt.Printf("(Note: References to objects created within the script are skipped)\n")
			}
			exec, logger := newLoggedExecutor("check")
			defer logger.Close()
			defer exec.Close()

			// Connect to project
			connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
			for _, stmt := range connectProg.Statements {
				if err := exec.Execute(stmt); err != nil {
					fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
					os.Exit(1)
				}
			}

			// Validate the program (considers objects defined within the script)
			validationErrors := exec.ValidateProgram(prog)

			// Check for project conflicts: plain CREATE where the document already exists
			validationErrors = append(validationErrors, exec.CheckProjectConflicts(prog)...)

			if len(validationErrors) > 0 {
				if isStructured {
					var refViolations []linter.Violation
					for _, err := range validationErrors {
						refViolations = append(refViolations, linter.Violation{
							RuleID:   "MDL-REF",
							Severity: linter.SeverityError,
							Message:  err.Error(),
						})
					}
					formatter.Format(refViolations, os.Stderr)
				} else {
					fmt.Fprintf(os.Stderr, "Reference errors:\n")
					for _, err := range validationErrors {
						fmt.Fprintf(os.Stderr, "  %v\n", err)
					}
					fmt.Fprintf(os.Stderr, "\n✗ %d reference error(s) found\n", len(validationErrors))
				}
				os.Exit(1)
			}
			if !isStructured {
				fmt.Printf("✓ All references valid\n")
			}
		}

		// Post-migration scan: walk the project for native widgets that
		// have pluggable replacements (Studio Pro does not auto-migrate
		// these on a Mendix major-version upgrade).
		if postMigration {
			if projectPath == "" {
				fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for --post-migration")
				os.Exit(1)
			}
			if !isStructured {
				fmt.Printf("\nScanning project for legacy native widgets: %s\n", projectPath)
			}
			legacyViolations, err := scanLegacyWidgets(projectPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error scanning project: %v\n", err)
				os.Exit(1)
			}
			if isStructured {
				formatter.Format(legacyViolations, os.Stderr)
			} else if len(legacyViolations) > 0 {
				fmt.Fprintln(os.Stderr)
				formatter.Format(legacyViolations, os.Stderr)
				fmt.Fprintf(os.Stderr, "\n✗ %d legacy widget(s) found\n", len(legacyViolations))
			} else {
				fmt.Printf("✓ No legacy native widgets found\n")
			}
			if len(legacyViolations) > 0 {
				summary := linter.Summarize(legacyViolations)
				if summary.Errors > 0 {
					os.Exit(1)
				}
			}
		}

		if !isStructured {
			fmt.Println("\nCheck passed!")
			// Qualify the verdict when nothing was resolved against a model. A
			// bare "Check passed!" reads as more than it is: without a project
			// no icon, entity, page or microflow name in the script has been
			// looked up, and those are exactly what this command gets reached
			// for. Saying so beats leaving the reader to infer it.
			if !checkRefs {
				fmt.Println("  (no project given — icon, entity, page and microflow references were")
				fmt.Println("   not resolved; re-run with -p <project.mpr> for full coverage)")
			}
		}
	},
}
