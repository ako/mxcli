// SPDX-License-Identifier: Apache-2.0

// cmd_brain.go - `mxcli brain` : project knowledge mxcli cannot compute
package main

import (
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var brainCmd = &cobra.Command{
	Use:   "brain",
	Short: "Project knowledge mxcli cannot compute (docs/brain/)",
	Long: `Record and check the project knowledge mxcli cannot compute.

Why a pattern was chosen here, which marketplace version broke what, what a
recurring mxbuild error means in this app. Anything mxcli CAN answer does not
belong here — entities, microflows, pages, bindings and references are all
queryable, and a note that transcribes them is a note that will disagree with
the project.

Entries live in docs/brain/, committed and reviewed like any other change. An
entry's anchors decide its file: @Sales.Order puts it in modules/Sales.md, and
an entry with no anchor is cross-cutting and lives in project.md. That is what
lets a session load the shards for the modules it is touching instead of the
whole store.

An agent captures; a person promotes. The queue is in .mxcli/ and is git-ignored,
so nothing reaches a pull request until someone has looked at it.`,
	Example: `  mxcli brain init -p app.mpr
  mxcli brain capture "Orders are committed by Finance, not Sales" -a @Sales.Order -a @Finance.ACT_Post -p app.mpr
  mxcli brain staged -p app.mpr
  mxcli brain promote a1b2c3 -p app.mpr
  mxcli brain check -p app.mpr
  mxcli brain show -p app.mpr`,
}

var brainInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create docs/brain/ (refuses a docs/brain/ it did not write)",
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		written, err := brain.NewStore(dir).Init()
		if err != nil {
			brainFatal(err)
		}
		if len(written) == 0 {
			fmt.Printf("Already initialised: %s\n", filepath.Join(dir, brain.StoreDir))
			return
		}
		for _, p := range written {
			fmt.Printf("Created %s\n", p)
		}
		fmt.Println("\nCapture something with:  mxcli brain capture \"<text>\" -a @Module.Element")
	},
}

var brainCaptureCmd = &cobra.Command{
	Use:   "capture <text>",
	Short: "Queue something worth remembering (does not commit it)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		anchors, _ := cmd.Flags().GetStringSlice("anchor")
		e, err := brain.NewEntry(args[0], anchors, time.Now())
		if err != nil {
			brainFatal(err)
		}
		added, err := brain.NewQueue(dir).Append(e)
		if err != nil {
			brainFatal(err)
		}
		if !added {
			fmt.Printf("Already queued as %s — not added again.\n", e.ID)
			return
		}
		fmt.Printf("Queued %s -> would promote into %s\n", e.ID, shardLabel(e.Shard()))
		fmt.Println("Review with 'mxcli brain staged'; commit it with 'mxcli brain promote " + e.ID + "'.")
	},
}

var brainStagedCmd = &cobra.Command{
	Use:   "staged",
	Short: "List the queue, with the shard each entry would land in",
	Run: func(cmd *cobra.Command, args []string) {
		entries, err := brain.NewQueue(brainProjectDir(cmd)).Load()
		if err != nil {
			brainFatal(err)
		}
		if len(entries) == 0 {
			fmt.Println("Nothing staged.")
			return
		}
		for _, e := range entries {
			fmt.Printf("%s  %-14s  %s\n", e.ID, shardLabel(e.Shard()), e.Title)
			if len(e.Anchors) > 0 {
				fmt.Printf("        %s\n", strings.Join(e.Anchors, " "))
			}
		}
		fmt.Printf("\n%d staged. Promote with 'mxcli brain promote <id>'.\n", len(entries))
	},
}

var brainPromoteCmd = &cobra.Command{
	Use:   "promote <id>",
	Short: "Write a staged entry into its shard (the human step)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		store, queue := brain.NewStore(dir), brain.NewQueue(dir)
		if !store.Exists() {
			brainFatal(fmt.Errorf("no store yet — run 'mxcli brain init' first"))
		}
		e, ok, err := queue.Get(args[0])
		if err != nil {
			brainFatal(err)
		}
		if !ok {
			brainFatal(fmt.Errorf("no staged entry with id %s", args[0]))
		}
		shard := e.Shard()
		if to, _ := cmd.Flags().GetString("to"); to != "" {
			shard = to
		}
		if err := store.Promote(e, shard); err != nil {
			brainFatal(err)
		}
		if _, err := queue.Drop(e.ID); err != nil {
			brainFatal(err)
		}
		fmt.Printf("Promoted %s into %s\n", e.ID, store.ShardPath(shard))
	},
}

