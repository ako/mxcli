// SPDX-License-Identifier: Apache-2.0

package linter

import (
	"os"
	"path/filepath"
	"strings"
)

// FindLintRulesDir locates the `.claude/lint-rules/` directory that applies to a
// project, searching startDir and then each ancestor up to (and including) the
// repository root.
//
// Discovery used to be exactly `filepath.Join(filepath.Dir(mpr), ".claude",
// "lint-rules")`, which found the rules only when `.claude/` sat beside the
// .mpr. One `.claude/` at the repository root — the ordinary Claude Code layout,
// and what `mxcli init` produced for a solution repo — meant every bundled
// Starlark rule was silently absent: measured at 19 rules loaded instead of 48,
// with no warning, on a project whose author believed 29 convention and security
// rules were guarding it (#904).
//
// The walk stops at the repository root rather than continuing to the filesystem
// root. Escaping the repo would let a stray `.claude/` in a home directory or a
// temp directory supply rules to every project beneath it, which is worse than
// finding none: the run would look green under somebody else's rules.
func FindLintRulesDir(startDir string) string {
	if startDir == "" {
		return ""
	}

	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}

	for {
		candidate := filepath.Join(dir, ".claude", "lint-rules")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}

		// Check the repo root itself, then stop.
		if isRepoRoot(dir) {
			return ""
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Filesystem root, and no repo root was ever seen.
			return ""
		}
		dir = parent
	}
}

// isRepoRoot reports whether dir looks like the top of a checkout. `.git` is a
// directory in a normal clone and a file in a worktree or submodule, so its type
// is not checked.
func isRepoRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// MatchesRuleID reports whether a user-supplied rule id names the given rule.
//
// Matching is case-insensitive and ignores surrounding whitespace. Both matter
// because the `--rules` allowlist disables every rule it does not match, so
// `-r conv009` used to disable all rules and report a clean project rather than
// failing to match anything visible.
func MatchesRuleID(requested, ruleID string) bool {
	return strings.EqualFold(strings.TrimSpace(requested), strings.TrimSpace(ruleID))
}

// UnknownRuleIDs returns the requested ids that name no rule in known,
// preserving both the request order and the spelling the caller used, so the
// error message quotes what was actually typed.
//
// `--rules` is an allowlist applied by disabling everything else, so an id that
// matches nothing disables every rule and the run reports "No issues found." —
// indistinguishable from a clean project. Callers must treat a non-empty result
// as an error (#904).
func UnknownRuleIDs(requested []string, known []Rule) []string {
	if len(requested) == 0 {
		return nil
	}

	var unknown []string
	for _, want := range requested {
		found := false
		for _, rule := range known {
			if MatchesRuleID(want, rule.ID()) {
				found = true
				break
			}
		}
		if !found {
			unknown = append(unknown, want)
		}
	}
	return unknown
}
