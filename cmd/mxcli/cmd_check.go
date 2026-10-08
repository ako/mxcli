// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/testrunner"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var checkCmd = &cobra.Command{
	Use:   "check <file|-> [file...]",
	Short: "Check an MDL script for errors without executing it",
	Long: `Check an MDL script file for syntax errors and optionally validate references.

Without a project it checks syntax and the semantic rules that need no model.
Pass -p and it also resolves every reference — modules, entities, pages,
microflows and icons — against that project; --references is implied by -p and
is kept only for compatibility.

It also resolves MEMBER names, not just the entity they belong to: an attribute
named in a create/change activity is looked up on that entity and its
generalizations, so a typo is reported here rather than as CE1613 at the far end
of a build. This needs the target's entity to be known, which it is for a create
(the entity is in the statement) and for a change on a parameter, a database
retrieve, an association retrieve, or a loop over one of those. A variable bound
by some other activity is left unchecked rather than guessed at.

The same applies inside widgets: a page's XPath constraint has every step
resolved against the entity it filters, and a template parameter (ContentParams
/ CaptionParams) rooted in a variable is reported with no project at all.

Expression kinds are checked where the position declares one: a bare word as a
create/change member's value (Mendix expressions have no bare identifiers), and
a log message's template parameter, which must be a String — Mendix does not
coerce there, so wrap a non-String one in toString(...).

Reference validation is smart: it automatically skips references to objects
that are created within the script itself. For example, if your script creates
a module "MyModule" and then creates entities in it, no error will be reported
for the module reference.

Creation ORDER is checked separately, and without a project. The executor
resolves most references when it writes the referring document, so naming
something a later statement creates fails partway through "mxcli exec" — with
earlier statements already written, since exec is not transactional. Those are
reported as MDL-ORDER01 (and MDL-PAGE01 for a widget's page reference).

Given a project it also type-checks the expressions in the script's microflows
and nanoflows: comparing an enumeration attribute to a string literal (in a
create/change member, or in a condition such as: if $obj/Status = 'Open'),
operand and argument type mismatches, and the like. Attribute paths resolve
through associations too, so $Order/Sales.Order_Customer/Name is typed.
These need the project to answer what an attribute's type is and which values an
enumeration has, which is why they need -p. They report under exprcheck's own
E0xx codes, and — like every other check here — only an error severity fails the
run.

Given a project it also predicts what exec would refuse. A "create or modify"
of a microflow or nanoflow the project holds is applied as a patch of the stored
flow; a change the patch cannot make (inside a loop body, a redrawn connector, a
return in an inserted fragment, ...) is refused by exec under "mdl 1;", and
check reports it as an MDL-V1-REBUILD error with exec's message. Under mdl 0
exec rebuilds the whole flow instead, and check reports the MDL-V1-REBUILD
warning exec prints. An "alter microflow|nanoflow" exec would refuse is MDL090.
This is the verdict "mxcli diff" reports as "Refused:", computed by the same
code. A statement on a flow an earlier statement of the script changes, or
whose flow does not build until an earlier statement has run, is not predicted.

Several files are checked as one script set, run in the order given: each
file is checked as it would be alone, and the set is first read as a whole for
two "create or modify" statements declaring the same flow — a placeholder
("stub") followed by the real flow. That is reported as an MDL-STUB01 warning
naming both statements: run in order, the stub replaces the real flow before
the real statement restores it, on every run, and with the two under different
language headers the real statement can be refused while the stub's rebuild
goes through. Since #843 a self-recursive flow is created in one statement, so
the stub can be dropped. One file alone is not read this way.

Output includes structured rule IDs (MDL prefix for reference and script rules,
E0xx for expression type rules) for each validation issue.

A deprecated MDL spelling — an alias left over from consolidating MDL onto one
canonical form, such as "create or replace" for "create or modify" or "show" for
"list" — is reported as an MDL-DEPRnnn warning naming the canonical form. Pass
--deprecations=error to fail the run on one instead, e.g. in CI over docs.

Use --post-migration to scan an existing project (independent of the script)
for legacy native widgets that have pluggable replacements — Studio Pro does
not auto-migrate these on a Mendix major-version upgrade.

Examples:
  # Check syntax only (no project needed)
  mxcli check script.mdl

  # Check syntax, resolve references, and type-check expressions
  mxcli check script.mdl -p app.mpr

  # Scan the project for legacy native widgets after a Mendix upgrade
  mxcli check script.mdl -p app.mpr --post-migration

  # Output as JSON or SARIF: one document on stdout covering every phase
  # (progress goes to stderr); the exit code still says whether it passed
  mxcli check script.mdl --format json
  mxcli check script.mdl -p app.mpr --format sarif > results.sarif

  # Check a script set, run in this order
  mxcli check 10-domain.mdl 20-flows.mdl 30-pages.mdl -p app.mpr

  # Read the script from stdin
  cat script.mdl | mxcli check -
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		if code := runCheckFiles(cmd, args); code != 0 {
			os.Exit(code)
		}
	},
}

// runCheckFile checks one script and returns the exit code: 0 when it passed.
func runCheckFile(cmd *cobra.Command, filePath string) int {
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
	depPolicy := deprecationPolicy(cmd)
	format := resolveFormat(cmd, "text")
	isStructured := format != "" && format != "text"

	outputFormat := linter.OutputFormat(format)
	formatter := linter.GetFormatter(outputFormat, !isStructured)
	// The text report runs in tiers (semantic checks, references, project
	// verdicts, legacy widgets) and each printed its own "N issues" line, so
	// a warning in one tier and an error in another read as two separate
	// counts with a "✓" between them, and neither was the total. The tiers
	// now print their violations and finish prints one summary over all.
	var printed []linter.Violation
	if tf, ok := formatter.(*linter.TextFormatter); ok {
		tf.NoSummary = true
	}
	report := func(vs []linter.Violation) {
		formatter.Format(vs, os.Stderr)
		printed = append(printed, vs...)
	}

	// In a structured format the payload is ONE document on stdout, emitted
	// once at the end (or at the first failing phase). Each phase used to
	// format its own violations to stderr, so `check --format json` put
	// nothing parseable on stdout — only the executor's "Connected to:"
	// chatter — and a run reaching several phases wrote several documents.
	var structured []linter.Violation
	finish := func(code int) int {
		if isStructured {
			formatter.Format(structured, os.Stdout)
		} else if len(printed) > 0 {
			fmt.Fprintln(os.Stderr)
			linter.WriteSummary(os.Stderr, printed, 0)
		}
		return code
	}

	// Read the script (a path, or "-" for stdin)
	content, err := readMDLSource(filePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		return 1
	}

	// Parse the script
	if !isStructured {
		fmt.Printf("Checking syntax: %s\n", mdlSourceLabel(filePath))
	}

	// A .test.mdl / .test.md file is not top-level MDL: each block is a
	// microflow body. Render it as the microflows it becomes, on the source's
	// own lines, so every rule below applies to what the author actually wrote
	// (mendixlabs/mxcli#1103).
	source := string(content)
	var testProblems []linter.Violation
	if testrunner.IsTestFile(filePath) {
		checked, terr := testrunner.CheckSource(source, filePath)
		if terr != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", terr)
			return 1
		}
		source = checked.MDL
		// A rendering with nothing in it means the file declares no @test
		// block. That is not #618's "the parser could not begin reading it"
		// — the parser was handed an empty rendering, not the author's text
		// — so it gets its own message rather than one quoting a line that
		// was never parsed.
		if strings.TrimSpace(source) == "" && strings.TrimSpace(string(content)) != "" {
			fmt.Fprintln(os.Stderr, noTestsDeclaredError(mdlSourceLabel(filePath)))
			return 1
		}
		for _, p := range checked.Problems {
			testProblems = append(testProblems, linter.Violation{
				RuleID:   "MDL-TEST01",
				Severity: linter.SeverityError,
				Message:  fmt.Sprintf("test %q: %s", p.Test, p.Message),
				Location: linter.Location{DocumentType: "test", DocumentName: p.Test},
			})
		}
	}

	prog, errs := visitor.Build(source)
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
			structured = append(structured, parseViolations...)
			return finish(1)
		} else {
			fmt.Fprintf(os.Stderr, "Syntax errors found:\n")
			for _, err := range errs {
				fmt.Fprintf(os.Stderr, "  - %v\n", err)
			}
			// Hint: if script contains IMPORT/QUERY with single $ but not $$, suggest dollar-quoting
			src := source
			if (strings.Contains(src, "IMPORT") || strings.Contains(src, "import")) &&
				(strings.Contains(src, "QUERY") || strings.Contains(src, "query")) &&
				strings.Contains(src, "$") && !strings.Contains(src, "$$") {
				fmt.Fprintf(os.Stderr, "\nHint: SQL queries in IMPORT statements should use dollar-quoting ($$...$$) instead of single quotes.\n")
				fmt.Fprintf(os.Stderr, "  Example: IMPORT FROM alias QUERY $$SELECT * FROM table$$ INTO Module.Entity MAP (...)\n")
			}
		}
		return 1
	}
	// Zero statements from non-empty input is not an empty script: the parser
	// never got into the file. Both gates refuse it (ako/mxcli#618).
	//
	// `source`, not `content`: for a test file they differ, and the message
	// names a line from whichever text the parser was actually given.
	if line, bad := unparsableInput(source, len(prog.Statements)); bad {
		fmt.Fprintln(os.Stderr, unparsableInputError(filePath, line))
		return 1
	}
	if !isStructured {
		fmt.Printf("✓ Syntax OK (%d statements)\n", len(prog.Statements))
	}

	// Every semantic check lives in executor.ValidateProgram, so `mxcli exec`
	// refuses exactly what `mxcli check` reports. Adding a check there gives
	// both commands it at once.
	violations := append(testProblems, executor.ValidateProgram(prog, projectPath)...)

	// With a project, connect before reporting: two semantic rules read the
	// stored model, so `check -p` reports what `exec -p` would (ako/mxcli#943).
	//   - MDL067 is dropped for a flow already stored the way the script
	//     commits — the filter exec's preflight applies.
	//   - MDL-WORKFLOW10 reads a called microflow the script does not create.
	var exec *executor.Executor
	if projectPath != "" {
		e, logger := newLoggedExecutorTo("check", progressSink(format))
		defer logger.Close()
		defer e.Close()
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := e.Execute(stmt); err != nil {
				fmt.Fprintf(os.Stderr, "Error connecting: %v\n", err)
				return 1
			}
		}
		exec = e
		if b := exec.Backend(); b != nil {
			violations = executor.DropSettledCommitNotes(violations, prog, executor.NewStoredCommitEvents(b))
			// MDL004 on a void microflow's end-event value is a warning for a flow
			// already stored that way: re-applying it changes nothing.
			violations = executor.SettleStoredVoidReturnValues(violations, prog, executor.NewStoredVoidReturns(b))
			violations = append(violations, executor.StoredTaskClaimViolations(prog, b)...)
		}
	}
	violations = executor.ApplyDeprecationPolicy(violations, depPolicy)

	if isStructured {
		// Always emit structured output (even when clean)
		structured = append(structured, violations...)
	} else if len(violations) > 0 {
		fmt.Fprintln(os.Stderr)
		report(violations)
	}

	// A semantic error fails the run, but does not hide the reference tier:
	// an unresolved name is a separate error mxbuild reports alongside it, and
	// stopping here made `check --references` look as if it had accepted a
	// call to a parameter the action does not have (CE1613, ako/mxcli#953).
	// The catalog-backed tier after it still waits for a clean script.
	semanticErrors := len(violations) > 0 && linter.Summarize(violations).Errors > 0
	if semanticErrors && !checkRefs {
		return finish(1)
	}

	// If reference checking requested
	if checkRefs {
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for reference checking")
			return 1
		}

		if !isStructured {
			fmt.Printf("\nValidating references against: %s\n", projectPath)
			fmt.Printf("(Note: References to objects created within the script are skipped)\n")
		}
		// exec was connected above, before the semantic report.

		// A test file's microflows live in the runner's MxTest module, which
		// the runner creates first and the project does not have
		// (ako/mxcli#677).
		refProg := prog
		if testrunner.IsTestFile(filePath) {
			refProg = testrunner.WithRunnerModule(prog)
		}

		// Validate the program (considers objects defined within the script)
		validationErrors, refWarnings := exec.ValidateProgramWithWarnings(refProg)

		// Check for project conflicts: plain CREATE where the document already exists
		validationErrors = append(validationErrors, exec.CheckProjectConflicts(refProg)...)

		// Unresolved references in EXCLUDED documents: reported, never
		// failing the run — Mendix does not validate excluded documents.
		// In structured mode they join the error list (one document, not two)
		// or are emitted on their own when there is nothing else.
		var warnViolations []linter.Violation
		for _, w := range refWarnings {
			warnViolations = append(warnViolations, linter.Violation{
				RuleID:   "MDL-REF",
				Severity: linter.SeverityWarning,
				Message:  w,
			})
		}
		if len(refWarnings) > 0 && !isStructured {
			fmt.Fprintf(os.Stderr, "Reference warnings:\n")
			for _, w := range refWarnings {
				fmt.Fprintf(os.Stderr, "  %s\n", w)
			}
		} else if len(warnViolations) > 0 && len(validationErrors) == 0 {
			structured = append(structured, warnViolations...)
		}

		if len(validationErrors) > 0 {
			if isStructured {
				refViolations := warnViolations
				for _, err := range validationErrors {
					refViolations = append(refViolations, linter.Violation{
						RuleID:   "MDL-REF",
						Severity: linter.SeverityError,
						Message:  err.Error(),
					})
				}
				structured = append(structured, refViolations...)
			} else {
				fmt.Fprintf(os.Stderr, "Reference errors:\n")
				for _, err := range validationErrors {
					fmt.Fprintf(os.Stderr, "  %v\n", err)
				}
				fmt.Fprintf(os.Stderr, "\n✗ %d reference error(s) found\n", len(validationErrors))
			}
			return finish(1)
		}
		if !isStructured {
			fmt.Printf("✓ All references valid\n")
		}
		if semanticErrors {
			return finish(1)
		}

		// The catalog-backed tier: the checks whose answers only exist once a
		// project is connected. It runs after the reference check because a
		// script naming things that do not exist has a more basic problem
		// than a mistyped operand — and because building the catalog for a
		// run that already failed is wasted work.
		//
		// Like every other violation this command emits, only an error
		// severity fails the run. Warnings and hints are advice, and a
		// checker whose first outing turns advice into a broken build is a
		// checker people turn off.
		//
		// MDL087 is what this script REMOVES from the project, which nothing
		// reported until now (ako/mxcli#562). `create or modify entity`
		// rebuilds the entity from the statement, so a member the script does
		// not restate is deleted — and the loss only becomes visible slices
		// later, as a CE1613 on whatever still binds it. exec prints the same
		// list, but only as it applies the statement; by then it is gone.
		//
		// Expression type checking is the other half: the rules that need an
		// attribute's type, an enumeration's cases or a microflow's return
		// type. The scope-local tier already ran in the unconditional pass.
		//
		// The flow verdicts are what exec would refuse (ako/mxcli#876): a
		// `create or modify` of a stored flow whose change the splice cannot
		// make is refused under mdl 1 and rebuilt with MDL-V1-REBUILD under
		// mdl 0, and an alter whose patch fails is refused under both. They
		// run the verdict exec and diff run, so the three agree.
		projectViolations := exec.CheckEntityMemberDrops(prog)
		projectViolations = append(projectViolations, exec.TypeCheckProgram(prog)...)
		projectViolations = append(projectViolations, exec.CheckFlowVerdicts(prog)...)
		// MDL-SEC21 (MxBuild CE0106): a flow this script leaves with no allowed
		// role while a page, snippet, nanoflow, menu or navigation uses it. The
		// reported shape was drop + create in separate runs, which loses the
		// roles that `create or modify` keeps.
		projectViolations = append(projectViolations, exec.CheckFlowAccess(prog)...)
		// MDL-I18N01 (MxBuild CE4899): a script that changes the default
		// language leaves every required caption written in the old one empty
		// in the new one (ako/mxcli#944).
		projectViolations = append(projectViolations, exec.CheckDefaultLanguageCaptions(prog)...)

		if len(projectViolations) > 0 {
			if isStructured {
				structured = append(structured, projectViolations...)
			} else {
				fmt.Fprintln(os.Stderr)
				report(projectViolations)
			}
			if linter.Summarize(projectViolations).Errors > 0 {
				return finish(1)
			}
		} else if !isStructured {
			fmt.Printf("✓ Expression types OK, no unstated member drops, no flow change exec would refuse, no used flow left without access\n")
		}
	}

	// Post-migration scan: walk the project for native widgets that
	// have pluggable replacements (Studio Pro does not auto-migrate
	// these on a Mendix major-version upgrade).
	if postMigration {
		if projectPath == "" {
			fmt.Fprintln(os.Stderr, "Error: --project (-p) is required for --post-migration")
			return 1
		}
		if !isStructured {
			fmt.Printf("\nScanning project for legacy native widgets: %s\n", projectPath)
		}
		legacyViolations, err := scanLegacyWidgets(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error scanning project: %v\n", err)
			return 1
		}
		if isStructured {
			structured = append(structured, legacyViolations...)
		} else if len(legacyViolations) > 0 {
			fmt.Fprintln(os.Stderr)
			report(legacyViolations)
			fmt.Fprintf(os.Stderr, "\n✗ %d legacy widget(s) found\n", len(legacyViolations))
		} else {
			fmt.Printf("✓ No legacy native widgets found\n")
		}
		if len(legacyViolations) > 0 {
			summary := linter.Summarize(legacyViolations)
			if summary.Errors > 0 {
				return finish(1)
			}
		}
	}

	finish(0)
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
	return 0
}

// runCheckFiles checks each script of a run and returns the worst exit code.
// One file is checked exactly as before. Several files are a script set run
// in order (ako/mxcli#905): before the files are checked one by one, the set
// is read as a whole for what no single file shows — a flow two of its
// `create or modify` statements declare, the stub-then-real pattern
// (MDL-STUB01, a warning).
func runCheckFiles(cmd *cobra.Command, files []string) int {
	if len(files) == 1 {
		return runCheckFile(cmd, files[0])
	}
	for _, f := range files {
		if f == stdinPath {
			fmt.Fprintln(os.Stderr, "Error: '-' (stdin) checks one script; it cannot be one of several files")
			return 1
		}
	}
	if format := resolveFormat(cmd, "text"); format != "" && format != "text" {
		fmt.Fprintf(os.Stderr, "Error: --format %s writes one document for one script; check the files one at a time\n", format)
		return 1
	}
	if v := stubThenRealViolations(findFlowRedeclarations(parseScriptSet(files))); len(v) > 0 {
		fmt.Printf("Checking the script set: %d files\n", len(files))
		linter.GetFormatter(linter.OutputFormat("text"), true).Format(v, os.Stderr)
		fmt.Fprintln(os.Stderr)
	}
	worst := 0
	for i, f := range files {
		if i > 0 {
			fmt.Println()
		}
		if code := runCheckFile(cmd, f); code > worst {
			worst = code
		}
	}
	return worst
}

// parseScriptSet reads and parses each file for the set-level checks. A file
// that cannot be read or parsed is left out (Prog nil): its own check reports
// that.
func parseScriptSet(files []string) []setScript {
	var out []setScript
	for _, f := range files {
		sc := setScript{Path: f}
		if data, err := readMDLSource(f); err == nil && !testrunner.IsTestFile(f) {
			sc.Source = string(data)
			if prog, errs := visitor.Build(sc.Source); len(errs) == 0 {
				sc.Prog = prog
			}
		}
		out = append(out, sc)
	}
	return out
}
