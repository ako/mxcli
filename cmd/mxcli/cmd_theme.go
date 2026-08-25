// SPDX-License-Identifier: Apache-2.0

// cmd_theme.go - `mxcli theme` : apply, build and switch app themes
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/theme"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/spf13/cobra"
)

var themeCmd = &cobra.Command{
	Use:   "theme",
	Short: "Apply, build and switch app themes (styling only — no model changes)",
	Long: `Apply, build and switch themes for a Mendix project.

Three themes ship in the binary: signal (the default — cool slate, one teal
signal colour), ledger (warm paper, hairline rules, serif headings) and console
(dark-first, geometric). Run 'mxcli theme list' to see them and
'mxcli theme show <name>' for a theme's palette and the files it writes.

A theme is a set of files under theme/ — the model (.mpr) is never touched, so
it hot-applies under 'mxcli run --local --watch' and cannot affect a build.
Atlas Core is left untouched too. Each theme is a palette of --mxt-* tokens in
theme/web/custom-variables.scss, a shared map wiring those onto ~60 Atlas
variables, and a partial imported from theme/web/main.scss (which compiles
last). Re-branding is one line: change --mxt-brand.

Every theme ships light and dark palettes. With --variant auto (the default) the
app follows the operating system before first paint and honours a theme-light or
theme-dark class on the root element; --variant light or dark bakes one palette.
Nothing in Mendix sets that class, so 'mxcli theme switcher install' adds a
toggle — that subcommand is the only one here that writes to the model.

A project can carry its own themes too. 'mxcli theme create <name>' scaffolds one
into ` + theme.LocalThemesDir + `/, optionally seeding the palette from a design
file that declares --mxt-* tokens; from then on it behaves like a built-in one.

Several themes can be installed at once as a switchable set — 'mxcli theme apply
signal ledger console' — and then selected at runtime by a class on <html>, with
no rebuild. The first named renders by default. That works because a theme is
almost entirely token values: the Atlas wiring, the recipe layer and the widget
layer are identical across themes and resolve every colour through var(), so one
copy of them serves the whole set.

Every generated block is fenced between mxcli:theme markers whose digest records
what mxcli wrote. Edit inside a fence and a later apply refuses rather than
discarding your work; edit outside it and mxcli never touches your lines.

New projects get the default theme automatically — see 'mxcli new --theme'.`,
}

var themeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the available themes",
	Long: `List the available themes.

Three ship in the binary. With -p, any theme the project keeps in
` + theme.LocalThemesDir + `/ is listed too, marked "local"; a local theme with
the same name as a built-in one shadows it.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := themeProjectDirIfGiven(cmd)
		if err != nil {
			return err
		}
		themes, err := theme.List(dir)
		if err != nil {
			return err
		}
		local := false
		for _, t := range themes {
			marker := " "
			if t.Name == theme.DefaultName {
				marker = "*"
			}
			origin := ""
			if t.Local {
				origin, local = "local", true
			}
			fmt.Printf("%s %-12s %-12s %-6s %s\n", marker, t.Name, t.Title, origin, t.Summary)
		}
		fmt.Printf("\n* = applied by default. Use 'mxcli new --theme none' to opt out.\n")
		if local {
			fmt.Printf("local = from %s/ in this project.\n", theme.LocalThemesDir)
		} else if dir == "" {
			fmt.Printf("Pass -p to also list this project's own themes.\n")
		}
		return nil
	},
}

var themeShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show what a theme contains and which files it writes",
	Long: `Show what a theme contains and which files it writes.

