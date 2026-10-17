// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/workflow"
	"github.com/spf13/cobra"
)

// cmd_workflow.go: the companion's prepared workflows (vibe code, PRD to app,
// upgrade, migrate), each a skill held to gates mxcli decides.

var workflowCmd = &cobra.Command{
	Use:   "workflow",
	Short: "Prepared workflows held to quality gates (vibe code, PRD to app, …)",
	Long: `Run a prepared workflow: a sequence of skills an agent follows, held to
gates that mxcli commands decide — security lint, tests, a quality score that
must not fall, a change note. A gate's result is what its command answered when
'workflow check' ran it; nobody ticks a box.

A workflow is defined by a workflow.json beside its skill (in the project's
.claude/skills/ or .ai-context/skills/, else the copy this mxcli ships). A run
is one JSON file under docs/brain/workflows/.

Not to be confused with Mendix workflows (the document type): these are
procedures for building the app.`,
	Example: `  mxcli workflow list
  mxcli workflow start vibe "Let managers approve orders" -p app.mpr
  mxcli workflow check -p app.mpr
  mxcli workflow note "Added Order.Approved and ACT_Order_Approve, granted to Manager, tested" -p app.mpr
  mxcli workflow close -p app.mpr
  mxcli workflow status --json -p app.mpr`,
}

// workflowProject resolves the project file and its directory from -p, else
// the single .mpr in the working directory.
func workflowProject(cmd *cobra.Command) (mpr, dir string, err error) {
	mpr, _ = cmd.Flags().GetString("project")
	if mpr == "" {
		if found := findMprFile("."); found != "" {
			mpr = found
		} else {
			return "", "", errors.New("no .mpr here; pass -p app.mpr")
		}
	}
	abs, err := filepath.Abs(mpr)
	if err != nil {
		return "", "", err
	}
	return abs, filepath.Dir(abs), nil
}

// loadWorkflows reads the shipped definitions, then the project's, which win.
func loadWorkflows(projectDir string) ([]workflow.Definition, error) {
	shipped, err := workflow.LoadDefinitions(skillsFS, "skills")
	if err != nil {
		return nil, err
	}
	byID := map[string]workflow.Definition{}
	for _, d := range shipped {
		byID[d.ID] = d
	}
	if projectDir != "" {
		project, err := workflow.LoadDefinitions(os.DirFS(projectDir), ".ai-context/skills", ".claude/skills")
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		for _, d := range project {
			byID[d.ID] = d
		}
	}
	var out []workflow.Definition
	for _, d := range shipped {
		out = append(out, byID[d.ID])
		delete(byID, d.ID)
	}
	for _, d := range byID { // workflows only the project defines
		out = append(out, d)
	}
	return out, nil
}

// selfRunner runs this same mxcli binary for a gate.
type selfRunner struct{ dir string }

func (r selfRunner) Run(args []string) (int, string, string, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, "", "", err
	}
	c := exec.Command(self, args...)
	c.Dir = r.dir
	c.Env = append(os.Environ(), "MXCLI_QUIET=1")
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	err = c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), out.String(), errb.String(), nil
	}
	return 0, out.String(), errb.String(), err
}

// pickRun is the named run, else the newest open one.
func pickRun(store *workflow.Store, args []string) (*workflow.Run, error) {
	if len(args) > 0 && args[0] != "" {
		return store.Load(args[0])
	}
	open, err := store.Open()
	if err != nil {
		return nil, err
	}
	if len(open) == 0 {
		return nil, errors.New("no open workflow run; start one with: mxcli workflow start <workflow> \"<intent>\"")
	}
	return open[0], nil
}

func printWorkflowJSON(v any) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

