// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/deprecation"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/linter"
)

// execPreflight runs exec's pre-flight checks on prog, printing what they
// report to w, and returns why exec refuses to run the script — the message it
// prints before exiting — or "" when it runs it. `mxcli diff` runs the same
// checks, so it refuses exactly the scripts exec refuses (ako/mxcli#807).
//
// exec is the executor the script would run on, connected to projectPath ("" when
// the script connects itself). script names the script for the hint that
// points at `mxcli check` ("-" for stdin). showInfo prints info-level notes in
// full; otherwise they are counted on one line (see printPreflightViolations).
// continueOnError is exec's --continue-on-error: a flow change exec would refuse
// is then reported but does not refuse the script, since that mode asks for
// every statement that can run to run.
func execPreflight(exec *executor.Executor, prog *ast.Program, projectPath, script string, skipCheck, showInfo, continueOnError bool, depPolicy deprecation.Policy, w io.Writer, color bool) string {
	// Pre-flight: refuse a script whose semantic checks report an error,
	// rather than writing part of it and leaving the model to mxbuild.
	// exec is not transactional, so "run it and see" means a half-applied
	// model. Warnings are printed and do not stop the run.
	if !skipCheck {
		violations := executor.ApplyDeprecationPolicy(executor.ValidateProgram(prog, projectPath), depPolicy)
		// The bare-commit note (MDL067) says a re-run flips what is stored;
		// for a flow the project already holds that way it does not, and the
		// note would repeat on every run of an idempotent script.
		if exec != nil {
			if b := exec.Backend(); b != nil {
				violations = executor.DropSettledCommitNotes(violations, prog, executor.NewStoredCommitEvents(b))
				// MDL004 on a void microflow's end-event value is a warning for a flow
				// already stored that way: re-applying it changes nothing.
				violations = executor.SettleStoredVoidReturnValues(violations, prog, executor.NewStoredVoidReturns(b))
				// A called microflow the script does not create is read from the
				// project for MDL-WORKFLOW10 (ako/mxcli#943).
				violations = append(violations, executor.StoredTaskClaimViolations(prog, b)...)
			}
		}
		printPreflightViolations(violations, script, showInfo, w, color)
		if summary := linter.Summarize(violations); summary.Errors > 0 {
			return fmt.Sprintf(
				"\nRefusing to execute: %d error(s) above. Nothing was written.\n"+
					"  exec applies statements one at a time and cannot roll back, so a script\n"+
					"  with a known error would leave the model partly updated.\n"+
					"  Fix them, or re-run with --no-check to apply the script anyway.\n",
				summary.Errors)
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
	// re-run, and refusing it would break scripts that work today. Its one
	// exception is: a create over a name ANOTHER kind already has in the
	// module (ako/mxcli#793) is never a re-run — `or modify` of a kind that
	// lacks the name still adds an element — and Mendix rejects the model
	// (CE0122 / CE0065), so it is refused here, after the references.
	if !skipCheck && projectPath != "" {
		refErrs, refWarnings := exec.ValidateProgramWithWarnings(prog)
		for _, warning := range refWarnings {
			fmt.Fprintf(w, "Reference warning: %s\n", warning)
		}
		if len(refErrs) > 0 {
			for _, refErr := range refErrs {
				fmt.Fprintf(w, "Reference error: %v\n", refErr)
			}
			return fmt.Sprintf(
				"\nRefusing to execute: %d unresolved reference(s) above. Nothing was written.\n"+
					"  A name that resolves to nothing is written into the model as it stands and\n"+
					"  is not reported until mxbuild rejects it (CE1613) — and a misspelled MODULE\n"+
					"  is created rather than refused.\n"+
					"  Fix them, or re-run with --no-check to apply the script anyway.\n",
				len(refErrs))
		}
		if clashes := exec.CheckProjectNameClashes(prog); len(clashes) > 0 {
			for _, c := range clashes {
				fmt.Fprintf(w, "Name clash: %v\n", c)
			}
			return fmt.Sprintf(
				"\nRefusing to execute: %d name clash(es) above. Nothing was written.\n"+
					"  Mendix rejects the model (CE0122 / CE0065) however the script continues.\n"+
					"  Rename, or re-run with --no-check to apply the script anyway (each clashing\n"+
					"  create is still refused when it runs).\n",
				len(clashes))
		}

		// Fourth pass: a flow change exec would refuse when it reaches it — a
		// splice under mdl 1, an alter whose patch fails. `check -p` already
		// runs this verdict; exec did not, so it wrote every statement before
		// the refused one and none after, leaving the model half-applied.
		verdicts := exec.CheckFlowVerdicts(prog)
		if len(verdicts) > 0 {
			(&linter.TextFormatter{UseColor: color}).Format(verdicts, w)
			if n := linter.Summarize(verdicts).Errors; n > 0 && !continueOnError {
				return fmt.Sprintf(
					"\nRefusing to execute: %d flow change(s) above would be refused when reached. Nothing was written.\n"+
						"  exec applies statements one at a time, so the statements before a refused\n"+
						"  change would be written and the ones after it would not.\n"+
						"  Make the change the refusal names, or re-run with --continue-on-error to\n"+
						"  apply every other statement.\n",
					n)
			}
		}
	}
	return ""
}

// printPreflightViolations prints what exec's semantic pass found: errors and
// warnings in full, info notes as one count line unless showInfo is set.
//
// An info note never stops exec and asks nothing of a script that is being
// re-run; printed in full on every run they buried the warnings that do (on
// report pages MDL-WIDGET15 alone was ~35 notes a run). `check` is where a
// script is reviewed, and it still prints every note, so the count line
// points there. Only this text report changes: exec has no structured
// diagnostics output, and `check --format json|sarif` is untouched.
func printPreflightViolations(violations []linter.Violation, script string, showInfo bool, w io.Writer, color bool) {
	shown := violations
	infos := 0
	if !showInfo {
		shown = nil
		for _, v := range violations {
			if v.Severity == linter.SeverityInfo {
				infos++
				continue
			}
			shown = append(shown, v)
		}
	}
	if len(shown) > 0 {
		// The summary line counts the omitted notes too, marked not shown.
		f := &linter.TextFormatter{UseColor: color, OmittedInfos: infos}
		f.Format(shown, w)
	}
	if infos > 0 {
		noun := "info notes"
		if infos == 1 {
			noun = "info note"
		}
		if script == "" {
			script = "<script>"
		}
		fmt.Fprintf(w, "%d %s not shown — run `mxcli check %s` to see them (or pass --verbose)\n",
			infos, noun, script)
	}
}