The token list at the end is the theme's vocabulary: those are the names a
design artifact has to declare for 'mxcli theme create --from' to seed a
palette from it.`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := themeProjectDirIfGiven(cmd)
		if err != nil {
			return err
		}
		t, err := theme.Get(dir, args[0])
		if err != nil {
			return err
		}
		origin := "built in"
		if t.Local {
			origin = "local to this project (" + theme.LocalThemesDir + "/" + t.Name + ")"
		}
		fmt.Printf("%s (%s) v%s — %s\n\n%s\n\n%s\n\n", t.Title, t.Name, t.Version, origin, t.Summary, t.Description)
		fmt.Printf("Default palette: %s (auto switches to %s)\n\n", t.DefaultVariant, t.AltVariant())
		if len(t.Colorway) > 0 {
			fmt.Printf("Colorway: %s\n\n", strings.Join(t.Colorway, "  "))
		}
		fmt.Println("Files:")
		for _, f := range t.Files {
			fmt.Printf("  %-42s %-9s %s\n", f.Path, f.Mode, f.Purpose)
		}
		if tokens, err := theme.TokenNames(dir, t.Name); err == nil && len(tokens) > 0 {
			fmt.Printf("\nTokens (%d):\n", len(tokens))
			for _, chunk := range chunkStrings(tokens, 3) {
				fmt.Printf("  %s\n", strings.TrimRight(strings.Join(chunk, "  "), " "))
			}
		}
		return nil
	},
}

// chunkStrings groups a sorted list into rows of n, padded so the columns line
// up in a terminal.
func chunkStrings(in []string, n int) [][]string {
	width := 0
	for _, s := range in {
		if len(s) > width {
			width = len(s)
		}
	}
	var out [][]string
	for i := 0; i < len(in); i += n {
		end := i + n
		if end > len(in) {
			end = len(in)
		}
		row := make([]string, 0, end-i)
		for _, s := range in[i:end] {
			row = append(row, fmt.Sprintf("%-*s", width, s))
		}
		out = append(out, row)
	}
	return out
}

var themeCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Scaffold a project-local theme, optionally seeded from a design",
	Long: `Scaffold a theme this project owns, in ` + theme.LocalThemesDir + `/<name>/.

A created theme is a first-class entry in the registry: 'theme list' shows it,
'theme show' describes it, 'theme apply <name>' installs it. That is the point —
hand-editing a generated block instead would work once and then be refused by
the digest fence on the next apply.

Scaffolding copies an existing theme, so the Atlas wiring and the widget layer —
identical in every theme, and where most of the hard-won detail lives — come
across byte for byte. What you edit is the palette.

--from takes either a theme name or a file:

  --from console            scaffold from the console theme, palette unchanged
  --from design/app.dc.html read --mxt-* declarations out of the file and seed
                            the palette with them (--base picks the scaffold)

