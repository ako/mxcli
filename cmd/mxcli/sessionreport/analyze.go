// SPDX-License-Identifier: Apache-2.0

package sessionreport

import (
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Count is a named tally.
type Count struct {
	Name string `json:"name"`
	N    int    `json:"n"`
}

// BashBreakdown splits Bash calls by the program they ran.
type BashBreakdown struct {
	Calls int `json:"calls"`
	// ByBucket counts each Bash call once, under its most significant
	// program: mxcli, playwright, build, git, other — in that order.
	ByBucket []Count `json:"by_bucket"`
	// MxcliInvocations counts mxcli commands, not calls: `exec x && docker
	// check` is one call and two invocations.
	MxcliInvocations int     `json:"mxcli_invocations"`
	MxcliVerbs       []Count `json:"mxcli_verbs"`
	// ChainedCalls are Bash calls that ran more than one mxcli command.
	ChainedCalls int `json:"chained_calls"`
}

// CostlyResult is one tool result ranked by what it cost to keep around.
type CostlyResult struct {
	Tool     string `json:"tool"`
	What     string `json:"what"`
	Category string `json:"category"`
	Tokens   int    `json:"tokens"`
	// Readers is the number of model calls that re-read the result: every
	// later call in its conversation, up to the next compaction.
	Readers int   `json:"readers"`
	Cost    int64 `json:"cost"`
	Agent   bool  `json:"in_subagent,omitempty"`
}

// Chain is a failed call and the calls that followed up on it.
type Chain struct {
	Tool       string `json:"tool"`
	What       string `json:"what"`
	FirstError string `json:"first_error"`
	// Length is the follow-up calls: re-runs, edits and re-reads of the same
	// script, file or command. The failed call itself is not counted.
	Length   int  `json:"length"`
	Resolved bool `json:"resolved"`
}

// ErrorFamily aggregates chains by normalised first error.
type ErrorFamily struct {
	Error    string `json:"error"`
	Failures int    `json:"failures"`
	Chains   int    `json:"chains"`
	Retries  int    `json:"retries"`
}

// Report is the analysis of one session, or of several combined.
type Report struct {
	Label       string    `json:"label"`
	Sessions    int       `json:"sessions"`
	Subagents   int       `json:"subagents"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	WallSeconds float64   `json:"wall_seconds"`
	// ActiveSeconds is the main conversation's wall time without pauses
	// longer than 15 minutes.
	ActiveSeconds float64 `json:"active_seconds"`

	ModelCalls     int   `json:"model_calls"`
	MainModelCalls int   `json:"main_model_calls"`
	ToolCalls      int   `json:"tool_calls"`
	Tokens         Usage `json:"tokens"`
	// AvgContext is input + cache read + cache write per model call: the
	// average conversation size a call re-read.
	AvgContext  int64 `json:"avg_context_tokens"`
	Compactions int   `json:"compactions"`

	// PreambleTokens is the main conversation's fixed floor: what every model
	// call re-reads before any work — system prompt, tool definitions, the
	// skill index, CLAUDE.md. PreamblePaid is that floor times the calls that
	// paid it, summed per conversation, because the preamble is never
	// amortised and each subagent carries its own.
	//
	// Two things only this pair answers. How much of a session was fixed
	// overhead rather than work — the share is PreamblePaid over the context
	// read, and it falls as a session gets longer, so a lever aimed at the
	// preamble is worth least exactly where the bill is biggest. And what a
	// phase split costs: k phases re-pay the floor k times, so the floor is a
	// bound that splitting cannot go below.
	//
	// Both are upper bounds — see Conversation.FirstContext.
	PreambleTokens int64 `json:"preamble_tokens"`
	PreamblePaid   int64 `json:"preamble_paid_tokens"`

	ByTool     []Count       `json:"by_tool"`
	Bash       BashBreakdown `json:"bash"`
	ByCategory []Count       `json:"by_category"`

	// ResultTokens is the estimated size of every tool result together;
	// ReReadCost is Σ size × readers, the part of the cache-read bill that
	// tool results account for (the rest is prompts, system context and the
	// model's own output being re-read).
	ResultTokens int64          `json:"result_tokens"`
	ReReadCost   int64          `json:"reread_cost"`
	Top          []CostlyResult `json:"top_results"`

	Failures      int           `json:"failed_calls"`
	RetryCalls    int           `json:"retry_calls"`
	Chains        []Chain       `json:"retry_chains"`
	ErrorFamilies []ErrorFamily `json:"error_families"`

	FileReads       int     `json:"file_reads"`
	RepeatedReads   []Count `json:"repeated_reads"`
	DocLookups      int     `json:"doc_lookups"`
	RepeatedLookups []Count `json:"repeated_lookups"`
}

// Options tune the analysis.
type Options struct {
	Top  int      // results to keep in Top; 0 means 10
	Verb VerbFunc // mxcli verb resolver; nil means DefaultVerb
}

// chainIdleLimit closes an open retry chain after this many unrelated calls:
// past that, a call on the same file is new work, not a retry.
const chainIdleLimit = 8

// callInfo is the derived view of a call.
type callInfo struct {
	*Call
	conv     *Conversation
	category string
	segs     []Segment
	targets  []string
	snippet  string
}

// Analyze reports on the given sessions together. Call it with one session
// for a per-session report.
func Analyze(sessions []*Session, opts Options) Report {
	if opts.Top <= 0 {
		opts.Top = 10
	}
	if opts.Verb == nil {
		opts.Verb = DefaultVerb
	}
	rep := Report{Sessions: len(sessions)}
	if len(sessions) == 1 {
		rep.Label = shortID(sessions[0].ID)
	} else {
		rep.Label = "combined"
	}

	tools := map[string]int{}
	buckets := map[string]int{}
	verbs := map[string]int{}
	cats := map[string]int{}
	reads := map[string]int{}
	lookups := map[string]int{}
	families := map[string]*ErrorFamily{}
	var costly []CostlyResult

	for _, s := range sessions {
		var first, last time.Time
		for _, conv := range s.Conversations {
			if conv.Key != "main" {
				rep.Subagents++
			} else {
				rep.MainModelCalls += conv.ModelCalls
				rep.ActiveSeconds += conv.Active.Seconds()
				// The floor a project can act on is the main conversation's.
				// Over several sessions the largest is the honest one to show:
				// an average would hide the context a call actually starts at.
				if conv.FirstContext > rep.PreambleTokens {
					rep.PreambleTokens = conv.FirstContext
				}
			}
			rep.PreamblePaid += conv.FirstContext * int64(conv.ModelCalls)
			if !conv.First.IsZero() && (first.IsZero() || conv.First.Before(first)) {
				first = conv.First
			}
			if conv.Last.After(last) {
				last = conv.Last
			}
			rep.ModelCalls += conv.ModelCalls
			rep.Tokens.add(conv.Usage)
			rep.Compactions += len(conv.Boundaries)

			infos := make([]*callInfo, 0, len(conv.Calls))
			for _, c := range conv.Calls {
				ci := &callInfo{Call: c, conv: conv}
				ci.category, ci.segs = categorize(c, opts.Verb)
				ci.targets = callTargets(ci)
				ci.snippet = Snippet(c)
				infos = append(infos, ci)
			}
			chains := findChains(infos)
			for _, ch := range chains {
				if ch.Length > 0 {
					rep.Chains = append(rep.Chains, ch.Chain)
				}
				key := NormalizeError(ch.FirstError)
				f := families[key]
				if f == nil {
					f = &ErrorFamily{Error: key}
					families[key] = f
				}
				f.Failures += ch.failures
				if ch.Length > 0 {
					f.Chains++
					f.Retries += ch.Length
				}
			}

			for _, ci := range infos {
				rep.ToolCalls++
				tools[ci.Tool]++
				cats[ci.category]++
				if ci.Failed {
					rep.Failures++
				}
				if ci.category == CatRetry {
					rep.RetryCalls++
				}
				if ci.Tool == "Bash" {
					countBash(ci, &rep.Bash, buckets, verbs)
				}
				countLookups(ci, reads, lookups, &rep)

				if ci.HasResult {
					readers := conv.readersAfter(ci.Turn)
					cost := int64(ci.ResultTokens) * int64(readers)
					rep.ResultTokens += int64(ci.ResultTokens)
					rep.ReReadCost += cost
					costly = append(costly, CostlyResult{
						Tool: ci.Tool, What: ci.snippet, Category: ci.category,
						Tokens: ci.ResultTokens, Readers: readers, Cost: cost,
						Agent: conv.Key != "main",
					})
				}
			}
		}
		if !first.IsZero() {
			if rep.Start.IsZero() || first.Before(rep.Start) {
				rep.Start = first
			}
			if last.After(rep.End) {
				rep.End = last
			}
			rep.WallSeconds += last.Sub(first).Seconds()
		}
	}

	if rep.ModelCalls > 0 {
		rep.AvgContext = (rep.Tokens.Input + rep.Tokens.CacheRead + rep.Tokens.CacheWrite) / int64(rep.ModelCalls)
	}
	rep.ByTool = sortedCounts(tools)
	rep.ByCategory = categoryCounts(cats)
	rep.Bash.ByBucket = sortedCounts(buckets)
	rep.Bash.MxcliVerbs = sortedCounts(verbs)

	sort.SliceStable(costly, func(i, j int) bool { return costly[i].Cost > costly[j].Cost })
	if len(costly) > opts.Top {
		costly = costly[:opts.Top]
	}
	rep.Top = costly

	sort.SliceStable(rep.Chains, func(i, j int) bool { return rep.Chains[i].Length > rep.Chains[j].Length })
	for _, f := range families {
		rep.ErrorFamilies = append(rep.ErrorFamilies, *f)
	}
	sort.Slice(rep.ErrorFamilies, func(i, j int) bool {
		a, b := rep.ErrorFamilies[i], rep.ErrorFamilies[j]
		if a.Retries != b.Retries {
			return a.Retries > b.Retries
		}
		if a.Failures != b.Failures {
			return a.Failures > b.Failures
		}
		return a.Error < b.Error
	})
	rep.RepeatedReads = repeated(reads)
	rep.RepeatedLookups = repeated(lookups)
	return rep
}

func shortID(id string) string {
	if len(id) > 8 && strings.Count(id, "-") >= 4 {
		return id[:8]
	}
	return id
}

func countBash(ci *callInfo, bb *BashBreakdown, buckets, verbs map[string]int) {
	bb.Calls++
	best, rank := BucketOther, 0
	n := 0
	for _, s := range ci.segs {
		if r := bucketRank[s.Bucket]; r > rank && !(s.Bucket == BucketOther && quietPrograms[s.Program]) {
			best, rank = s.Bucket, r
		}
		if s.Bucket == BucketMxcli {
			n++
			verbs[s.Verb]++
		}
	}
	buckets[best]++
	bb.MxcliInvocations += n
	if n > 1 {
		bb.ChainedCalls++
	}
}

var skillPath = regexp.MustCompile(`skills/(?:[^/]+/)*?([^/]+)/SKILL\.md$|skills/(?:[^/]+/)*([^/]+)\.md$`)

func countLookups(ci *callInfo, reads, lookups map[string]int, rep *Report) {
	notePath := func(p string) {
		if p == "" {
			return
		}
		rep.FileReads++
		reads[shortPath(p)]++
		if key := docKey(p); key != "" {
			rep.DocLookups++
			lookups[key]++
		}
	}
	switch ci.Tool {
	case "Read":
		notePath(ci.Path)
	case "Skill":
		rep.DocLookups++
		lookups["skill "+ci.Name]++
	case "Bash":
		for _, s := range ci.segs {
			switch {
			case s.Bucket == BucketMxcli && (strings.HasPrefix(s.Verb, "syntax") || strings.HasPrefix(s.Verb, "help")):
				rep.DocLookups++
				lookups["mxcli "+strings.Join(nonFlags(s.Args[1:]), " ")]++
			case s.Program == "cat" || s.Program == "less" || s.Program == "head":
				for _, a := range nonFlags(s.Args[1:]) {
					if strings.Contains(a, ".") {
						notePath(a)
					}
				}
			}
		}
	}
}

// docKey names a documentation read: a skill, or the project instructions.
func docKey(p string) string {
	base := path.Base(p)
	if base == "CLAUDE.md" || base == "AGENTS.md" {
		return base
	}
	if m := skillPath.FindStringSubmatch(p); m != nil {
		name := m[1]
		if name == "" {
			name = m[2]
		}
		return "skill " + name
	}
	return ""
}

func nonFlags(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.ContainsAny(a, "<>") {
			continue // a redirection, not an argument
		}
		if strings.HasPrefix(a, "-") {
			if valuedRootFlags[a] || a == "-n" {
				i++
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

var fileArg = regexp.MustCompile(`^[^-][^\s]*\.[A-Za-z]{1,5}$`)

// callTargets names what a call works on, so a follow-up on the same thing
// can be recognised: the scripts an mxcli command was given, the file a
// Read/Edit/Write touched, otherwise the command itself.
func callTargets(ci *callInfo) []string {
	switch ci.Tool {
	case "Read", "Edit", "Write", "MultiEdit", "NotebookEdit":
		if ci.Path != "" {
			return []string{"file:" + path.Base(ci.Path)}
		}
	case "Bash":
		var t []string
		for _, s := range ci.segs {
			if s.Bucket == BucketMxcli {
				if len(s.Scripts) > 0 {
					for _, sc := range s.Scripts {
						t = append(t, "file:"+path.Base(sc))
					}
				} else {
					t = append(t, "mxcli:"+s.Verb)
				}
				continue
			}
			if quietPrograms[s.Program] {
				continue
			}
			for _, a := range s.Args[1:] {
				if fileArg.MatchString(a) {
					t = append(t, "file:"+path.Base(a))
				}
			}
		}
		if len(t) == 0 {
			t = append(t, "cmd:"+spaces.ReplaceAllString(strings.TrimSpace(ci.Command), " "))
		}
		return t
	}
	return []string{"tool:" + ci.Tool + ":" + ci.snippet}
}

type chainState struct {
	Chain
	targets  map[string]bool
	idle     int
	open     bool
	failures int
}

// touches reports whether a call only reads or edits a file: it continues a
// chain but cannot resolve it.
func touches(tool string) bool {
	switch tool {
	case "Read", "Edit", "Write", "MultiEdit", "NotebookEdit", "Grep", "Glob":
		return true
	}
	return false
}

// findChains walks one conversation's calls in order. A failed call opens a
// chain on its targets; a later call sharing a target joins it and is
// recategorised as retry. A joining call that runs something and succeeds
// resolves the chain; a chain nobody returns to for chainIdleLimit calls is
// closed unresolved.
func findChains(infos []*callInfo) []*chainState {
	var all []*chainState
	for _, ci := range infos {
		joined := false
		for _, ch := range all {
			if !ch.open || !overlaps(ch.targets, ci.targets) {
				continue
			}
			joined = true
			ch.Length++
			ch.idle = 0
			ci.category = CatRetry
			if ci.Failed {
				ch.failures++
			} else if ci.HasResult && !touches(ci.Tool) {
				ch.Resolved = true
				ch.open = false
			}
			break
		}
		if !joined {
			for _, ch := range all {
				if ch.open {
					ch.idle++
					if ch.idle > chainIdleLimit {
						ch.open = false
					}
				}
			}
			if ci.Failed {
				ch := &chainState{
					Chain:   Chain{Tool: ci.Tool, What: ci.snippet, FirstError: ci.ErrorLine},
					targets: map[string]bool{}, open: true, failures: 1,
				}
				for _, t := range ci.targets {
					ch.targets[t] = true
				}
				all = append(all, ch)
			}
		}
	}
	return all
}

func overlaps(set map[string]bool, ts []string) bool {
	for _, t := range ts {
		if set[t] {
			return true
		}
	}
	return false
}

func sortedCounts(m map[string]int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func categoryCounts(m map[string]int) []Count {
	var out []Count
	for _, c := range CategoryOrder {
		if m[c] > 0 {
			out = append(out, Count{c, m[c]})
		}
	}
	return out
}

func repeated(m map[string]int) []Count {
	var out []Count
	for _, c := range sortedCounts(m) {
		if c.N > 1 {
			out = append(out, c)
		}
	}
	return out
}
