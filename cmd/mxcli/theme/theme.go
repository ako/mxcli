// SPDX-License-Identifier: Apache-2.0

// Package theme applies mxcli's built-in default styling to a Mendix project.
//
// A theme is a set of files dropped into the project's theme/ folder — no model
// (.mpr) changes at all — so it compiles through Mendix's normal SCSS chain, hot
// applies under `mxcli run --local --watch`, and is removed by deleting a block.
//
// Where the files go was settled against a real Mendix 11.13 project rather than
// assumed, and two placements matter:
//
//   - theme/web/custom-variables.scss is imported by *every* module's theme
//     source (once per module), so it holds declarations only — never rules.
//     Atlas maps these CSS custom properties onto its components and onto the
//     brand-aware pluggable widgets, which is why a token retune re-brands the
//     whole app for free.
//
//   - theme/web/main.scss compiles LAST — after Atlas Core and after every
//     module theme source — so the partial it imports can override any Atlas
//     rule without !important. This is also why a theme must not write to
//     themesource/<name>/: a theme source folder is only compiled when <name>
//     matches a real module in the model, so an invented folder is silently
//     dropped from the build.
package theme

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultName is the theme applied when the user does not choose one.
const DefaultName = "signal"

// NoneName opts out of default styling, leaving Atlas untouched.
const NoneName = "none"

// Variant selects which light/dark behaviour a theme is written with.
type Variant string

const (
	// VariantAuto follows the OS preference and honours a theme-light /
	// theme-dark class on the root element. The default.
	VariantAuto Variant = "auto"
	// VariantLight bakes the light palette with no switching.
	VariantLight Variant = "light"
	// VariantDark bakes the dark palette with no switching.
	VariantDark Variant = "dark"
)

// ParseVariant validates a user-supplied variant name.
func ParseVariant(s string) (Variant, error) {
	switch Variant(s) {
	case VariantAuto, VariantLight, VariantDark:
		return Variant(s), nil
	}
	return "", fmt.Errorf("unknown variant %q (want auto, light or dark)", s)
}

// FileSpec documents one file a theme writes, for `mxcli theme show`.
type FileSpec struct {
	Path    string `json:"path"`
	Mode    string `json:"mode"` // "block" or "verbatim"
	Purpose string `json:"purpose"`
}

