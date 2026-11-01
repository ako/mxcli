// SPDX-License-Identifier: Apache-2.0

package sessionreport

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// human renders a token count compactly: 950, 12.3k, 4.1M.
func human(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fG", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprintf("%d", n)
}

func dur(sec float64) string {
	d := time.Duration(sec) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm%02ds", m, int(d.Seconds())%60)
}

func joinCounts(cs []Count, limit int) string {
	var parts []string
	for i, c := range cs {
		if limit > 0 && i == limit {
			rest := 0
			for _, r := range cs[i:] {
				rest += r.N
			}
			parts = append(parts, fmt.Sprintf("+%d more (%d)", len(cs)-i, rest))
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", c.Name, c.N))
	}
	return strings.Join(parts, ", ")
}

func pct(n, total int) string {
	if total == 0 {
		return "0%"
	}
	return fmt.Sprintf("%d%%", (n*100+total/2)/total)
}

// Render writes the terse text form of a report. It prints counts, tokens,
// and short command/path/error snippets only — never prompt or result text.
func Render(w io.Writer, r Report) {
	head := "session " + r.Label
	if r.Sessions > 1 {
		head = fmt.Sprintf("combined: %d sessions", r.Sessions)
	}
	if r.Subagents > 0 {
		head += fmt.Sprintf(" (+%d subagents)", r.Subagents)
	}
	if !r.Start.IsZero() {
		head += fmt.Sprintf("  %s  wall %s", r.Start.UTC().Format("2006-01-02 15:04"), dur(r.WallSeconds))
		if r.WallSeconds > 60 && r.ActiveSeconds < r.WallSeconds*0.9 {
			head += fmt.Sprintf(" (active %s)", dur(r.ActiveSeconds))
		}
	}
	fmt.Fprintln(w, head)

	calls := fmt.Sprintf("model calls %d", r.ModelCalls)
	if r.Subagents > 0 {
		calls += fmt.Sprintf(" (main %d)", r.MainModelCalls)
	}
	fmt.Fprintf(w, "%s  tool calls %d  avg context %s", calls, r.ToolCalls, human(r.AvgContext))
	if r.Compactions > 0 {
		fmt.Fprintf(w, "  compactions %d", r.Compactions)
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "tokens: cache-read %s  cache-write %s  output %s  input %s\n",
		human(r.Tokens.CacheRead), human(r.Tokens.CacheWrite), human(r.Tokens.Output), human(r.Tokens.Input))
	if r.Tokens.CacheRead > 0 {
		fmt.Fprintf(w, "tool results: %s tokens, re-read cost %s (%s of cache-read)\n",
			human(r.ResultTokens), human(r.ReReadCost), pct(int(r.ReReadCost/1000), int(r.Tokens.CacheRead/1000)))
	}
	renderPreamble(w, r)
	if r.ToolCalls == 0 {
		return
	}
	fmt.Fprintf(w, "tools: %s\n", joinCounts(r.ByTool, 8))
	if r.Bash.Calls > 0 {
		fmt.Fprintf(w, "bash %d: %s\n", r.Bash.Calls, joinCounts(r.Bash.ByBucket, 0))
		if r.Bash.MxcliInvocations > 0 {
			fmt.Fprintf(w, "  mxcli %d invocations", r.Bash.MxcliInvocations)
			if r.Bash.ChainedCalls > 0 {
				fmt.Fprintf(w, " (%d calls chained >1)", r.Bash.ChainedCalls)
			}
			fmt.Fprintf(w, ": %s\n", joinCounts(r.Bash.MxcliVerbs, 10))
		}
	}
	var cats []string
	for _, c := range r.ByCategory {
		cats = append(cats, fmt.Sprintf("%s %d (%s)", c.Name, c.N, pct(c.N, r.ToolCalls)))
	}
	fmt.Fprintf(w, "category: %s\n", strings.Join(cats, ", "))

	if len(r.Top) > 0 {
		fmt.Fprintln(w, "costliest results (tokens × later calls):")
		for i, c := range r.Top {
			sub := ""
			if c.Agent {
				sub = " [sub]"
			}
			fmt.Fprintf(w, "  %2d %6s ×%-4d =%6s  %-5s %s%s\n", i+1, human(int64(c.Tokens)), c.Readers, human(c.Cost), c.Tool, c.What, sub)
		}
	}

	resolved := 0
	for _, c := range r.Chains {
		if c.Resolved {
			resolved++
		}
	}
	fmt.Fprintf(w, "failures %d  retry chains %d (%d retry calls, %d resolved)\n", r.Failures, len(r.Chains), r.RetryCalls, resolved)
	for i, c := range r.Chains {
		if i == 5 {
			break
		}
		state := "open"
		if c.Resolved {
			state = "ok"
		}
		fmt.Fprintf(w, "  %3d %-4s %s | %s\n", c.Length, state, c.What, c.FirstError)
	}
	shown := 0
	for _, f := range r.ErrorFamilies {
		if f.Retries == 0 && f.Failures < 2 {
			continue
		}
		if shown == 0 {
			fmt.Fprintln(w, "errors by retries (retries/chains/failures):")
		}
		if shown == 8 {
			break
		}
		fmt.Fprintf(w, "  %3d/%d/%d %s\n", f.Retries, f.Chains, f.Failures, f.Error)
		shown++
	}

	fmt.Fprintf(w, "lookups: %d file reads", r.FileReads)
	if len(r.RepeatedReads) > 0 {
		extra := 0
		for _, c := range r.RepeatedReads {
			extra += c.N - 1
		}
		fmt.Fprintf(w, ", %d repeats (%s)", extra, joinCountsX(r.RepeatedReads, 5))
	}
	fmt.Fprintf(w, "; %d doc lookups", r.DocLookups)
	if len(r.RepeatedLookups) > 0 {
		fmt.Fprintf(w, ", repeated: %s", joinCountsX(r.RepeatedLookups, 5))
	}
	fmt.Fprintln(w)
}

func joinCountsX(cs []Count, limit int) string {
	var parts []string
	for i, c := range cs {
		if i == limit {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprintf("%s ×%d", c.Name, c.N))
	}
	return strings.Join(parts, ", ")
}

// renderPreamble reports the fixed cost every model call re-reads before any
// work, and what it came to over the session. The floor alone does not say
// whether it is worth acting on — the share does, and it shrinks as a session
// gets longer, which is the whole point of printing both.
func renderPreamble(w io.Writer, r Report) {
	total := r.Tokens.Input + r.Tokens.CacheRead + r.Tokens.CacheWrite
	if total <= 0 || r.PreamblePaid <= 0 {
		return
	}
	share := pct(int(r.PreamblePaid/1000), int(total/1000))
	switch {
	case r.PreamblePaid > total:
		// Over the whole bill means the floor is not a floor: the transcript
		// starts mid-conversation, so the first call's context is prior work
		// as well as preamble. Say so rather than print a share over 100%.
		fmt.Fprintf(w, "preamble: %s floor overstates it (resumed or already-compacted transcript: the first call carries prior conversation)\n",
			human(r.PreambleTokens))
	case r.Subagents > 0:
		// With subagents the floor × calls arithmetic does not hold on the
		// shown numbers — each conversation pays its own, smaller floor — so
		// the multiplication is only spelled out when there is one context.
		fmt.Fprintf(w, "preamble: %s floor (main), %s paid over %d calls in %d contexts (%s of context read)\n",
			human(r.PreambleTokens), human(r.PreamblePaid), r.ModelCalls, r.Subagents+1, share)
	default:
		fmt.Fprintf(w, "preamble: %s floor × %d calls = %s (%s of context read)\n",
			human(r.PreambleTokens), r.ModelCalls, human(r.PreamblePaid), share)
	}
}