var brainDropCmd = &cobra.Command{
	Use:   "drop <id>",
	Short: "Remove an entry from the queue or from its shard",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dir := brainProjectDir(cmd)
		if dropped, err := brain.NewQueue(dir).Drop(args[0]); err != nil {
			brainFatal(err)
		} else if dropped {
			fmt.Printf("Dropped %s from the queue.\n", args[0])
			return
		}
		shard, deletedFile, err := brain.NewStore(dir).Drop(args[0])
		if err != nil {
			brainFatal(err)
		}
		if shard == "" {
			brainFatal(fmt.Errorf("no entry with id %s, staged or committed", args[0]))
		}
		fmt.Printf("Dropped %s from %s\n", args[0], shardLabel(shard))
		if deletedFile {
			fmt.Printf("%s had no entries left and was removed.\n", shardLabel(shard))
		}
	},
}

var brainShowCmd = &cobra.Command{
	Use:   "show [shard]",
	Short: "Size and headroom per shard (computed, never written down)",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		store := brain.NewStore(brainProjectDir(cmd))
		if !store.Exists() {
			fmt.Println("No store yet. Create one with 'mxcli brain init'.")
			return
		}
		usage, err := store.Usage()
		if err != nil {
			brainFatal(err)
		}
		// Width is computed from the names actually present: a module shard is
		// named after its module, and those run long.
		width := len("SHARD")
		for _, u := range usage {
			if n := len(shardLabel(u.Shard)); n > width {
				width = n
			}
		}
		fmt.Printf("%-*s %8s %8s %12s\n", width, "SHARD", "ENTRIES", "LINES", "HEADROOM")
		for _, u := range usage {
			if len(args) == 1 && u.Shard != args[0] {
				continue
			}
			note := ""
			if u.Over() {
				note = "  OVER CAP"
			}
			fmt.Printf("%-*s %8d %8d %7d/%-4d%s\n", width, shardLabel(u.Shard), u.Entries, u.Lines, u.Headroom(), u.Cap, note)
		}
	},
}

var brainCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Do the anchors still resolve, and is every entry in the right shard?",
	Long: `Validate the store against the model.

Two independent questions. Each anchor is resolved, and reported as one of
three states — only "not found" is a failure. An anchor whose target exists but
is of a document type the catalog does not index is reported as NOT INDEXABLE
and passes: treating it as missing would demand edits to entries that are
perfectly current.

Separately, each entry is checked for being in the right shard. That is a second
axis, not a fourth state — an anchor can resolve perfectly and the entry still
sit in the wrong file. At least one anchor must belong to the entry's shard;
anchors into other modules are fine, because a fact can genuinely span two.`,
	Run: func(cmd *cobra.Command, args []string) {
		projectPath := brainProjectPath(cmd)
		store := brain.NewStore(filepath.Dir(projectPath))
		if !store.Exists() {
			fmt.Println("No store yet. Create one with 'mxcli brain init'.")
			return
		}
		shards, err := store.ListShards()
		if err != nil {
			brainFatal(err)
		}
		if changed, _ := cmd.Flags().GetBool("changed"); changed {
			shards, err = changedShards(filepath.Dir(projectPath), shards)
			if err != nil {
				brainFatal(err)
			}
			if len(shards) == 0 {
				fmt.Println("No brain shards changed.")
				return
			}
		}

		resolver, closeFn, err := openBrainResolver(projectPath)
		if err != nil {
			brainFatal(err)
		}
		defer closeFn()

		rep, err := brain.Check(store, resolver, shards)
		if err != nil {
			brainFatal(err)
		}
		if ci, _ := cmd.Flags().GetBool("ci"); ci {
			printBrainReportCI(rep)
		} else {
			printBrainReport(rep)
		}
		if rep.Failed() {
			os.Exit(1)
		}
	},
}