// Theme is a named styling package embedded in the mxcli binary.
type Theme struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Version     string   `json:"version"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Colorway    []string `json:"colorway"`
	// DefaultVariant is the palette the theme is written around; the other one
	// is what `auto` switches to. Console is dark-first, Signal and Ledger are
	// light-first.
	DefaultVariant Variant    `json:"defaultVariant"`
	Files          []FileSpec `json:"files"`
	// Local marks a theme that came from the project's own LocalThemesDir
	// rather than from the binary. Set on load, never serialised — where a
	// theme lives is a property of the lookup, not of its theme.json.
	Local bool `json:"-"`
}

// AltVariant is the variant `auto` switches to — the opposite of the default.
func (t *Theme) AltVariant() Variant {
	if t.DefaultVariant == VariantDark {
		return VariantLight
	}
	return VariantDark
}

// FileResult is what applying one file did.
type FileResult struct {
	Path   string
	Action Action
}

// Result is the outcome of an Apply or Remove.
type Result struct {
	Theme string
	Files []FileResult
}

// Changed reports whether anything on disk actually moved.
func (r *Result) Changed() bool {
	for _, f := range r.Files {
		if f.Action != ActionUnchanged && f.Action != ActionSkipped {
			return true
		}
	}
	return false
}

// Options tunes an Apply or Remove.
type Options struct {
	// Force overwrites blocks that carry local edits.
	Force bool
	// DryRun reports what would change without writing.
	DryRun bool
	// Variant selects light/dark behaviour. Empty means VariantAuto.
	Variant Variant
	// KeepOthers leaves other themes' blocks in place. Off by default: two
	// themes both mapping the Atlas leaves would fight in the cascade, and
	// which one won would depend on import order rather than on intent.
	KeepOthers bool
}

// List returns the themes visible from projectDir, ordered by name: the ones
// embedded in the binary plus any the project keeps in LocalThemesDir, with a
// local theme shadowing an embedded one of the same name.
//
// projectDir may be empty to list the embedded set alone.
func List(projectDir string) ([]Theme, error) {
	srcs, err := sources(projectDir)
	if err != nil {
		return nil, err
	}
	themes := make([]Theme, 0, len(srcs))
	for _, s := range srcs {
		t, err := s.load(path.Base(s.root))
		if err != nil {
			return nil, err
		}
		themes = append(themes, *t)
	}
	return themes, nil
}

// Get returns one theme by name, preferring the project's own copy over the
// embedded one. projectDir may be empty to look only in the binary.
func Get(projectDir, name string) (*Theme, error) {
	s, err := sourceFor(projectDir, name)
	if err != nil {
		return nil, err
	}
	return s.load(name)
}

// TokenNames returns the --mxt-* tokens a theme declares, sorted. This is the
// vocabulary a design artifact has to speak to seed the theme, so `theme show`
// prints it and `theme create --from <file>` validates against it.
func TokenNames(projectDir, name string) ([]string, error) {
	src, err := sourceFor(projectDir, name)
	if err != nil {
		return nil, err
	}
	return knownTokens(src)
}

// knownTokens collects every --mxt-* name declared anywhere in a theme's SCSS:
// the default palette in custom-variables.scss and the alt palette in the
// theme's own partial.
func knownTokens(src source) ([]string, error) {
	seen := map[string]bool{}
	root := src.filesRoot()
	err := fs.WalkDir(src.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".scss") {
			return err
		}
		body, err := fs.ReadFile(src.fsys, p)
		if err != nil {
			return err
		}
		for _, n := range declaredTokens(string(body)) {
			seen[n] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

// Installed reports which themes have a block in projectDir, read from
// the mxcli:theme:begin markers rather than assumed.
//
// `theme remove` with no name used to fall back to the default theme, which on a
// project themed with anything else removed nothing, reported every file as
// unchanged and exited 0 — a silent no-op on the documented invocation.
func Installed(projectDir string) ([]string, error) {
	all, err := List(projectDir)
	if err != nil {
		return nil, err
	}
	var found []string
	for _, t := range all {
		paths, err := assetPaths(projectDir, t.Name)
		if err != nil {
			return nil, err
		}
		if themeHasBlockIn(projectDir, paths, t.Name) {
			found = append(found, t.Name)
		}
	}
	return found, nil
}

func themeHasBlockIn(projectDir string, paths map[string]bool, name string) bool {
	for rel := range paths {
		if !isBlockFile(rel) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(projectDir, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		if _, ok := findBlock(string(body), name); ok {
			return true
		}
	}
	return false
}

// Resolve picks the theme a bare `apply` or `remove` should act on: whatever is
// installed, falling back to fallback when nothing is. A project carrying two
// themes is reported rather than guessed at.
func Resolve(projectDir, fallback string) (string, error) {
	installed, err := Installed(projectDir)
	if err != nil {
		return "", err
	}
	switch len(installed) {
	case 0:
		if fallback == "" {
			return "", fmt.Errorf("no mxcli theme found in %s (run `mxcli theme apply` to add one)", projectDir)
		}
		return fallback, nil
	case 1:
		return installed[0], nil
	default:
		return "", fmt.Errorf("%s carries more than one theme (%s); name the one you mean",
			projectDir, strings.Join(installed, ", "))
	}
}

// Apply writes a theme's files into projectDir.
//
// projectDir is the folder holding the .mpr — the theme/ tree sits beside it.
func Apply(projectDir, name string, opts Options) (*Result, error) {
	src, err := sourceFor(projectDir, name)
	if err != nil {
		return nil, err
	}
	t, err := src.load(name)
	if err != nil {
		return nil, err
	}
	if err := requireMendixProject(projectDir); err != nil {
		return nil, err
	}
	if opts.Variant == "" {
		opts.Variant = VariantAuto
	}

	// Only one theme at a time. Both would map the same Atlas leaves, so which
	// palette won would come down to SCSS import order rather than to what the
	// user asked for. Removing first also cleans up the previous theme's fonts.
	if !opts.KeepOthers {
		if err := removeRivalThemes(projectDir, t, opts); err != nil {
			return nil, err
		}
	}

	res := &Result{Theme: name}
	root := src.filesRoot()
	// Edited blocks are collected rather than thrown on the first hit: a user who
	// has hand-tuned two files should see both named once, not discover the second
	// only after dealing with the first.
	var skipped []error

	walkErr := fs.WalkDir(src.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		if err := refuseSelfWrite(rel); err != nil {
			return err
		}
		target := filepath.Join(projectDir, filepath.FromSlash(rel))

		body, err := fs.ReadFile(src.fsys, p)
		if err != nil {
			return err
		}

		var fr FileResult
		if isBlockFile(rel) {
			fr, err = applyBlockFile(target, rel, t, expand(string(body), t, opts), opts)
		} else {
			fr, err = applyVerbatimFile(target, rel, body, opts)
		}
		if err != nil {
			var modified *ErrBlockModified
			if !errors.As(err, &modified) {
				return err
			}
			skipped = append(skipped, err)
		}
		res.Files = append(res.Files, fr)
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}

	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return res, errors.Join(skipped...)
}

// Remove cuts a theme's blocks back out and deletes the files it owns outright.
func Remove(projectDir, name string, opts Options) (*Result, error) {
	return remove(projectDir, name, opts, nil)
}

// remove is Remove with a set of project-relative paths that must survive,
// because another theme is about to write them.
func remove(projectDir, name string, opts Options, protect map[string]bool) (*Result, error) {
	src, err := sourceFor(projectDir, name)
	if err != nil {
		return nil, err
	}
	t, err := src.load(name)
	if err != nil {
		return nil, err
	}

	res := &Result{Theme: name}
	root := src.filesRoot()
	var skipped []error

	walkErr := fs.WalkDir(src.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := strings.TrimPrefix(p, root+"/")
		if err := refuseSelfWrite(rel); err != nil {
			return err
		}
		target := filepath.Join(projectDir, filepath.FromSlash(rel))

		existing, readErr := os.ReadFile(target)
		if os.IsNotExist(readErr) {
			res.Files = append(res.Files, FileResult{Path: rel, Action: ActionUnchanged})
			return nil
		}
		if readErr != nil {
			return readErr
		}

		if !isBlockFile(rel) {
			// A verbatim file another theme is about to write (every theme ships
			// mxcli-fonts/OFL.txt) stays put; deleting it would make the incoming
			// apply report a change on a project that ends up identical.
			if protect[rel] {
				res.Files = append(res.Files, FileResult{Path: rel, Action: ActionUnchanged})
				return nil
			}
			if !opts.DryRun {
				if err := os.Remove(target); err != nil {
					return err
				}
			}
			res.Files = append(res.Files, FileResult{Path: rel, Action: ActionRemoved})
			return nil
		}

		out, action, err := removeBlock(string(existing), t.Name, opts.Force)
		if err != nil {
			var modified *ErrBlockModified
			if !errors.As(err, &modified) {
				return err
			}
			modified.Path = rel
			skipped = append(skipped, modified)
			res.Files = append(res.Files, FileResult{Path: rel, Action: ActionSkipped})
			return nil
		}
		// A file that is entirely ours is deleted rather than left empty — unless
		// the incoming theme is about to write it, in which case the shell is
		// truncated and left for its apply to fill. Truncating is the point: an
		// earlier version returned here without writing, so the outgoing theme's
		// block survived and the incoming apply appended a second one, leaving
		// two themes mapping the same Atlas variables in one file.
		if out == "" && protect[rel] {
			if !opts.DryRun {
				if err := os.WriteFile(target, nil, 0o644); err != nil {
					return err
				}
			}
			res.Files = append(res.Files, FileResult{Path: rel, Action: action})
			return nil
		}
		if out == "" {
			if !opts.DryRun {
				if err := os.Remove(target); err != nil {
					return err
				}
			}
			res.Files = append(res.Files, FileResult{Path: rel, Action: ActionRemoved})
			return nil
		}
		if action != ActionUnchanged && !opts.DryRun {
			if err := os.WriteFile(target, []byte(out), 0o644); err != nil {
				return err
			}
		}
		res.Files = append(res.Files, FileResult{Path: rel, Action: action})
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}
	pruneEmptyThemeDirs(projectDir, src)

	sort.Slice(res.Files, func(i, j int) bool { return res.Files[i].Path < res.Files[j].Path })
	return res, errors.Join(skipped...)
}

// pruneEmptyThemeDirs removes directories the theme introduced once its files
// are gone, so `theme remove` leaves no empty theme/web/mxcli-fonts/ behind.
// os.Remove is the guard: it refuses a directory that still holds anything the
// project put there.
func pruneEmptyThemeDirs(projectDir string, src source) {
	root := src.filesRoot()
	var dirs []string
	_ = fs.WalkDir(src.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == root {
			return err
		}
		dirs = append(dirs, strings.TrimPrefix(p, root+"/"))
		return nil
	})
	// Deepest first, so a nested tree empties from the bottom up.
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, rel := range dirs {
		_ = os.Remove(filepath.Join(projectDir, filepath.FromSlash(rel)))
	}
}

func applyBlockFile(target, rel string, t *Theme, body string, opts Options) (FileResult, error) {
	existing, err := os.ReadFile(target)
	if err != nil && !os.IsNotExist(err) {
		return FileResult{}, err
	}

	out, action, err := applyBlock(string(existing), t.Name, t.Version, body, opts.Force)
	if err != nil {
		var modified *ErrBlockModified
		if errors.As(err, &modified) {
			modified.Path = rel
			return FileResult{Path: rel, Action: ActionSkipped}, modified
		}
		return FileResult{}, err
	}
	if action == ActionUnchanged || opts.DryRun {
		return FileResult{Path: rel, Action: action}, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return FileResult{}, err
	}
	if err := os.WriteFile(target, []byte(out), 0o644); err != nil {
		return FileResult{}, err
	}
	return FileResult{Path: rel, Action: action}, nil
}

func applyVerbatimFile(target, rel string, body []byte, opts Options) (FileResult, error) {
	existing, err := os.ReadFile(target)
	switch {
	case err == nil && string(existing) == string(body):
		return FileResult{Path: rel, Action: ActionUnchanged}, nil
	case err != nil && !os.IsNotExist(err):
		return FileResult{}, err
	}

	action := ActionCreated
	if err == nil {
		action = ActionUpdated
	}
	if opts.DryRun {
		return FileResult{Path: rel, Action: action}, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return FileResult{}, err
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return FileResult{}, err
	}
	return FileResult{Path: rel, Action: action}, nil
}

// expand fills the placeholders a theme asset may carry. Kept deliberately
// tiny — the assets are real SCSS that must stay readable and editable in
// place, so the only things templated are the two values a user chooses at
// apply time.
func expand(body string, t *Theme, opts Options) string {
	r := strings.NewReplacer(
		"{{VARIANT}}", string(opts.Variant),
		"{{THEME}}", t.Name,
	)
	return r.Replace(body)
}

// removeRivalThemes strips every other embedded theme from the project. A
// hand-edited rival block is left alone and reported, same as anywhere else.
//
// Files the incoming theme also ships are protected. Themes share paths —
// every one of them writes theme/web/mxcli-fonts/OFL.txt — so without this the
// rival pass deletes a file that is about to be written again, which shows up
// as an apply that is never idempotent.
func removeRivalThemes(projectDir string, incoming *Theme, opts Options) error {
	all, err := List(projectDir)
	if err != nil {
		return err
	}
	protect, err := assetPaths(projectDir, incoming.Name)
	if err != nil {
		return err
	}
	for _, other := range all {
		if other.Name == incoming.Name {
			continue
		}
		if _, err := remove(projectDir, other.Name, opts, protect); err != nil {
			return fmt.Errorf("removing previous theme %q: %w", other.Name, err)
		}
	}
	return nil
}

// assetPaths is the set of project-relative paths a theme writes.
func assetPaths(projectDir, name string) (map[string]bool, error) {
	src, err := sourceFor(projectDir, name)
	if err != nil {
		return nil, err
	}
	root := src.filesRoot()
	out := map[string]bool{}
	err = fs.WalkDir(src.fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		out[strings.TrimPrefix(p, root+"/")] = true
		return nil
	})
	return out, err
}

// refuseSelfWrite stops a theme from owning files inside the themes folder.
//
// `remove` deletes what a theme owns, so a hand-authored tree with a file at
// theme/mxcli-themes/... would delete a theme's own source the first time the
// project switched themes. Nothing mxcli scaffolds does this; the guard is
// here because these trees are meant to be edited by hand.
func refuseSelfWrite(rel string) error {
	if rel == LocalThemesDir || strings.HasPrefix(rel, LocalThemesDir+"/") {
		return fmt.Errorf("a theme must not write to %s (%s); that folder holds theme sources, and `theme remove` deletes what a theme owns",
			LocalThemesDir, rel)
	}
	return nil
}

// isBlockFile reports whether a file gets the marker treatment. Anything else
// (fonts, licences) is copied byte for byte.
func isBlockFile(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".scss", ".css", ".js":
		return true
	}
	return false
}

// requireMendixProject refuses to scatter theme files into a directory that is
// not a Mendix project. Applying to the wrong folder is silent otherwise: the
// files land, nothing compiles them, and the app just looks unstyled.
func requireMendixProject(dir string) error {
	if entries, err := filepath.Glob(filepath.Join(dir, "*.mpr")); err == nil && len(entries) > 0 {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, "themesource", "atlas_core")); err == nil {
		return nil
	}
	return fmt.Errorf("%s does not look like a Mendix project (no .mpr and no themesource/atlas_core)", dir)
}
