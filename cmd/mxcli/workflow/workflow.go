// SPDX-License-Identifier: Apache-2.0

// Package workflow runs the companion's prepared workflows: a sequence of
// skills an agent follows, held to gates that mxcli commands decide
// (docs/11-proposals/proposal-mxcli-companion-workflows.md).
//
// A workflow is defined by a workflow.json beside its skill's SKILL.md. A run is
// one JSON file under docs/brain/workflows/: the intent, the change note, and
// every gate check with its results. A gate's status is never written by hand:
// it is what the gate's command answered when `mxcli workflow check` ran it.
package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// Gate statuses.
const (
	StatusPass = "pass"
	StatusFail = "fail" // a blocking gate that did not pass
	StatusWarn = "warn" // a reporting gate that did not pass
)

// Gate kinds.
const (
	KindIntent  = "intent"  // the run has an intent
	KindNote    = "note"    // the run has a change note
	KindCommand = "command" // an mxcli command exits 0
	KindScore   = "score"   // `mxcli report --format json` did not fall below the run's baseline
	KindLint    = "lint"    // `mxcli lint --format json` shows no finding of the gate's rules the run did not start with
)

// Gate is one check a run is held to.
type Gate struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Blocking bool   `json:"blocking,omitempty"`
	// Args are the mxcli arguments for command and score gates; {mpr} stands
	// for the project file.
	Args []string `json:"args,omitempty"`
	// RequiresPath fails the gate, without running it, when this path (relative
	// to the project directory) does not exist: a tests gate with no tests.
	RequiresPath string `json:"requiresPath,omitempty"`
	// Rules are the rule-id prefixes a lint gate counts (SEC, MPR007).
	Rules []string `json:"rules,omitempty"`
	// Hint says what to do when the gate does not pass.
	Hint string `json:"hint,omitempty"`
}

// Definition is a workflow, as its workflow.json declares it.
type Definition struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Command string `json:"command"`
	Skill   string `json:"skill"`
	Gates   []Gate `json:"gates"`
}