// printBrainReportCI emits one stable, greppable line per problem and nothing
// at all when the store is clean — what a CI log wants. Informational states
// (a not-indexable anchor) are omitted rather than printed, because a line in a
// CI log reads as something to fix.
func printBrainReportCI(rep brain.Report) {
	for _, m := range rep.Malformed {
		fmt.Printf("brain: malformed entry: %s\n", m)
	}
	for _, f := range rep.Findings {
		if f.State != brain.NotFound {
			continue
		}
		fmt.Printf("brain: %s: %s: anchor %s not found\n", shardLabel(f.Shard), f.EntryID, f.Anchor)
	}
	for _, m := range rep.Misfiled {
		fmt.Printf("brain: %s: %s: misfiled, resolves in %s\n", shardLabel(m.Shard), m.EntryID, m.Belongs)
	}
}

func printBrainReport(rep brain.Report) {
	for _, m := range rep.Malformed {
		fmt.Printf("MALFORMED   %s\n", m)
	}
	for _, f := range rep.Findings {
		label := "NOT FOUND  "
		if f.State == brain.NotIndexable {
			label = "not indexable"
		}
		fmt.Printf("%-13s %s  %s (%s: %q)\n", label, f.Anchor, shardLabel(f.Shard), f.EntryID, f.Title)
	}
	for _, m := range rep.Misfiled {
		belongs := m.Belongs
		if belongs == "" {
			belongs = "unknown"
		}
		fmt.Printf("MISFILED      %s: %q is in %s but resolves in %s\n",
			m.EntryID, m.Title, shardLabel(m.Shard), belongs)
	}
	fmt.Printf("\n%d entries, %d anchors, %d resolved, across %d shard(s).\n",
		rep.Entries, rep.Anchors, rep.ResolvedN, len(rep.Shards))
	if !rep.Failed() {
		fmt.Println("OK")
	}
}

// changedShards narrows a check to the shards a diff touches. That is the cheap
// half of the CI answer: the catalog still has to be built once, but nothing
// pays to re-check shards nobody edited.
func changedShards(projectDir string, all []string) ([]string, error) {
	out, err := runGit(projectDir, "diff", "--name-only", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("--changed needs a git repository: %w", err)
	}
	staged, err := runGit(projectDir, "diff", "--name-only", "--cached")
	if err != nil {
		return nil, err
	}
	touched := map[string]bool{}
	for _, line := range strings.Split(out+"\n"+staged, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, brain.StoreDir+"/") {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(line), ".md")
		touched[base] = true
	}
	var shards []string
	for _, s := range all {
		if touched[s] {
			shards = append(shards, s)
		}
	}
	sort.Strings(shards)
	return shards, nil
}

// catalogResolver answers anchors from the catalog, falling back to a
// type-agnostic unit lookup for the types the catalog's objects view does not
// cover. Without that fallback an anchor to such a document reads as missing,
// which is a false staleness signal (A1).
type catalogResolver struct {
	cat *catalog.Catalog
	be  backend.FullBackend
}

func (r *catalogResolver) Resolve(a brain.Anchor) (brain.Resolution, error) {
	if a.IsMember() {
		rows, err := r.query(fmt.Sprintf(
			"SELECT ModuleName FROM attributes_data WHERE EntityQualifiedName = '%s' AND Name = '%s' LIMIT 1",
			sqlLiteral(a.QualifiedName()), sqlLiteral(a.Member)))
		if err != nil {
			return brain.Resolution{}, err
		}
		if len(rows) == 1 {
			return brain.Resolution{State: brain.Resolved, Module: brainCell(rows[0][0]), Kind: "attribute"}, nil
		}
		// The member is gone. Whether its entity survives changes nothing: the
		// anchor as written names something that is not there.
		return brain.Resolution{State: brain.NotFound}, nil
	}

	rows, err := r.query(fmt.Sprintf(
		"SELECT ObjectType, ModuleName, Name FROM objects WHERE QualifiedName = '%s' LIMIT 1",
		sqlLiteral(a.QualifiedName())))
	if err != nil {
		return brain.Resolution{}, err
	}
	if len(rows) == 1 {
		objectType, moduleName, name := brainCell(rows[0][0]), brainCell(rows[0][1]), brainCell(rows[0][2])
		// A MODULE row carries no ModuleName — it IS the module — so the
		// misfiling comparison has to be given the module's own name.
		if objectType == "MODULE" {
			moduleName = name
		}
		return brain.Resolution{State: brain.Resolved, Module: moduleName, Kind: strings.ToLower(objectType)}, nil
	}

	if a.Element == "" {
		return brain.Resolution{State: brain.NotFound}, nil // a module is always indexed
	}
	unit, err := r.be.FindDocumentUnit(a.Module, a.Element)
	if err != nil || unit == nil {
		return brain.Resolution{State: brain.NotFound}, nil
	}
	return brain.Resolution{State: brain.NotIndexable, Module: a.Module, Kind: unit.Kind}, nil
}

