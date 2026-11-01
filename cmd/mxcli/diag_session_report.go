// SPDX-License-Identifier: Apache-2.0

// diag_session_report.go answers what `diag loop-report` cannot: where did a
// whole agent session's calls and tokens go — not only its mxcli processes.
// See docs/11-proposals/PROPOSAL_agent_loop_efficiency.md, lever 6.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/sessionreport"
	"github.com/spf13/cobra"
)

// cobraVerb resolves an mxcli argv to its command path with the real command
// tree, as loop-report does, so the session report and the loop report name
// the same command the same way.
func cobraVerb(args []string) string {
	if len(args) > 1 {
		if cmd, _, err := rootCmd.Find(args[1:]); err == nil && cmd != nil && cmd != rootCmd {
			return strings.TrimPrefix(cmd.CommandPath(), rootCmd.Name()+" ")
		}
	}
	return ""
}

// sessionReportJSON is the --json document.
type sessionReportJSON struct {
	Sessions []sessionreport.Report `json:"sessions"`
	Combined *sessionreport.Report  `json:"combined,omitempty"`
}

// writeSessionReports parses the transcripts and writes one report per
// transcript, plus a combined one when there are several.
func writeSessionReports(w io.Writer, paths []string, top int, subagents, asJSON bool) error {
	opts := sessionreport.Options{Top: top, Verb: cobraVerb}
	var sessions []*sessionreport.Session
	for _, p := range paths {
		s, err := sessionreport.ParseFile(p, subagents)
		if err != nil {
			return err
		}
		sessions = append(sessions, s)
	}
	var out sessionReportJSON
	for _, s := range sessions {
		out.Sessions = append(out.Sessions, sessionreport.Analyze([]*sessionreport.Session{s}, opts))
	}
	if len(sessions) > 1 {
		c := sessionreport.Analyze(sessions, opts)
		out.Combined = &c
	}
	if asJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for i, r := range out.Sessions {
		if i > 0 {
			fmt.Fprintln(w)
		}
		sessionreport.Render(w, r)
	}
	if out.Combined != nil {
		fmt.Fprintln(w)
		sessionreport.Render(w, *out.Combined)
	}
	return nil
}

var diagSessionReportCmd = &cobra.Command{
	Use:   "session-report <transcript.jsonl>...",
	Short: "Report where an agent session's tool calls and tokens went, from its transcript",
	Long: `Read Claude Code session transcripts (JSON Lines) and report, per session:

  - model calls, tool calls by tool, cache-read / cache-write / output tokens,
    and wall time
  - the preamble floor: what every call re-reads before any work (system
    prompt, tool definitions, the skill index, CLAUDE.md), times the calls
    that paid it, as a share of the context read
  - Bash calls split into mxcli commands, playwright, build, git and other
  - every call in one category: orientation, write, validate, apply, verify,
    diagnosis, retry, delegate, other
  - the costliest tool results: size in tokens × the model calls that re-read it
  - error → retry chains, aggregated by normalised error message
  - repeated reads of the same file and repeated syntax/help/skill lookups

Cost grows with calls × conversation size, so a result read early is paid for
by every later call. The preamble is the floor under that: it is never
amortised, so its share falls as a session gets longer — and k phases re-pay
it k times. diag loop-report counts mxcli processes from mxcli's own
logs; this reads the agent's transcript, so it also sees Read/Edit/Grep/Skill
calls, result sizes and failures.

Transcripts live in ~/.claude/projects/<project>/<session>.jsonl. Subagent
transcripts in <session>/subagents/ are included (--subagents=false to skip).
The output carries counts and short command, path and error snippets only —
no prompt or tool output — so it can be shared without the conversation.

Examples:
  mxcli diag session-report ~/.claude/projects/-home-me-app/1234abcd-….jsonl
  mxcli diag session-report --top 20 a.jsonl b.jsonl
  mxcli diag session-report --json run/transcript.jsonl
`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		top, _ := cmd.Flags().GetInt("top")
		subagents, _ := cmd.Flags().GetBool("subagents")
		asJSON, _ := cmd.Flags().GetBool("json")
		if err := writeSessionReports(os.Stdout, args, top, subagents, asJSON); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	diagSessionReportCmd.Flags().Bool("json", false, "Emit the reports as JSON")
	diagSessionReportCmd.Flags().Int("top", 10, "How many of the costliest results to list")
	diagSessionReportCmd.Flags().Bool("subagents", true, "Include subagent transcripts from <session>/subagents/")
	diagCmd.AddCommand(diagSessionReportCmd)
}
