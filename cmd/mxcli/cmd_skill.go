// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/mendixlabs/mxcli/cmd/mxcli/skillpack"
)

var (
	skillPackDir       string
	skillPackNamespace string
	skillPackProject   string
)

// packsFS returns the embedded packs rooted at the pack directory, so callers
// see `<pack>/SKILL.md` rather than `skillpacks/<pack>/SKILL.md`.
func packsFS() (fs.FS, error) {
	return fs.Sub(skillPacksFS, "skillpacks")
}

// targetSkillsDir is where packs are installed. Packs are directory-shaped, so
// they go to .claude/skills/ (which reads a directory as a skill) rather than
// .ai-context/skills/, which is the flat prose set.
func targetSkillsDir(projectDir string) string {
	if skillPackDir != "" {
		return skillPackDir
	}
	return filepath.Join(projectDir, ".claude", "skills")
}

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage skill packs — skills that carry assets, not just prose",
	Long: `Manage skill packs.

A skill pack is a directory: SKILL.md plus the references, spec templates,
scripts and MDL that go with it. Packs are opt-in — unlike the prose skills
written by ` + "`mxcli init`" + `, a pack may install a widget or apply Java actions
to the model, so nothing is installed until you ask for it.`,
}

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available skill packs and which are installed here",
	RunE: func(cmd *cobra.Command, args []string) error {
		fsys, err := packsFS()
		if err != nil {
			return err
		}
		packs, err := skillpack.List(fsys)
		if err != nil {
			return err
		}
		if len(packs) == 0 {
			fmt.Println("No skill packs are bundled in this build.")
			return nil
		}

		dir := targetSkillsDir(".")
		installed := map[string]bool{}
		names, err := skillpack.Installed(dir)
		if err != nil {
			return err
		}
		for _, n := range names {
			installed[n] = true
		}

		for _, p := range packs {
			mark := " "
			if installed[p.Name] {
				mark = "*"
			}
			fmt.Printf("%s %-24s %-8s", mark, p.Name, p.Version)
			if p.MinMendixVersion != "" {
				fmt.Printf(" (Mendix %s+)", p.MinMendixVersion)
			}
			if p.WritesToModel() {
				fmt.Print(" [--apply writes to the model]")
			}
			fmt.Println()
		}
		fmt.Printf("\n* = installed in %s\n", dir)
		return nil
	},
}

var skillAddCmd = &cobra.Command{
	Use:   "add <pack>",
	Short: "Install a skill pack into this project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fsys, err := packsFS()
		if err != nil {
			return err
		}
		pack, err := skillpack.Load(fsys, args[0])
		if err != nil {
			return err
		}
		dir := targetSkillsDir(".")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		opts := skillpack.Options{}
		var ns string
		if pack.NeedsNamespace() {
			ns, err = resolveNamespace()
			if err != nil {
				return err
			}
			// The widget source lands in <dir>/<pack>/widget/, so the build's
			// output target is relative to there.
			//
			// Both sides are made absolute first. filepath.Rel refuses to mix a
			// relative and an absolute path, and the fallback is an ABSOLUTE
			// projectPath baked into a package.json that gets committed — which
			// works on exactly one machine and fails silently everywhere else.
			widgetDir, err1 := filepath.Abs(filepath.Join(dir, pack.Name, "widget"))
			projDir, err2 := filepath.Abs(projectDirForPack())
			rel := projDir
			if err1 == nil && err2 == nil {
				if r, relErr := filepath.Rel(widgetDir, projDir); relErr == nil {
					rel = r
				}
			}
			opts.Vars = skillpack.Vars(ns, filepath.ToSlash(rel))
		}

		res, err := skillpack.InstallWith(fsys, pack.Name, dir, opts)
		if err != nil {
			return err
		}

		switch {
		case !res.Changed():
			fmt.Printf("%s is already up to date in %s\n", pack.Name, dir)
		default:
			fmt.Printf("Installed %s %s into %s\n", pack.Name, pack.Version, dir)
			fmt.Printf("  %d file(s) written", len(res.Written))
			if len(res.Pruned) > 0 {
				fmt.Printf(", %d removed (no longer shipped)", len(res.Pruned))
			}
			fmt.Println()
		}

		if ns != "" {
			fmt.Printf("\nNamespace: %s\n", ns)
			fmt.Printf("  widget id: %s\n", skillpack.WidgetID(ns, "vegachart.VegaChart"))
			fmt.Println("  Set before the build, so the id is right the first time a page references it.")
			fmt.Println("  Renaming later means re-applying every page that carries the widget.")
			fmt.Printf("\nBuild it:\n  cd %s && npm ci && npm run build\n",
				filepath.Join(dir, pack.Name, "widget"))
			fmt.Println("  The .mpk lands in the project's widgets/ — commit it, or every other")
			fmt.Println("  clone of the repo references a widget nobody has.")
			// Without this, the first page authored against the widget fails with
			// "no definition for widget ...", which reads as a packaging problem
			// rather than a step nobody was told about.
			fmt.Println("\nThen let mxcli see it:\n  mxcli widget init -p <app>.mpr")
		}

		// Copying the pack never touches the model. Anything that would is
		// reported as a next step the user runs deliberately — a documentation
		// install that silently added Java actions to the .mpr would be exactly
		// the kind of surprise this repo keeps refusing.
		if pack.WritesToModel() {
			fmt.Println("\nThis pack ships MDL that adds Java actions to the model. It has NOT been applied.")
			for _, m := range pack.Installs.MDL {
				fmt.Printf("  review then apply:  mxcli exec %s -p <app>.mpr\n",
					filepath.Join(dir, pack.Name, filepath.FromSlash(m)))
			}
			fmt.Println("  (the MDL uses a MyModule placeholder — set the target module first)")
		}
		return nil
	},
}