func (r *catalogResolver) query(sql string) ([][]any, error) {
	res, err := r.cat.Query(sql)
	if err != nil {
		return nil, err
	}
	return res.Rows, nil
}

// sqlLiteral escapes a value for a SQL string literal. Anchors are validated as
// Mendix identifiers before they get here, so nothing can reach this with a
// quote in it — the escape is belt and braces, not the guard.
func sqlLiteral(s string) string { return strings.ReplaceAll(s, "'", "''") }

func brainCell(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func openBrainResolver(projectPath string) (brain.Resolver, func(), error) {
	// Catalog progress goes to stderr so stdout carries only the report — the
	// same split cmd_lint.go makes, and what lets `brain check` be piped.
	exec, logger := newLoggedExecutorTo("subcommand", os.Stderr)
	cleanup := func() { logger.Close(); exec.Close() }

	for _, src := range []string{
		fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)),
		"REFRESH CATALOG",
	} {
		prog, errs := visitor.Build(src)
		if len(errs) > 0 {
			cleanup()
			return nil, nil, errs[0]
		}
		for _, stmt := range prog.Statements {
			if err := exec.Execute(stmt); err != nil {
				cleanup()
				return nil, nil, fmt.Errorf("%s: %w", src, err)
			}
		}
	}
	return &catalogResolver{cat: exec.Catalog(), be: exec.Backend()}, cleanup, nil
}

// runGit shells out in the project directory. Only used by --changed, where the
// question is which files the working tree has touched.
func runGit(dir string, args ...string) (string, error) {
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func shardLabel(shard string) string {
	if shard == brain.ProjectShard {
		return "project.md"
	}
	return shard + ".md"
}

func brainProjectPath(cmd *cobra.Command) string {
	p, _ := cmd.Flags().GetString("project")
	if p == "" {
		fmt.Fprintln(os.Stderr, "Error: --project (-p) is required")
		os.Exit(1)
	}
	return p
}

func brainProjectDir(cmd *cobra.Command) string { return filepath.Dir(brainProjectPath(cmd)) }

func brainFatal(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}

func init() {
	for _, c := range []*cobra.Command{
		brainInitCmd, brainCaptureCmd, brainStagedCmd, brainPromoteCmd,
		brainDropCmd, brainShowCmd, brainCheckCmd,
	} {
		c.Flags().StringP("project", "p", "", "Path to the .mpr file")
	}
	brainCaptureCmd.Flags().StringSliceP("anchor", "a", nil,
		"Anchor into the model (@Module, @Module.Element, @Module.Entity.Attribute); repeatable")
	brainPromoteCmd.Flags().String("to", "",
		"Override the derived shard (use 'project' for a cross-cutting fact)")
	brainCheckCmd.Flags().Bool("changed", false, "Only check shards touched by the working tree")
	brainCheckCmd.Flags().Bool("ci", false, "Machine-friendly output for CI")

	brainCmd.AddCommand(brainInitCmd, brainCaptureCmd, brainStagedCmd,
		brainPromoteCmd, brainDropCmd, brainShowCmd, brainCheckCmd)
	rootCmd.AddCommand(brainCmd)
}