Seeding reads plain CSS custom properties, wherever they appear in the file:

  :root { --mxt-brand: #7f5af0; --mxt-ground: #fffffe; }
  @media (prefers-color-scheme: dark) { :root { --mxt-ground: #16161a; } }

Declarations inside a dark block seed the dark palette; everything else seeds
the light one. A token the base theme does not declare is an error, not a
silent no-op — run 'mxcli theme show signal' for the vocabulary.

Nothing is applied. Edit the scaffold, then:

  mxcli theme apply <name> -p app.mpr

Examples:
  mxcli theme create acme -p app.mpr
  mxcli theme create acme -p app.mpr --from console
  mxcli theme create acme -p app.mpr --from design/canvas.dc.html
  mxcli theme create acme -p app.mpr --from tokens.css --base console
  mxcli theme create acme -p app.mpr --from tokens.css --dry-run`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := themeProjectDir(cmd)
		if err != nil {
			return err
		}
		from, _ := cmd.Flags().GetString("from")
		base, _ := cmd.Flags().GetString("base")
		title, _ := cmd.Flags().GetString("title")
		summary, _ := cmd.Flags().GetString("summary")
		force, _ := cmd.Flags().GetBool("force")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		res, err := theme.Create(dir, args[0], theme.CreateOptions{
			From: from, Base: base, Title: title, Summary: summary, Force: force, DryRun: dryRun,
		})
		if err != nil {
			return err
		}

		verb := ""
		if dryRun {
			verb = " (dry run)"
		}
		fmt.Printf("Theme '%s' scaffolded from '%s'%s\n  %s\n", res.Name, res.Base, verb, res.Dir)
		for _, f := range res.Files {
			fmt.Printf("  %-9s %s\n", f.Action, f.Path)
		}
		if res.Tokens != nil {
			fmt.Printf("\nSeeded %d token(s) from %s: %d base, %d dark, %d light.\n",
				res.Tokens.Count(), res.Tokens.Source,
				len(res.Tokens.Base), len(res.Tokens.Dark), len(res.Tokens.Light))
		}
		if !dryRun {
			fmt.Printf("\nEdit the palette, then apply it:\n"+
				"  mxcli theme apply %s -p <app.mpr>\n", res.Name)
		}
		return nil
	},
}

var themeApplyCmd = &cobra.Command{
	Use:   "apply [name...]",
	Short: "Apply a theme to an existing project",
	Long: `Apply a theme to an existing project.

Applying is idempotent: re-running replaces only mxcli's own generated blocks.
A block carrying local edits is reported and left alone unless --force is given.

Name several themes to install a switchable set. Each palette is scoped to its
own root class, so exactly one is ever live and selecting one is a class swap on
<html> — no rebuild, no reload. The first named is the default: the one that
renders before any class is set. Themes not in the set are removed.

With no name, apply refreshes the set the project already has, in its installed
order, and falls back to the default only when it has none.

--variant auto (the default) ships both palettes: the app follows the OS and
honours a theme-light / theme-dark class on the root element. --variant light or
dark bakes a single palette with no switching.


Examples:
  mxcli theme apply -p app.mpr
  mxcli theme apply ledger -p app.mpr
  mxcli theme apply console -p app.mpr --variant dark
  mxcli theme apply signal -p app.mpr --dry-run
  mxcli theme apply signal -p app.mpr --force
  mxcli theme apply signal ledger console -p app.mpr   # switchable set`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := themeProjectDir(cmd)
		if err != nil {
			return err
		}
		// A bare `apply` refreshes whatever the project already has — the whole
		// set, in its installed order, so the default is not silently promoted.
		// Only a project with no theme falls back to the built-in default.
		names := args
		if len(names) == 0 {
			if names, err = theme.ResolveSet(dir, theme.DefaultName); err != nil {
				return err
			}
		}
		force, _ := cmd.Flags().GetBool("force")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		variantFlag, _ := cmd.Flags().GetString("variant")
		variant, err := theme.ParseVariant(variantFlag)
		if err != nil {
			return err
		}

		set, err := theme.ApplySet(dir, names, theme.Options{Force: force, DryRun: dryRun, Variant: variant})
		if set != nil {
			for _, res := range set.Themes {
				printThemeResult(res, dryRun)
			}
		}
		if err != nil {
			return err
		}
		if dryRun || !set.Changed() {
			return nil
		}
		if len(names) > 1 {
			fmt.Printf("\n%d themes installed, switchable at runtime; '%s' renders by default.\n"+
				"Each palette is scoped to :root.%s<name> in one stylesheet — selecting one is a\n"+
				"class swap on <html>, no rebuild. Install the switcher to drive it:\n"+
				"  mxcli theme switcher install -p <app.mpr>\n",
				len(names), names[0], theme.SkinClassPrefix)
		} else if variant == theme.VariantAuto {
			fmt.Printf("\nLight and dark follow the OS. Add 'mxcli theme switcher install -p <app.mpr>'\n" +
				"for a user-facing toggle.\n")
		}
		fmt.Printf("\nRun 'mxcli run --local --watch -p <app.mpr>' to see it; SCSS edits hot-apply.\n")
		return nil
	},
}

var themeRemoveCmd = &cobra.Command{
	Use:   "remove [name...]",
	Short: "Remove a theme's generated blocks from a project",
	Long: `Remove a theme's generated blocks from a project.

With no name, the installed themes are read from the mxcli:theme markers in the
project and all of them are removed; if it has none, that is an error rather
than a silent no-op.

A block carrying local edits is reported and left alone unless --force is given.

Examples:
  mxcli theme remove -p app.mpr
  mxcli theme remove ledger -p app.mpr --dry-run`,
	Args:          cobra.ArbitraryArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := themeProjectDir(cmd)
		if err != nil {
			return err
		}
		// No fallback here. Defaulting to the built-in theme meant that on a
		// project themed with any other one, remove reported every file as
		// unchanged and exited 0 — leaving the theme fully installed.
		names := args
		if len(names) == 0 {
			if names, err = theme.ResolveSet(dir, ""); err != nil {
				return err
			}
		}
		force, _ := cmd.Flags().GetBool("force")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		for _, name := range names {
			res, err := theme.Remove(dir, name, theme.Options{Force: force, DryRun: dryRun})
			printThemeResult(res, dryRun)
			if err != nil {
				return err
			}
		}
		return nil
	},
}