func workflowExit(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

var workflowListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the prepared workflows",
	Run: func(cmd *cobra.Command, args []string) {
		_, dir, _ := workflowProject(cmd)
		defs, err := loadWorkflows(dir)
		if err != nil {
			workflowExit(err)
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			printWorkflowJSON(defs)
			return
		}
		for _, d := range defs {
			fmt.Printf("%-10s %-14s %s  (%s)\n", d.ID, d.Title, d.Summary, d.Command)
		}
	},
}

var workflowStartCmd = &cobra.Command{
	Use:   "start <workflow> <intent>",
	Short: "Start a run of a workflow, with its intent in one sentence",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		mpr, dir, err := workflowProject(cmd)
		if err != nil {
			workflowExit(err)
		}
		defs, err := loadWorkflows(dir)
		if err != nil {
			workflowExit(err)
		}
		def, ok := workflow.Find(defs, args[0])
		if !ok {
			workflowExit(fmt.Errorf("no workflow %q (see: mxcli workflow list)", args[0]))
		}
		store := workflow.NewStore(dir)
		run, err := store.Create(def.ID, args[1], time.Now())
		if err != nil {
			workflowExit(err)
		}
		// The quality gate compares against the score the run started from, and
		// a lint gate against the findings it started with.
		if score, ok := workflow.Score(def, selfRunner{dir: dir}, mpr); ok {
			run.BaselineScore = &score
		}
		run.BaselineFindings = workflow.Findings(def, selfRunner{dir: dir}, mpr)
		if err := store.Save(run); err != nil {
			workflowExit(err)
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			printWorkflowJSON(run)
			return
		}
		fmt.Printf("Started %s (%s): %s\n", run.ID, def.Title, run.Intent)
		if run.BaselineScore != nil {
			fmt.Printf("Quality baseline: %.1f\n", *run.BaselineScore)
		}
		if n := len(run.BaselineFindings); n > 0 {
			fmt.Printf("Security baseline: %d finding(s) already there; the change must not add any\n", n)
		}
		fmt.Printf("Follow the %s skill; run 'mxcli workflow check' to see the gates.\n", def.Skill)
	},
}

var workflowCheckCmd = &cobra.Command{
	Use:   "check [run]",
	Short: "Run the gates of a run (the newest open one by default) and record the results",
	Long: `Run every gate of the run's workflow and record the results in the run's
file. Exits 1 when a blocking gate does not pass, so an agent cannot call the
work done while one fails.`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		mpr, dir, err := workflowProject(cmd)
		if err != nil {
			workflowExit(err)
		}
		store := workflow.NewStore(dir)
		run, err := pickRun(store, args)
		if err != nil {
			workflowExit(err)
		}
		defs, err := loadWorkflows(dir)
		if err != nil {
			workflowExit(err)
		}
		def, ok := workflow.Find(defs, run.Workflow)
		if !ok {
			workflowExit(fmt.Errorf("run %s uses workflow %q, which is not defined here", run.ID, run.Workflow))
		}
		results := workflow.Evaluate(def, run, selfRunner{dir: dir}, mpr, dir)
		run.Checks = append(run.Checks, workflow.Check{At: time.Now().UTC().Format(time.RFC3339), Results: results})
		if err := store.Save(run); err != nil {
			workflowExit(err)
		}
		blocked := workflow.BlockingFailures(def, run)
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			printWorkflowJSON(map[string]any{"run": run.ID, "results": results, "blocked": blocked})
		} else {
			titles := map[string]workflow.Gate{}
			for _, g := range def.Gates {
				titles[g.ID] = g
			}
			fmt.Printf("%s — %s\n", run.ID, run.Intent)
			for _, r := range results {
				g := titles[r.Gate]
				mark := map[string]string{workflow.StatusPass: "✓", workflow.StatusFail: "✗", workflow.StatusWarn: "!"}[r.Status]
				line := fmt.Sprintf("  %s %-12s", mark, g.Title)
				if r.Detail != "" {
					line += " " + r.Detail
				}
				if r.Status != workflow.StatusPass && g.Hint != "" {
					line += " — " + g.Hint
				}
				fmt.Println(line)
			}
		}
		if len(blocked) > 0 {
			if asJSON, _ := cmd.Flags().GetBool("json"); !asJSON {
				fmt.Fprintf(os.Stderr, "Blocked by: %s\n", strings.Join(blocked, ", "))
			}
			os.Exit(1)
		}
	},
}

