// SPDX-License-Identifier: Apache-2.0

// diag_loop_report.go answers "where did this session's mxcli calls go?".
//
// It exists because the cost of an agent-driven session is dominated by the
// NUMBER of tool calls, not by their output: every model call re-reads the whole
// conversation, so total cost is calls x conversation size, and when a removed
// call takes its tool output with it the total falls with the square of the call
// count. A measured comparison of the same class of app built with mxcli vs on
// Vercel put the gap at 4.25x the model calls and 5.4x the conversation re-read.
// See docs/11-proposals/PROPOSAL_agent_loop_efficiency.md.
//
// Every mxcli invocation already writes a session_start record naming its argv
// (mdl/diaglog), so the distribution of calls is on disk and nobody has looked.
// This turns "the loop feels expensive" into a ranked list, and — run before and
// after a change — into evidence that the change worked.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/diaglog"
	"github.com/spf13/cobra"
)

// logRecord is one JSON Lines entry. Only the fields this report reads are
// declared; diaglog writes more.
type logRecord struct {
	Time             time.Time `json:"time"`
	Msg              string    `json:"msg"`
	Args             []string  `json:"args"`
	Mode             string    `json:"mode"`
	CommandsExecuted int       `json:"commands_executed"`
	ErrorsCount      int       `json:"errors_count"`
	PID              int       `json:"pid"`
	// ParentPID is the mxcli process that spawned this one, when one did. It is
	// a string because diaglog writes "" for a top-level run (ako/mxcli#629).
	ParentPID string `json:"parent_pid"`
}

// invocation is one mxcli process: a session_start and the session_end that
// closes it, if there was one.
type invocation struct {
	Verb     string
	Script   string
	Start    time.Time
	End      time.Time
	Ended    bool
	Commands int
	Errors   int
	PID      int
	// Spawned marks a run mxcli started itself. `mxcli test` runs three before a
	// single test executes, so counting them as calls the agent made overstates
	// the loop and understates `test` (ako/mxcli#629).
	Spawned bool
}

// Duration is wall time for an invocation that closed. An invocation that did
// not close has no knowable duration — see Ended.
func (i invocation) Duration() time.Duration { return i.End.Sub(i.Start) }

// verbStats aggregates one command verb.
type verbStats struct {
	Verb string `json:"verb"`
	// Count is invocations of this verb; Unclosed is how many of them wrote no
	// session_end — the report's only evidence of a non-zero exit. Deliberately
	// NOT named Failed, and there is no per-verb error field either: the two
	// populations are disjoint (see loopReport.StatementErrors) and one name for
	// two meanings is how a report starts lying.
	Count    int           `json:"count"`
	Unclosed int           `json:"unclosed"`
	TotalDur time.Duration `json:"-"`
	TotalSec float64       `json:"total_seconds"`
	MedianMS int64         `json:"median_ms"`
}

// loopReport is the whole analysis, and is what --json emits.
type loopReport struct {
	Invocations int `json:"invocations"`
	// StatementErrors counts runs that CLOSED while their summary reported at
	// least one failed statement — in practice `exec --continue-on-error`, which
	// is the only path that keeps going after one. It is NOT the failure count:
	// a run that exits non-zero exits through os.Exit and writes no summary at
	// all, so it lands in Unclosed instead.
	//
	// It was called `failed` until ako/mxcli#620. Measured against a real
	// 442-invocation log it read 0 throughout while real non-zero exits were
	// happening, so anyone reading the JSON alongside `unclosed: 10` drew the
	// wrong conclusion from a field that was doing its job. Naming it after what
	// it measures is the fix; making it mean "failed" would take an exit-code
	// path through all ~250 os.Exit sites.
	StatementErrors int `json:"runs_with_statement_errors"`
	// Unclosed is runs with no session_end. See the comment at its increment.
	Unclosed int `json:"unclosed"`
	// Spawned is runs mxcli started itself, excluded from every other figure
	// here. They are real processes and their time is already inside their
	// parent's, so counting them again would double it (ako/mxcli#629).
	Spawned      int         `json:"spawned_by_mxcli"`
	WallSeconds  float64     `json:"wall_seconds"`
	ByVerb       []verbStats `json:"by_verb"`
	CheckExecDup int         `json:"check_then_exec_pairs"`
	Span         string      `json:"span"`
}