var themeSwitcherCmd = &cobra.Command{
	Use:   "switcher",
	Short: "Install a runtime light/dark switcher (this one does touch the model)",
	Long: `Install a runtime theme switcher.

Unlike 'theme apply', this writes to the model: three JavaScript actions and two
nanoflows. It has to. A theme's light/dark blocks key off a class on the root
element, Mendix ships the slot but nothing that sets it, and there is no
theme-level hook to run script before first paint — so an explicit user choice
has to come from something the client can execute.

The CSS still does most of the work: --variant auto already renders the right
palette before first paint by following the OS. The switcher only covers the
case where a user overrides that.

Use --print to see the MDL without running it.`,
}

var themeSwitcherInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Create the switcher actions and nanoflows in a module",
	Long: `Create the theme switcher's JavaScript actions and nanoflows.

Creates, in the module given by --module:

  ToggleAppTheme      flips light/dark, resolving "follow the OS" first, and
                      remembers the choice in localStorage
  SetAppTheme         sets a theme explicitly; pass "auto" to clear the override
  ApplyStoredTheme    re-applies a remembered choice
  ACT_ToggleTheme     a nanoflow a button can call

Then wire a button wherever it belongs, typically a layout or settings page:

  actionbutton btnTheme (caption: 'Theme', action: nanoflow <Module>.ACT_ToggleTheme)

A click flips the palette and remembers it. The class is set on <html>, so
popups and modals — rendered at <body>, outside any page container — follow too.

Reload behaviour: the app goes back to following the OS. Mendix has no page
on-load event to re-apply the stored value from, and the usual substitute (a
data view with a nanoflow data source) is not authorable by mxcli on either
engine yet. ApplyStoredTheme is installed and ready for it — wire it in Studio
Pro if the choice must survive a reload.

Use --print to see the MDL without running it.

Examples:
  mxcli theme switcher install -p app.mpr
  mxcli theme switcher install -p app.mpr --module Ops
  mxcli theme switcher install --print --module Ops`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		module, _ := cmd.Flags().GetString("module")
		projectPath, _ := cmd.Flags().GetString("project")

		// The skin actions are generated against the set actually installed, so
		// a cycle button can never offer a theme whose CSS is not in the page.
		var skins []string
		if projectPath != "" {
			dir, err := themeProjectDir(cmd)
			if err != nil {
				return err
			}
			if skins, err = theme.InstalledOrder(dir); err != nil {
				return err
			}
		} else if flag, _ := cmd.Flags().GetString("skins"); flag != "" {
			skins = strings.Split(flag, ",")
		}
		script := theme.SwitcherMDL(module, skins)

		if printOnly, _ := cmd.Flags().GetBool("print"); printOnly {
			fmt.Print(script)
			return nil
		}

		if projectPath == "" {
			return fmt.Errorf("--project is required (or use --print to see the MDL)")
		}
		if err := execThemeMDL(projectPath, script); err != nil {
			return err
		}
		fmt.Println()
		fmt.Println(theme.SwitcherNextSteps(module))
		return nil
	},
}