var skillRemoveCmd = &cobra.Command{
	Use:   "remove <pack>",
	Short: "Remove an installed skill pack from this project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir := targetSkillsDir(".")
		removed, err := skillpack.Remove(dir, args[0])
		if err != nil {
			return err
		}
		if !removed {
			fmt.Printf("%s is not installed in %s\n", args[0], dir)
			return nil
		}
		fmt.Printf("Removed %s from %s\n", args[0], dir)
		fmt.Println("Anything the pack applied to the model (Java actions, widgets) is left alone.")
		return nil
	},
}

var skillUpgradeCmd = &cobra.Command{
	Use:   "upgrade [pack]",
	Short: "Re-install installed packs from this binary, pruning files they no longer ship",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fsys, err := packsFS()
		if err != nil {
			return err
		}
		dir := targetSkillsDir(".")
		names, err := skillpack.Installed(dir)
		if err != nil {
			return err
		}
		if len(args) == 1 {
			names = []string{args[0]}
		}
		if len(names) == 0 {
			fmt.Printf("No skill packs installed in %s\n", dir)
			return nil
		}
		quiet := true
		for _, n := range names {
			res, err := skillpack.Install(fsys, n, dir)
			if err != nil {
				return err
			}
			if res.Changed() {
				quiet = false
				fmt.Printf("%s: %d written, %d pruned\n", n, len(res.Written), len(res.Pruned))
			}
		}
		if quiet {
			// Silence is the common case and the only acceptable one, same as
			// the flat-skill sync.
			return nil
		}
		return nil
	},
}

func init() {
	skillCmd.PersistentFlags().StringVar(&skillPackDir, "dir", "",
		"Install packs here instead of ./.claude/skills")
	skillAddCmd.Flags().StringVar(&skillPackNamespace, "namespace", "",
		"Widget namespace for packs that ship a widget (default: derived from the project name)")
	skillAddCmd.Flags().StringVarP(&skillPackProject, "project", "p", "",
		"Path to the .mpr the pack is being installed for")
	skillCmd.AddCommand(skillListCmd, skillAddCmd, skillRemoveCmd, skillUpgradeCmd)
	rootCmd.AddCommand(skillCmd)
}

// resolveNamespace decides the widget namespace for a pack that ships one.
//
// Explicit --namespace wins. Otherwise it comes from the project name, which is
// a default rather than something to apply silently — the caller prints it, so a
// project called App1112 does not quietly become the vendor prefix of a widget
// that ends up somewhere else.
func resolveNamespace() (string, error) {
	if skillPackNamespace != "" {
		return skillpack.NormalizeNamespace(skillPackNamespace)
	}
	mpr := skillPackProject
	if mpr == "" {
		mpr = findMprFile(".")
	}
	if mpr == "" {
		return "", fmt.Errorf("this pack ships a widget, whose id must carry your namespace.\n" +
			"No .mpr found here to derive one from — pass --namespace acme (or -p <app>.mpr)")
	}
	return skillpack.NamespaceFromProject(mpr)
}

// projectDirForPack is the directory the built widget package should land in,
// which is the directory holding the .mpr.
func projectDirForPack() string {
	mpr := skillPackProject
	if mpr == "" {
		mpr = findMprFile(".")
	}
	if mpr == "" {
		return "."
	}
	if abs, err := filepath.Abs(filepath.Dir(mpr)); err == nil {
		return abs
	}
	return filepath.Dir(mpr)
}