// parseLogRecords reads JSON Lines, skipping anything that does not parse. A
// truncated final line is normal (a process killed mid-write), so a bad line is
// not an error.
func parseLogRecords(lines []string) []logRecord {
	var out []logRecord
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r logRecord
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			continue
		}
		out = append(out, r)
	}
	return out
}

// invocationVerb resolves an argv to the cobra command path it invoked, so the
// verb list maintains itself: a renamed or added command is picked up with no
// change here. Falls back to the session's mode when argv names no subcommand,
// which is how the REPL and the one-shot `-c` form appear.
func invocationVerb(args []string, mode string) string {
	if len(args) > 1 {
		if cmd, _, err := rootCmd.Find(args[1:]); err == nil && cmd != nil && cmd != rootCmd {
			return strings.TrimPrefix(cmd.CommandPath(), rootCmd.Name()+" ")
		}
	}
	switch mode {
	case "batch":
		return "-c (one-shot)"
	case "repl":
		return "REPL"
	case "":
		return "(unknown)"
	default:
		return mode
	}
}

// invocationScript returns the first positional argument after the verb — the
// script a check or exec was given. Empty when there is none.
func invocationScript(args []string) string {
	for i := 1; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			// A flag that takes a value consumes the next token. Treating every
			// flag as valued would swallow a positional after a boolean flag,
			// but mxcli's scripts are passed before their flags in every
			// documented form, so the first positional is reached first.
			continue
		}
		if strings.HasSuffix(a, ".mdl") {
			return a
		}
	}
	return ""
}

// buildInvocations segments the record stream into processes.
//
// Only session_start carries a pid, so sessions are delimited by their own
// start records rather than correlated by pid: an invocation runs until the
// next session_end or the next session_start, whichever comes first. That is
// exact for sequential invocations, which is what an agent loop produces, and
// would mis-attribute two mxcli processes running CONCURRENTLY. The report says
// so rather than pretending otherwise.
func buildInvocations(records []logRecord) []invocation {
	var out []invocation
	open := map[int]int{} // pid -> index of its open invocation
	cur := -1
	for _, r := range records {
		switch r.Msg {
		case "session_start":
			out = append(out, invocation{
				Verb:    invocationVerb(r.Args, r.Mode),
				Script:  invocationScript(r.Args),
				Start:   r.Time,
				PID:     r.PID,
				Spawned: r.ParentPID != "",
			})
			cur = len(out) - 1
			if r.PID != 0 {
				open[r.PID] = cur
			}
		case "session_end":
			// Pair by pid when the log has one. `mxcli test` spawns three mxcli
			// processes before a single test runs, so their session_ends arrive
			// between the parent's start and its own end; closing "the most
			// recent open invocation" hands the parent's close to a child and
			// reports the parent as a failure. Measured: every `test` run showed
			// as unclosed although all its tests passed (ako/mxcli#629).
			i := cur
			if r.PID != 0 {
				j, ok := open[r.PID]
				if !ok {
					continue // an end whose start is outside this window
				}
				i = j
				delete(open, r.PID)
			}
			if i >= 0 && !out[i].Ended {
				out[i].End = r.Time
				out[i].Ended = true
				out[i].Commands = r.CommandsExecuted
				out[i].Errors = r.ErrorsCount
			}
		}
	}
	return out
}