var workflowNoteCmd = &cobra.Command{
	Use:   "note [run] <text>",
	Short: "Write the run's change note: what changed and why, in a few lines",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		_, dir, err := workflowProject(cmd)
		if err != nil {
			workflowExit(err)
		}
		store := workflow.NewStore(dir)
		text, runArgs := args[len(args)-1], args[:len(args)-1]
		run, err := pickRun(store, runArgs)
		if err != nil {
			workflowExit(err)
		}
		run.Note = text
		if err := store.Save(run); err != nil {
			workflowExit(err)
		}
		fmt.Printf("Change note written for %s\n", run.ID)
	},
}

var workflowCloseCmd = &cobra.Command{
	Use:   "close [run]",
	Short: "Close a run; refused while a blocking gate did not pass at its last check",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		_, dir, err := workflowProject(cmd)
		if err != nil {
			workflowExit(err)
		}
		store := workflow.NewStore(dir)
		run, err := pickRun(store, args)
		if err != nil {
			workflowExit(err)
		}
		defs, err := loadWorkflows(dir)
		if err != nil {
			workflowExit(err)
		}
		def, _ := workflow.Find(defs, run.Workflow)
		if blocked := workflow.BlockingFailures(def, run); len(blocked) > 0 {
			workflowExit(fmt.Errorf("not closing %s: blocking gates did not pass at its last check: %s (run 'mxcli workflow check')",
				run.ID, strings.Join(blocked, ", ")))
		}
		run.Status = "done"
		run.Closed = time.Now().UTC().Format(time.RFC3339)
		if err := store.Save(run); err != nil {
			workflowExit(err)
		}
		fmt.Printf("Closed %s\n", run.ID)
	},
}

var workflowStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the open runs and their last gate results",
	Run: func(cmd *cobra.Command, args []string) {
		_, dir, err := workflowProject(cmd)
		if err != nil {
			workflowExit(err)
		}
		open, err := workflow.NewStore(dir).Open()
		if err != nil {
			workflowExit(err)
		}
		defs, _ := loadWorkflows(dir)
		type status struct {
			*workflow.Run
			Title   string                `json:"title"`
			Last    []workflow.GateResult `json:"last"`
			Blocked []string              `json:"blocked"`
		}
		var out []status
		for _, r := range open {
			def, _ := workflow.Find(defs, r.Workflow)
			s := status{Run: r, Title: def.Title, Blocked: workflow.BlockingFailures(def, r)}
			if n := len(r.Checks); n > 0 {
				s.Last = r.Checks[n-1].Results
			}
			s.Run.Checks = nil // the full history stays in the file
			out = append(out, s)
		}
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			if out == nil {
				out = []status{}
			}
			printWorkflowJSON(out)
			return
		}
		if len(out) == 0 {
			fmt.Println("No open workflow run.")
			return
		}
		for _, s := range out {
			fmt.Printf("%s  %s — %s\n", s.ID, s.Title, s.Intent)
			for _, r := range s.Last {
				fmt.Printf("  %-5s %s %s\n", r.Status, r.Gate, r.Detail)
			}
		}
	},
}

func init() {
	for _, c := range []*cobra.Command{workflowListCmd, workflowStartCmd, workflowCheckCmd, workflowNoteCmd, workflowCloseCmd, workflowStatusCmd} {
		c.Flags().StringP("project", "p", "", "Path to the .mpr file (default: the one in this directory)")
		workflowCmd.AddCommand(c)
	}
	rootCmd.AddCommand(workflowCmd)
}
