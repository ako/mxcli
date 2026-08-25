// SPDX-License-Identifier: Apache-2.0

package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// LocalThemesDir is where a project keeps its own themes, relative to the
// folder holding the .mpr. One folder per theme, laid out exactly like an
// embedded one: theme.json plus a files/ tree mirroring the project.
//
// Two constraints fix this path:
//
//   - It must be committed. A theme derived from a design is source, not
//     scratch — the team shares it. That rules out .mxcli/, which `mxcli init`
//     writes into .gitignore.
//   - It must not be compiled. mxbuild's entry point is theme/web/main.scss;
//     it does not glob theme/ for other .scss files, so a sibling folder
//     holding theme *sources* is inert until `theme apply` copies them into
//     theme/web/.
const LocalThemesDir = "theme/mxcli-themes"

// source locates one theme's assets. Embedded themes read from the binary;
// project-local ones read from LocalThemesDir. Everything downstream — walking
// files/, reading theme.json — is written against this, so a theme behaves
// identically whichever side it came from.
type source struct {
	fsys fs.FS
	// root is the directory holding theme.json and files/, relative to fsys.
	root string
	// local marks a project-local theme, for display and for the refusal to
	// overwrite a built-in.
	local bool
}

func (s source) filesRoot() string { return path.Join(s.root, "files") }

// validName rejects anything that is not a single plain path segment.
// Without this a name of "../../etc" would walk out of the themes folder on
// the project-local side, where the FS is a real directory rather than an
// embed.
func validName(name string) error {
	if name == "" {
		return fmt.Errorf("theme name is empty")
	}
	if name != path.Clean(name) || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return fmt.Errorf("invalid theme name %q (one folder name, no path separators)", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("invalid theme name %q (must not start with a dot)", name)
	}
	return nil
}

// embeddedSource returns the source for a built-in theme, without checking
// that it exists.
func embeddedSource(name string) source {
	return source{fsys: assetsFS, root: path.Join(assetsRoot, name)}
}

// localSource returns the source for a project-local theme, without checking
// that it exists. projectDir must be non-empty.
func localSource(projectDir, name string) source {
	return source{
		fsys:  os.DirFS(filepath.Join(projectDir, filepath.FromSlash(LocalThemesDir))),
		root:  name,
		local: true,
	}
}

// sourceFor resolves a theme name to its assets. A project-local theme shadows
// an embedded one of the same name, so a project can retune `signal` in place
// and every command — apply, remove, list, show — follows the local copy.
//
// projectDir may be empty, which restricts the lookup to the embedded set
// (`mxcli new` validates --theme before the project exists).
func sourceFor(projectDir, name string) (source, error) {
	if err := validName(name); err != nil {
		return source{}, err
	}
	if projectDir != "" {
		s := localSource(projectDir, name)
		if _, err := fs.Stat(s.fsys, path.Join(s.root, "theme.json")); err == nil {
			return s, nil
		}
	}
	s := embeddedSource(name)
	if _, err := fs.Stat(s.fsys, path.Join(s.root, "theme.json")); err != nil {
		return source{}, unknownTheme(projectDir, name)
	}
	return s, nil
}

// unknownTheme names what the caller could have meant.
//
// Pointing at `mxcli theme list` was wrong the moment a project could own
// themes: that listing needs -p to see them, so the message sent a user who had
// just run `theme create` to a command that would not show what they created.
// Listing the visible set here needs no second command and cannot go stale.
func unknownTheme(projectDir, name string) error {
	msg := fmt.Sprintf("unknown theme %q; %s", name, availableThemes(projectDir))
	if projectDir == "" {
		return errors.New(msg)
	}
	return fmt.Errorf("%s\n\nTo add one of your own: mxcli theme create %s -p <app.mpr>", msg, name)
}

// availableThemes renders the set visible from projectDir, for an error that
// has to tell the user what they could have meant. It is a sentence fragment
// so callers can compose their own lead-in.
func availableThemes(projectDir string) string {
	available, err := sources(projectDir)
	if err != nil {
		return "the available themes could not be listed"
	}
	names := make([]string, 0, len(available))
	for _, s := range available {
		n := path.Base(s.root)
		if s.local {
			n += " (local)"
		}
		names = append(names, n)
	}
	out := "available: " + strings.Join(names, ", ")
	if projectDir == "" {
		out += "\n\nThat is the built-in set. Pass -p to include the themes a project owns."
	}
	return out
}

// sources returns every theme visible from projectDir, local ones shadowing
// embedded ones of the same name, ordered by name.
func sources(projectDir string) ([]source, error) {
	byName := map[string]source{}

	entries, err := fs.ReadDir(assetsFS, assetsRoot)
	if err != nil {
		return nil, fmt.Errorf("reading embedded themes: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			byName[e.Name()] = embeddedSource(e.Name())
		}
	}

	if projectDir != "" {
		dir := filepath.Join(projectDir, filepath.FromSlash(LocalThemesDir))
		local, err := os.ReadDir(dir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading %s: %w", dir, err)
		}
		for _, e := range local {
			if !e.IsDir() || validName(e.Name()) != nil {
				continue
			}
			s := localSource(projectDir, e.Name())
			// A folder without a theme.json is not a theme. Skipped rather
			// than reported: a project may keep notes or a design export
			// beside its themes, and `theme list` failing on one is worse
			// than ignoring it.
			if _, err := fs.Stat(s.fsys, path.Join(s.root, "theme.json")); err != nil {
				continue
			}
			byName[e.Name()] = s
		}
	}

	names := make([]string, 0, len(byName))
	for n := range byName {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]source, 0, len(names))
	for _, n := range names {
		out = append(out, byName[n])
	}
	return out, nil
}

// load reads and validates a source's theme.json.
func (s source) load(name string) (*Theme, error) {
	// Reached only when a source that existed a moment ago has gone; sourceFor
	// has already produced the helpful message for a name that never resolved.
	raw, err := fs.ReadFile(s.fsys, path.Join(s.root, "theme.json"))
	if err != nil {
		return nil, fmt.Errorf("theme %q: cannot read theme.json: %w", name, err)
	}
	var t Theme
	if err := json.Unmarshal(raw, &t); err != nil {
		return nil, fmt.Errorf("theme %q: malformed theme.json: %w", name, err)
	}
	if t.Name != name {
		return nil, fmt.Errorf("theme %q: theme.json declares name %q", name, t.Name)
	}
	t.Local = s.local
	return &t, nil
}