// analyzeLoop is the whole report, as a pure function of the records.
func analyzeLoop(records []logRecord) loopReport {
	invs := buildInvocations(records)

	rep := loopReport{}
	durs := map[string][]time.Duration{}
	stats := map[string]*verbStats{}

	for _, inv := range invs {
		// The report never counts itself. Since ako/mxcli#617 every command is
		// recorded from startSession, which excludes `diag` for this reason;
		// the filter stays as the second guard, because a report whose numbers
		// depend on one exclusion staying in place would drift silently.
		if inv.Verb == "diag" || strings.HasPrefix(inv.Verb, "diag ") {
			continue
		}
		// A run mxcli started itself is not a call anyone made. `mxcli test`
		// spawns `-c DESCRIBE SETTINGS`, `-c SHOW MODULES` and an `exec` of the
		// generated runner before the first test executes, so a session of 5
		// test runs carried 15 phantom entries in the table the report exists to
		// rank. Counted on its own line instead (ako/mxcli#629).
		if inv.Spawned {
			rep.Spawned++
			continue
		}
		s, ok := stats[inv.Verb]
		if !ok {
			s = &verbStats{Verb: inv.Verb}
			stats[inv.Verb] = s
		}
		s.Count++

		// An invocation with no session_end exited through os.Exit, which is how
		// mxcli reports almost every failure — Close() is deferred and deferred
		// calls do not run on os.Exit. Verified against a real log: the three
		// unclosed sessions were exactly the three runs that exited non-zero.
		// A process killed or still running looks identical, so this is reported
		// as "did not close" rather than asserted as "failed".
		if !inv.Ended {
			s.Unclosed++
			rep.Unclosed++
			continue
		}
		if inv.Errors > 0 {
			rep.StatementErrors++
		}
		d := inv.Duration()
		s.TotalDur += d
		durs[inv.Verb] = append(durs[inv.Verb], d)
		rep.WallSeconds += d.Seconds()
	}

	for verb, s := range stats {
		ds := durs[verb]
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
		if len(ds) > 0 {
			s.MedianMS = ds[len(ds)/2].Milliseconds()
		}
		s.TotalSec = s.TotalDur.Seconds()
		rep.ByVerb = append(rep.ByVerb, *s)
	}
	// Most invocations first; ties by name so the output is stable.
	sort.Slice(rep.ByVerb, func(i, j int) bool {
		if rep.ByVerb[i].Count != rep.ByVerb[j].Count {
			return rep.ByVerb[i].Count > rep.ByVerb[j].Count
		}
		return rep.ByVerb[i].Verb < rep.ByVerb[j].Verb
	})

	for _, s := range stats {
		rep.Invocations += s.Count
	}
	rep.CheckExecDup = countCheckThenExec(invs)
	if len(invs) > 0 {
		rep.Span = invs[0].Start.Format(time.RFC3339) + " .. " +
			invs[len(invs)-1].Start.Format(time.RFC3339)
	}
	return rep
}

// countCheckThenExec counts a `check` immediately followed by an `exec` of the
// SAME script — the two-call form the generated CLAUDE.md used to teach.
//
// It is deliberately not called "waste": since ako/mxcli#607 `exec` resolves
// references itself, so what the extra call still buys is project-conflict
// detection (a plain CREATE over an existing document), which exec does not do.
// The number is here to be weighed, not to be eliminated on sight.
func countCheckThenExec(invs []invocation) int {
	n := 0
	for i := 0; i+1 < len(invs); i++ {
		a, b := invs[i], invs[i+1]
		if a.Verb == "check" && b.Verb == "exec" && a.Script != "" && a.Script == b.Script {
			n++
		}
	}
	return n
}

// readLogLines reads every log file in dir, oldest first, so the records come
// back in time order across a multi-day retention window.
func readLogLines(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "mxcli-") && strings.HasSuffix(e.Name(), ".log") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // mxcli-YYYY-MM-DD.log sorts chronologically

	var lines []string
	for _, name := range names {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // argv can be long
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
		f.Close()
	}
	return lines, nil
}