// GateResult is what one gate answered at one check.
type GateResult struct {
	Gate   string `json:"gate"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// Check is one `mxcli workflow check`.
type Check struct {
	At      string       `json:"at"`
	Results []GateResult `json:"results"`
}

// Run is one use of a workflow.
type Run struct {
	SchemaVersion int      `json:"schema_version"`
	ID            string   `json:"id"`
	Workflow      string   `json:"workflow"`
	Intent        string   `json:"intent"`
	Note          string   `json:"note,omitempty"`
	Status        string   `json:"status"` // open | done
	Created       string   `json:"created"`
	Closed        string   `json:"closed,omitempty"`
	BaselineScore *float64 `json:"baseline_score,omitempty"`
	// BaselineFindings are the lint findings of the run's lint gates when it
	// started; a lint gate blocks only on findings not among them.
	BaselineFindings []string `json:"baseline_findings,omitempty"`
	Checks           []Check  `json:"checks,omitempty"`
}

// LoadDefinitions reads <root>/*/workflow.json under each root in order; a
// later root's definition replaces an earlier one with the same id, so a
// project can adjust the workflows mxcli ships.
func LoadDefinitions(fsys fs.FS, roots ...string) ([]Definition, error) {
	byID := map[string]Definition{}
	for _, root := range roots {
		matches, err := fs.Glob(fsys, path.Join(root, "*", "workflow.json"))
		if err != nil {
			return nil, err
		}
		for _, m := range matches {
			data, err := fs.ReadFile(fsys, m)
			if err != nil {
				return nil, err
			}
			var d Definition
			if err := json.Unmarshal(data, &d); err != nil {
				return nil, fmt.Errorf("%s: %w", m, err)
			}
			if d.ID == "" {
				return nil, fmt.Errorf("%s: no id", m)
			}
			byID[d.ID] = d
		}
	}
	out := make([]Definition, 0, len(byID))
	for _, d := range byID {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Find returns the definition with this id.
func Find(defs []Definition, id string) (Definition, bool) {
	for _, d := range defs {
		if d.ID == id {
			return d, true
		}
	}
	return Definition{}, false
}

// Runner runs mxcli with these arguments.
type Runner interface {
	Run(args []string) (exitCode int, stdout, stderr string, err error)
}

// Evaluate runs every gate of the definition against the run. projectDir is
// where RequiresPath is looked up.
func Evaluate(def Definition, run *Run, r Runner, mpr, projectDir string) []GateResult {
	var out []GateResult
	for _, g := range def.Gates {
		out = append(out, evaluate(g, run, r, mpr, projectDir))
	}
	return out
}

func failOrWarn(g Gate) string {
	if g.Blocking {
		return StatusFail
	}
	return StatusWarn
}

func evaluate(g Gate, run *Run, r Runner, mpr, projectDir string) GateResult {
	res := GateResult{Gate: g.ID}
	switch g.Kind {
	case KindIntent:
		if strings.TrimSpace(run.Intent) == "" {
			res.Status, res.Detail = failOrWarn(g), "the run has no intent"
		} else {
			res.Status = StatusPass
		}
		return res
	case KindNote:
		if strings.TrimSpace(run.Note) == "" {
			res.Status, res.Detail = failOrWarn(g), "write the change note: mxcli workflow note \"…\""
		} else {
			res.Status = StatusPass
		}
		return res
	}

	if g.RequiresPath != "" {
		if _, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(g.RequiresPath))); err != nil {
			res.Status, res.Detail = failOrWarn(g), fmt.Sprintf("no %s/ in the project", g.RequiresPath)
			return res
		}
	}
	args := make([]string, len(g.Args))
	for i, a := range g.Args {
		args[i] = strings.ReplaceAll(a, "{mpr}", mpr)
	}
	code, stdout, stderr, err := r.Run(args)
	if err != nil {
		res.Status, res.Detail = failOrWarn(g), err.Error()
		return res
	}

	switch g.Kind {
	case KindCommand:
		if code == 0 {
			res.Status = StatusPass
		} else {
			res.Status, res.Detail = failOrWarn(g), lastLine(stderr, stdout)
		}
	case KindScore:
		score, perr := overallScore(stdout)
		switch {
		case code != 0 || perr != nil:
			res.Status, res.Detail = failOrWarn(g), "could not score the project: "+lastLine(stderr, stdout)
		case run.BaselineScore == nil:
			res.Status, res.Detail = StatusPass, fmt.Sprintf("score %.1f (no baseline)", score)
		case score+0.05 < *run.BaselineScore:
			res.Status, res.Detail = failOrWarn(g), fmt.Sprintf("score %.1f, down from %.1f at the start", score, *run.BaselineScore)
		default:
			res.Status, res.Detail = StatusPass, fmt.Sprintf("score %.1f (start %.1f)", score, *run.BaselineScore)
		}
	case KindLint:
		current, perr := lintFindings(stdout, g.Rules)
		if perr != nil {
			res.Status, res.Detail = failOrWarn(g), "could not read lint's findings: "+lastLine(stderr, stdout)
			break
		}
		had := map[string]bool{}
		for _, f := range run.BaselineFindings {
			had[f] = true
		}
		var added []string
		for _, f := range current {
			if !had[f.key] {
				added = append(added, f.text)
			}
		}
		if len(added) == 0 {
			res.Status = StatusPass
			if n := len(current); n > 0 {
				res.Detail = fmt.Sprintf("no new findings (%d there before the change)", n)
			}
			break
		}
		res.Status = failOrWarn(g)
		res.Detail = fmt.Sprintf("%d new: %s", len(added), strings.Join(firstN(added, 3), "; "))
	default:
		res.Status, res.Detail = failOrWarn(g), "unknown gate kind "+g.Kind
	}
	return res
}

func firstN(s []string, n int) []string {
	if len(s) > n {
		return append(s[:n:n], "…")
	}
	return s
}

type finding struct{ key, text string }

// lintFindings reads `mxcli lint --format json` and keeps the findings of the
// given rule prefixes, keyed by rule and message so the same finding matches
// across checks.
func lintFindings(lintJSON string, rules []string) ([]finding, error) {
	start := strings.Index(lintJSON, "{")
	if start < 0 {
		return nil, errors.New("no JSON")
	}
	var out struct {
		Violations []struct {
			RuleID     string `json:"ruleId"`
			Message    string `json:"message"`
			Suggestion string `json:"suggestion"`
		} `json:"violations"`
	}
	if err := json.Unmarshal([]byte(lintJSON[start:]), &out); err != nil {
		return nil, err
	}
	var fs []finding
	for _, v := range out.Violations {
		for _, p := range rules {
			if strings.HasPrefix(v.RuleID, p) {
				text := v.RuleID + " " + v.Message
				if v.Suggestion != "" {
					text += " (" + v.Suggestion + ")"
				}
				fs = append(fs, finding{key: v.RuleID + "|" + v.Message, text: text})
				break
			}
		}
	}
	return fs, nil
}

// Findings runs a definition's lint gates, for the baseline taken at start.
func Findings(def Definition, r Runner, mpr string) []string {
	var keys []string
	for _, g := range def.Gates {
		if g.Kind != KindLint {
			continue
		}
		args := make([]string, len(g.Args))
		for i, a := range g.Args {
			args[i] = strings.ReplaceAll(a, "{mpr}", mpr)
		}
		_, stdout, _, err := r.Run(args)
		if err != nil {
			continue
		}
		fs, perr := lintFindings(stdout, g.Rules)
		if perr != nil {
			continue
		}
		for _, f := range fs {
			keys = append(keys, f.key)
		}
	}
	return keys
}

func overallScore(reportJSON string) (float64, error) {
	start := strings.Index(reportJSON, "{")
	if start < 0 {
		return 0, errors.New("no JSON")
	}
	var r struct {
		OverallScore *float64 `json:"overallScore"`
	}
	if err := json.Unmarshal([]byte(reportJSON[start:]), &r); err != nil {
		return 0, err
	}
	if r.OverallScore == nil {
		return 0, errors.New("no overallScore")
	}
	return *r.OverallScore, nil
}

// Score runs a definition's score gate, for the baseline taken at start.
func Score(def Definition, r Runner, mpr string) (float64, bool) {
	for _, g := range def.Gates {
		if g.Kind != KindScore {
			continue
		}
		args := make([]string, len(g.Args))
		for i, a := range g.Args {
			args[i] = strings.ReplaceAll(a, "{mpr}", mpr)
		}
		code, stdout, _, err := r.Run(args)
		if err != nil || code != 0 {
			return 0, false
		}
		s, perr := overallScore(stdout)
		return s, perr == nil
	}
	return 0, false
}

func lastLine(texts ...string) string {
	for _, t := range texts {
		lines := strings.Split(strings.TrimSpace(t), "\n")
		for i := len(lines) - 1; i >= 0; i-- {
			if l := strings.TrimSpace(lines[i]); l != "" {
				if len(l) > 200 {
					l = l[:200] + "…"
				}
				return l
			}
		}
	}
	return "exited non-zero with no output"
}

// BlockingFailures lists the blocking gates that did not pass at the run's
// last check — or every blocking gate when it was never checked.
func BlockingFailures(def Definition, run *Run) []string {
	var last map[string]string
	if n := len(run.Checks); n > 0 {
		last = map[string]string{}
		for _, r := range run.Checks[n-1].Results {
			last[r.Gate] = r.Status
		}
	}
	var out []string
	for _, g := range def.Gates {
		if g.Blocking && (last == nil || last[g.ID] != StatusPass) {
			out = append(out, g.ID)
		}
	}
	return out
}

// Store holds runs, one file each, under docs/brain/workflows/.
type Store struct{ projectDir string }

// NewStore is the store for a project directory.
func NewStore(projectDir string) *Store { return &Store{projectDir: projectDir} }

func (s *Store) dir() string {
	return filepath.Join(s.projectDir, filepath.FromSlash(brain.StoreDir), "workflows")
}

var slugUnsafe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(text string) string {
	s := strings.Trim(slugUnsafe.ReplaceAllString(strings.ToLower(text), "-"), "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	return s
}

// Create starts a run. A docs/brain/ that mxcli did not write is refused, as
// the brain itself refuses it: adding a folder to someone else's notes.
func (s *Store) Create(workflowID, intent string, now time.Time) (*Run, error) {
	brainDir := filepath.Join(s.projectDir, filepath.FromSlash(brain.StoreDir))
	if st, err := os.Stat(brainDir); err == nil && st.IsDir() {
		readme, rerr := os.ReadFile(filepath.Join(brainDir, "README.md"))
		if rerr == nil && !brain.HasMarker(string(readme)) {
			return nil, fmt.Errorf("%s exists but was not written by mxcli; not adding workflow runs to it", brain.StoreDir)
		}
	}
	id := now.Format("2006-01-02") + "-" + workflowID
	if sl := slug(intent); sl != "" {
		id += "-" + sl
	}
	base, n := id, 2
	for {
		if _, err := os.Stat(filepath.Join(s.dir(), id+".json")); errors.Is(err, os.ErrNotExist) {
			break
		}
		id = fmt.Sprintf("%s-%d", base, n)
		n++
	}
	run := &Run{SchemaVersion: 1, ID: id, Workflow: workflowID, Intent: intent, Status: "open", Created: now.UTC().Format(time.RFC3339)}
	return run, s.Save(run)
}

// Save writes the run's file.
func (s *Store) Save(run *Run) error {
	if err := os.MkdirAll(s.dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.dir(), run.ID+".json"), append(data, '\n'), 0o644)
}

// Load reads one run.
func (s *Store) Load(id string) (*Run, error) {
	data, err := os.ReadFile(filepath.Join(s.dir(), id+".json"))
	if err != nil {
		return nil, err
	}
	var r Run
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}
	return &r, nil
}

// All lists every run, newest first.
func (s *Store) All() ([]*Run, error) {
	entries, err := os.ReadDir(s.dir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*Run
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		r, err := s.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out, nil
}

// Open lists the runs not yet closed, newest first.
func (s *Store) Open() ([]*Run, error) {
	all, err := s.All()
	var out []*Run
	for _, r := range all {
		if r.Status == "open" {
			out = append(out, r)
		}
	}
	return out, err
}
