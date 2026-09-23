// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// runSyntax executes `mxcli syntax <args...>` and returns the combined output.
//
// Uses rootCmd.SetArgs + rootCmd.Execute because subcommand writers are
// inherited by walking up to the root.
func runSyntax(t *testing.T, args ...string) string {
	t.Helper()
	resetCmdFlags(syntaxCmd)

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&out)
	rootCmd.SetArgs(append([]string{"syntax"}, args...))
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("mxcli syntax %v: %v", args, err)
	}
	return out.String()
}

// mendixlabs/mxcli#1025: `mxcli syntax` advertises
//
//	mxcli syntax workflow user-task targeting     # Drill down to targeting
//
// and an agent that passes the topic as ONE string — which is what a tool
// wrapper, a quoted copy-paste or `sh -c` produces — got
// "Unknown topic: workflow user-task targeting" back, verbatim, down to the
// spaces. Reproduced on the reported v0.20.0 binary and on main.
//
// The REPL's `help` has resolved multi-word topics since it was written; the
// CLI joined its arguments and never split them. One question, two answers.
func TestSyntaxTopicSpellings(t *testing.T) {
	// Every spelling of the same topic must reach the same page.
	tests := []struct {
		name string
		args []string
	}{
		{"separate arguments", []string{"workflow", "user-task", "targeting"}},
		{"one quoted argument", []string{"workflow user-task targeting"}},
		{"dotted path", []string{"workflow.user-task.targeting"}},
		{"words, no hyphen", []string{"workflow", "user", "task", "targeting"}},
		{"one quoted argument, no hyphen", []string{"workflow user task targeting"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := runSyntax(t, tt.args...)
			if strings.Contains(out, "Unknown topic") {
				t.Errorf("mxcli syntax %q reported an unknown topic:\n%s",
					strings.Join(tt.args, " "), firstLines(out, 3))
			}
			if !strings.Contains(out, "workflow.user-task.targeting") {
				t.Errorf("mxcli syntax %q did not reach workflow.user-task.targeting:\n%s",
					strings.Join(tt.args, " "), firstLines(out, 3))
			}
		})
	}
}

// The other two commands the report names, plus the second advertised example.
func TestSyntaxQuotedTopicsFromIssue1025(t *testing.T) {
	for _, topic := range []string{
		"workflow user-task",
		"workflow parallel-split",
		"security entity-access",
	} {
		t.Run(topic, func(t *testing.T) {
			out := runSyntax(t, topic)
			if strings.Contains(out, "Unknown topic") {
				t.Errorf("mxcli syntax %q reported an unknown topic:\n%s", topic, firstLines(out, 3))
			}
		})
	}
}

// Every example in the command's own help text must resolve, whether its topic
// arrives as separate words or as one string. The help block is what the
// reporter followed.
func TestSyntaxHelpExamplesResolve(t *testing.T) {
	for _, line := range strings.Split(syntaxCmd.Long, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "mxcli syntax ") {
			continue
		}
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		var args, flags []string
		for _, w := range strings.Fields(strings.TrimPrefix(line, "mxcli syntax ")) {
			if strings.HasPrefix(w, "-") {
				flags = append(flags, w)
				continue
			}
			args = append(args, w)
		}
		if len(args) == 0 {
			continue
		}
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			if out := runSyntax(t, append(append([]string{}, args...), flags...)...); strings.Contains(out, "Unknown topic") {
				t.Errorf("help advertises %q, which reports an unknown topic:\n%s",
					line, firstLines(out, 3))
			}
			// The same topic handed over as ONE string is the shape that
			// failed in #1025.
			quoted := append([]string{strings.Join(args, " ")}, flags...)
			if out := runSyntax(t, quoted...); strings.Contains(out, "Unknown topic") {
				t.Errorf("help advertises %q; quoted as one argument it reports an unknown topic:\n%s",
					line, firstLines(out, 3))
			}
		})
	}
}

func firstLines(s string, n int) string {
	lines := strings.SplitN(s, "\n", n+1)
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