func renderLoopReport(rep loopReport, w io.Writer) {
	fmt.Fprintf(w, "mxcli invocations: %d", rep.Invocations)
	if rep.Span != "" {
		fmt.Fprintf(w, "   (%s)", rep.Span)
	}
	fmt.Fprintln(w)
	if rep.Invocations == 0 {
		fmt.Fprintln(w, "\nNo session records found. Logging is on unless MXCLI_LOG=0,")
		fmt.Fprintln(w, "and logs are kept for 7 days.")
		return
	}
	fmt.Fprintf(w, "Wall time in mxcli: %.1fs across the runs that closed\n", rep.WallSeconds)
	if rep.Unclosed > 0 {
		fmt.Fprintf(w, "Did not close: %d (mxcli exits through os.Exit on most failures,\n"+
			"               which skips the summary record — so these are very likely\n"+
			"               non-zero exits, but a killed process looks the same)\n", rep.Unclosed)
	}
	// Printed only when non-zero. It is near-always zero, and a "0" next to the
	// unclosed count reads as "nothing failed" — which is the opposite of what
	// the two numbers together mean.
	if rep.StatementErrors > 0 {
		fmt.Fprintf(w, "Finished with failed statements: %d (ran to the end and reported\n"+
			"               errors — `exec --continue-on-error`. Separate from the\n"+
			"               unclosed runs above, which exited instead)\n", rep.StatementErrors)
	}

	if rep.Spawned > 0 {
		fmt.Fprintf(w, "Started by mxcli itself: %d (test, new, eval and the LSP run mxcli;\n"+
			"               excluded above — their time is already inside the run\n"+
			"               that started them)\n", rep.Spawned)
	}

	fmt.Fprintln(w, "\nBy command, most calls first:")
	fmt.Fprintf(w, "  %-22s %6s %8s %10s %9s\n", "COMMAND", "CALLS", "UNCLOSED", "TOTAL", "MEDIAN")
	for _, s := range rep.ByVerb {
		fmt.Fprintf(w, "  %-22s %6d %8d %9.1fs %8dms\n",
			s.Verb, s.Count, s.Unclosed, s.TotalSec, s.MedianMS)
	}

	if rep.CheckExecDup > 0 {
		fmt.Fprintf(w, "\n`check` immediately followed by `exec` of the same script: %d\n", rep.CheckExecDup)
		fmt.Fprintln(w, "  Since #607 exec resolves references itself, so the second call buys")
		fmt.Fprintln(w, "  project-conflict detection and the no-apply preflight. Worth weighing,")
		fmt.Fprintln(w, "  not eliminating on sight.")
	}

	fmt.Fprintln(w, "\nWhat this cannot tell you:")
	fmt.Fprintln(w, "  - model calls. One bash call can run several mxcli commands, and the")
	fmt.Fprintln(w, "    agent's other calls are not here at all. This counts mxcli processes.")
	fmt.Fprintln(w, "  - output size. Nothing records how many bytes a command printed, which")
	fmt.Fprintln(w, "    is the other half of the bill.")
	fmt.Fprintln(w, "  - reloads vs restarts. A long-running `run --local` is one invocation")
	fmt.Fprintln(w, "    however many times it hot-applies a change.")
}

var diagLoopReportCmd = &cobra.Command{
	Use:   "loop-report",
	Short: "Report where this project's mxcli calls went, from the session logs",
	Long: `Summarise the session logs as a per-command call count, wall time and exit health.

Every mxcli invocation writes a session record naming its argv, so the shape of
an agent's loop is already on disk. This reports it: which commands were run,
how often, how long they took, and how many did not exit cleanly.

A run that fails exits through os.Exit and writes no summary record, so it is
counted as "did not close" rather than as a failure — the report says which it
is measuring rather than presenting one as the other.

It counts mxcli PROCESSES, not model calls — one shell command can run several.
Read it to find which command dominates a loop, and re-run it after a change to
see whether the loop actually moved.

Examples:
  mxcli diag loop-report
  mxcli diag loop-report --json
  mxcli diag loop-report --log-dir ./collected-logs
`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, _ []string) {
		dir, _ := cmd.Flags().GetString("log-dir")
		if dir == "" {
			dir = diaglog.LogDir()
		}
		asJSON, _ := cmd.Flags().GetBool("json")

		lines, err := readLogLines(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading logs from %s: %v\n", dir, err)
			os.Exit(1)
		}
		rep := analyzeLoop(parseLogRecords(lines))

		if asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(rep); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			return
		}
		renderLoopReport(rep, os.Stdout)
	},
}

func init() {
	diagLoopReportCmd.Flags().Bool("json", false, "Emit the report as JSON")
	diagLoopReportCmd.Flags().String("log-dir", "", "Read logs from this directory instead of ~/.mxcli/logs")
	diagCmd.AddCommand(diagLoopReportCmd)
}
