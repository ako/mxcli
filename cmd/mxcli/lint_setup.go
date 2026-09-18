// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/mendixlabs/mxcli/mdl/linter"
)

// projectLintRules builds the rule set for a project: the built-in Go rules plus
// every Starlark rule under the nearest .claude/lint-rules/.
//
// It exists so `mxcli lint` and `mxcli report` cannot disagree about what "the
// rules" are. They did: report carried its own inline copy of the built-in list
// that had fallen one rule behind (MDL-FLOW01), so the two commands scored a
// project against different rule sets — the same class as #904, where a silently
// reduced rule set produced a falsely high score, and just as invisible.
//
// Load failures are warned about rather than swallowed, because a rule that
// fails to load reads exactly like a project that has nothing to report.
func projectLintRules(projectDir string, warn io.Writer) []linter.Rule {
	lintRules := builtinLintRules()

	// Searched upward from the project, so one directory at the repo root
	// serves an app in a subfolder (#904).
	lintRulesDir := linter.FindLintRulesDir(projectDir)
	starlarkRules, loadFailures, err := linter.LoadStarlarkRulesFromDir(lintRulesDir)
	if err != nil {
		// Previously discarded in `lint`, which made an unreadable directory
		// look exactly like a project with no custom rules.
		fmt.Fprintf(warn, "Warning: could not read %s: %v\n", lintRulesDir, err)
	}
	for _, f := range loadFailures {
		fmt.Fprintf(warn, "Warning: rule file skipped: %s: %s\n", f.Path, f.Reason)
	}
	if len(loadFailures) > 0 {
		fmt.Fprintf(warn, "Warning: %d rule file(s) skipped — those rules did not run.\n", len(loadFailures))
	}
	for _, rule := range starlarkRules {
		lintRules = append(lintRules, rule)
	}
	return lintRules
}

// applyLintConfig loads the project's lint-config.yaml and applies it to lint,
// returning the config and the path it came from (empty when there is none).
//
// `mxcli report` did not call this at all (ako/mxcli#525). A team that had
// accepted a rule and disabled it in the config saw `mxcli lint` agree and the
// report's SCORE stay exactly where it was — so the score could not be
// calibrated even in principle, and the natural conclusion was that the tool
// disagreed with a deliberate decision rather than that it had not read the
// file.
//
// It applies the rule overrides and returns the config, leaving the caller to
// merge cfg.ExcludeModules into its own LintContext. That split is deliberate:
// `lint` additionally warns when --modules names a module the config excludes,
// which needs the config and the flags together, and a helper that silently set
// the excludes would make that warning easy to lose.
func applyLintConfig(lint *linter.Linter, projectDir string, warn io.Writer) (*linter.Config, string) {
	configPath := linter.FindConfigFile(projectDir)
	cfg, err := linter.LoadConfig(configPath)
	if err != nil {
		fmt.Fprintf(warn, "Warning: failed to load lint config: %v\n", err)
		return nil, configPath
	}
	cfg.ApplyConfig(lint)
	return cfg, configPath
}
