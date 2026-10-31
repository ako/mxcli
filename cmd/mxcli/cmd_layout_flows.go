// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

// cmd_layout_flows.go re-arranges microflows and nanoflows — the flow
// counterpart of `mxcli layout`, and the answer to "reset the layout" that
// does not bolt a clause onto CREATE MICROFLOW.
//
// It uses the same layout engine as CREATE: a flow laid out here looks exactly
// like the same flow created from MDL without @position. Only coordinates are
// written — the flow's content, IDs and every property MDL cannot express are
// left as stored — which is also why it can lay out a flow drawn in Studio Pro
// without rewriting it through MDL.

var layoutFlowsCmd = &cobra.Command{
	Use:   "flows [Module.Flow ...]",
	Short: "Re-arrange microflows and nanoflows with the CREATE MICROFLOW layout",
	Long: `Re-arrange microflows and nanoflows on their canvas.

The layout is the one CREATE MICROFLOW uses for a flow written without
@position: the main path runs left to right, branches fan out below it, loops
are sized to their bodies. Name the flows to lay out, or pass --module to lay
out every microflow and nanoflow in a module.

Positions change: activity and event positions and sizes, parameter
positions, and the anchors and curves of sequence flows. Content, element IDs
and everything else about the flow are left exactly as stored, apart from
the one documentation line described next.

The style is the project's layout-config.yaml (in .claude/ or next to the
.mpr; see 'mxcli layout config'), or the default when there is none. --style
lays out in a preset instead, to try one. Each flow laid out in a style other
than the default records it as an '@mxcli-layout:' line in its documentation,
which DESCRIBE and CREATE OR MODIFY then keep using for that flow; laying it out
in the default removes the line. That line is the one thing besides positions
this command writes.

A flow whose description does not rebuild into the same graph — something MDL
cannot yet express — is skipped with the reason, never laid out by guesswork.

Running this twice changes nothing the second time. It REPLACES the positions
of every flow it touches, including any arranged by hand; use --dry-run to see
what would move first. Marketplace modules and System are never touched unless
--include-marketplace is given.`,
	Example: `  mxcli layout flows -p app.mpr MyModule.ACT_Order_Submit
  mxcli layout flows -p app.mpr --module MyModule
  mxcli layout flows -p app.mpr --module MyModule --dry-run
  mxcli layout flows -p app.mpr --module MyModule --style true-right-false-down --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLayoutFlows(cmd, args)
	},
}

func init() {
	layoutFlowsCmd.Flags().StringSliceVar(&layoutModules, "module", nil,
		"module whose flows to lay out (repeatable)")
	layoutFlowsCmd.Flags().BoolVar(&layoutDryRun, "dry-run", false,
		"report what would move without writing")
	layoutFlowsCmd.Flags().StringVar(&layoutFlowStyle, "style", "",
		"lay out in this preset instead of the project's layout-config.yaml (happy-line, true-right-false-down)")
	layoutFlowsCmd.Flags().BoolVar(&layoutIncludeMarketplace, "include-marketplace", false,
		"also lay out flows in Marketplace modules (a module update replaces them, so this is normally pointless)")
	layoutCmd.AddCommand(layoutFlowsCmd)
	layoutCmd.AddCommand(layoutConfigCmd)
}

var layoutFlowStyle string

var layoutConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the layout style microflows and nanoflows are laid out with",
	Long: `Show the style CREATE MICROFLOW, CREATE NANOFLOW and 'mxcli layout flows'
lay flows out with, and the file it comes from.

The style is read from layout-config.yaml in the project's .claude/ directory or
next to the .mpr — a committed file, so everyone working on the project lays
flows out the same way:

  flows:
    preset: true-right-false-down   # happy-line (default) | true-right-false-down
    if-else: true-right-else-column # lanes | true-right-else-column

An unknown key or value is an error rather than a silent fallback.`,
	Example: `  mxcli layout config -p app.mpr`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectPath, _ := cmd.Flags().GetString("project")
		if projectPath == "" {
			return fmt.Errorf("no project given: pass -p <project.mpr>")
		}
		exec, logger := newLoggedExecutor("subcommand")
		defer logger.Close()
		defer exec.Close()
		exec.SetQuiet(true)
		connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
		for _, stmt := range connectProg.Statements {
			if err := exec.Execute(stmt); err != nil {
				return err
			}
		}
		style, source, err := exec.FlowLayoutStyle()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Flow layout: %s\nFrom: %s\n", style, source)
		return nil
	},
}

// flowTarget is one flow to lay out.
type flowTarget struct {
	kind string
	name ast.QualifiedName
}

func runLayoutFlows(cmd *cobra.Command, args []string) error {
	projectPath, _ := cmd.Flags().GetString("project")
	if projectPath == "" {
		return fmt.Errorf("no project given: pass -p <project.mpr>")
	}
	if _, err := os.Stat(projectPath); err != nil {
		return fmt.Errorf("project not found: %s", projectPath)
	}
	if len(args) == 0 && len(layoutModules) == 0 {
		return fmt.Errorf("name the flows to lay out, or pass --module: laying out every flow in the project is too broad to do by default")
	}

	exec, logger := newLoggedExecutor("subcommand")
	defer logger.Close()
	defer exec.Close()
	exec.SetQuiet(true)
	if err := exec.SetFlowLayoutStyle(layoutFlowStyle); err != nil {
		return err
	}
	connectProg, _ := visitor.Build(fmt.Sprintf("CONNECT LOCAL '%s'", visitor.QuoteString(projectPath)))
	for _, stmt := range connectProg.Statements {
		if err := exec.Execute(stmt); err != nil {
			return err
		}
	}

	targets, err := flowLayoutTargets(exec, args)
	if err != nil {
		return err
	}
	return layoutFlowTargets(cmd.OutOrStdout(), exec, targets)
}

// flowLayoutTargets resolves the named flows and the flows of every --module.
// A name the project does not have is an error, as a --module typo is in
// `mxcli layout`: otherwise it would report success having done nothing.
func flowLayoutTargets(exec *executor.Executor, args []string) ([]flowTarget, error) {
	var targets []flowTarget
	seen := map[string]bool{}
	add := func(kind, qn string) {
		if seen[qn] {
			return
		}
		seen[qn] = true
		mod, name, _ := strings.Cut(qn, ".")
		targets = append(targets, flowTarget{kind: kind, name: ast.QualifiedName{Module: mod, Name: name}})
	}

	// Both --module and a named flow go through the Marketplace/System filter
	// the domain model layout uses: naming a flow in a Marketplace module is as
	// pointless as naming the module, since the next update replaces both.
	modules, err := exec.Modules()
	if err != nil {
		return nil, err
	}
	if len(layoutModules) > 0 {
		wanted := map[string]string{}
		for _, m := range layoutModules {
			if t := strings.TrimSpace(m); t != "" {
				wanted[strings.ToLower(t)] = t
			}
		}
		mods, err := layoutTargets(modules, wanted)
		if err != nil {
			return nil, err
		}
		for _, m := range mods {
			mfs, nfs, err := exec.ListFlowNames(m.Name)
			if err != nil {
				return nil, err
			}
			for _, qn := range mfs {
				add("microflow", qn)
			}
			for _, qn := range nfs {
				add("nanoflow", qn)
			}
		}
	}

	for _, arg := range args {
		mod, name, ok := strings.Cut(strings.TrimSpace(arg), ".")
		if !ok || mod == "" || name == "" {
			return nil, fmt.Errorf("%q is not a qualified flow name (Module.Flow)", arg)
		}
		mods, err := layoutTargets(modules, map[string]string{strings.ToLower(mod): mod})
		if err != nil {
			return nil, err
		}
		mod = mods[0].Name // Mendix resolves module names case-insensitively
		mfs, nfs, err := exec.ListFlowNames(mod)
		if err != nil {
			return nil, err
		}
		qn := mod + "." + name
		switch {
		case slices.Contains(mfs, qn):
			add("microflow", qn)
		case slices.Contains(nfs, qn):
			add("nanoflow", qn)
		default:
			return nil, fmt.Errorf("no microflow or nanoflow named %s", qn)
		}
	}
	return targets, nil
}

func layoutFlowTargets(out io.Writer, exec *executor.Executor, targets []flowTarget) error {
	changed, unchanged, refused := 0, 0, 0
	for _, t := range targets {
		res, err := exec.LayoutFlow(t.kind, t.name, layoutDryRun)
		if err != nil {
			return fmt.Errorf("%s %s: %w", t.kind, t.name, err)
		}
		switch {
		case res.Refused != "":
			refused++
			fmt.Fprintf(out, "%s: skipped — %s\n", res.Name, res.Refused)
		case !res.Changed():
			unchanged++
			fmt.Fprintf(out, "%s: already laid out (%d objects)\n", res.Name, res.Objects)
		case layoutDryRun:
			changed++
			fmt.Fprintf(out, "%s: %d of %d objects and %d flows would move\n", res.Name, res.Moved, res.Objects, res.Flows)
		default:
			changed++
			fmt.Fprintf(out, "%s: moved %d of %d objects, re-anchored %d flows\n", res.Name, res.Moved, res.Objects, res.Flows)
		}
		if res.Restyled && res.Refused == "" {
			verb := "updates"
			if layoutDryRun {
				verb = "would update"
			}
			fmt.Fprintf(out, "%s: %s the %s line in its documentation\n", res.Name, verb, "@mxcli-layout:")
		}
	}

	switch {
	case len(targets) == 0:
		fmt.Fprintln(out, "Nothing to lay out.")
	case layoutDryRun:
		fmt.Fprintf(out, "Dry run: %d flows would change, %d already laid out, %d skipped. Re-run without --dry-run to apply.\n",
			changed, unchanged, refused)
	default:
		fmt.Fprintf(out, "%d flows laid out, %d already laid out, %d skipped.\n", changed, unchanged, refused)
	}
	return nil
}