// execThemeMDL runs a generated MDL script against a project, the same way
// `mxcli exec` runs one from a file.
func execThemeMDL(projectPath, script string) error {
	ex, logger := newLoggedExecutor("theme")
	defer logger.Close()
	defer ex.Close()

	connect := fmt.Sprintf("CONNECT LOCAL '%s';", visitor.QuoteString(projectPath))
	prog, errs := visitor.Build(connect)
	if len(errs) > 0 {
		return fmt.Errorf("connecting to %s: %v", projectPath, errs[0])
	}
	if err := ex.ExecuteProgram(prog); err != nil {
		return fmt.Errorf("connecting to %s: %w", projectPath, err)
	}

	prog, errs = visitor.Build(script)
	if len(errs) > 0 {
		return fmt.Errorf("parsing the generated switcher MDL: %v", errs[0])
	}
	return ex.ExecuteProgram(prog)
}

// themeProjectDirIfGiven resolves -p when it was passed and returns "" when it
// was not. Read-only commands work without a project — `theme list` on a fresh
// checkout should show the built-ins rather than fail — but they must not
// silently fall back to the working directory either, which would make a
// project's own themes appear or vanish depending on where the shell happens
// to be.
func themeProjectDirIfGiven(cmd *cobra.Command) (string, error) {
	if p, _ := cmd.Flags().GetString("project"); p == "" {
		return "", nil
	}
	return themeProjectDir(cmd)
}

// themeProjectDir resolves the folder holding the .mpr — the theme/ tree sits
// beside it. Accepts -p pointing at either the .mpr or its directory.
func themeProjectDir(cmd *cobra.Command) (string, error) {
	p, _ := cmd.Flags().GetString("project")
	if p == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return wd, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(abs); err == nil && info.IsDir() {
		return abs, nil
	}
	return filepath.Dir(abs), nil
}

func printThemeResult(res *theme.Result, dryRun bool) {
	if res == nil {
		return
	}
	verb := ""
	if dryRun {
		verb = " (dry run)"
	}
	fmt.Printf("Theme '%s'%s\n", res.Theme, verb)
	for _, f := range res.Files {
		fmt.Printf("  %-9s %s\n", f.Action, f.Path)
	}
}

func init() {
	for _, c := range []*cobra.Command{themeApplyCmd, themeRemoveCmd, themeCreateCmd} {
		c.Flags().StringP("project", "p", "", "Path to the .mpr file or project directory")
		c.Flags().Bool("force", false, "Overwrite blocks that carry local edits")
		c.Flags().Bool("dry-run", false, "Report what would change without writing")
	}
	for _, c := range []*cobra.Command{themeListCmd, themeShowCmd} {
		c.Flags().StringP("project", "p", "", "Path to the .mpr file or project directory (to include its own themes)")
	}
	themeApplyCmd.Flags().String("variant", string(theme.VariantAuto),
		"Light/dark behaviour: auto (follow the OS + honour a theme class), light, or dark")
	themeCreateCmd.Flags().String("from", "",
		"A theme name to scaffold from, or a file declaring --mxt-* tokens to seed the palette")
	themeCreateCmd.Flags().String("base", "",
		"Theme to scaffold from when --from names a file (default: "+theme.DefaultName+")")
	themeCreateCmd.Flags().String("title", "", "Display name (default: the theme name, title-cased)")
	themeCreateCmd.Flags().String("summary", "", "One-line summary shown by 'theme list'")
	themeSwitcherInstallCmd.Flags().StringP("project", "p", "", "Path to the .mpr file")
	themeSwitcherInstallCmd.Flags().String("module", "MyFirstModule", "Module to create the actions in")
	themeSwitcherInstallCmd.Flags().Bool("print", false, "Print the MDL instead of running it")
	themeSwitcherInstallCmd.Flags().String("skins", "",
		"With --print and no project: the switchable set to generate for, comma-separated")
	themeSwitcherCmd.AddCommand(themeSwitcherInstallCmd)
	themeCmd.AddCommand(themeListCmd, themeShowCmd, themeCreateCmd, themeApplyCmd, themeRemoveCmd, themeSwitcherCmd)
	rootCmd.AddCommand(themeCmd)
}
