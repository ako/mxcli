// SPDX-License-Identifier: Apache-2.0

// Package skillpack installs skill packs — skills that carry more than prose.
//
// A flat skill is one Markdown file, and the machinery for those flattens
// everything to a basename. A pack is a directory: SKILL.md plus references/,
// specs/, scripts/ and mdl/. Two rules follow, and both are load-bearing:
//
//  1. Files are written by their path RELATIVE TO THE PACK ROOT, never by
//     basename. Flattening does not error — it produces a plausible directory
//     with files silently overwriting each other (references/install.md and
//     specs/install.md collide), which is the failure mode this repo keeps
//     writing down: a tool accepting what it does not implement is worse than
//     one that rejects it.
//
//  2. Installing prunes files the pack no longer ships. Overwrite-without-delete
//     leaves a v1 asset behind forever, and a stale spec template is worse than
//     a missing one because it still looks current.
//
// The embedded FS is passed in rather than embedded here: go:embed can only
// reach files inside its own package directory, and taking an fs.FS is what
// lets the tests drive the real hazards through fstest.MapFS.
package skillpack

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ManifestName is the per-pack manifest, read from the pack root.
const ManifestName = "pack.yaml"

// Manifest describes a pack. Only Name is required; everything else is optional
// so that a pack can be added before its install story is settled.
type Manifest struct {
	Name             string   `yaml:"name"`
	Version          string   `yaml:"version"`
	Description      string   `yaml:"description"`
	MinMendixVersion string   `yaml:"min_mendix_version"`
	Source           string   `yaml:"source"`
	Installs         Installs `yaml:"installs"`
	Verify           string   `yaml:"verify"`
}

// Installs lists what a pack does to a project beyond copying its own files.
// These are deliberately separate from the copy: `skill add` writes the pack's
// documentation and assets, and only an explicit --apply runs anything that
// touches the model.
type Installs struct {
	Widgets []string `yaml:"widgets"`
	MDL     []string `yaml:"mdl"`
}

// Pack is a manifest plus the directory it was read from.
type Pack struct {
	Manifest
	Dir string // path within the source FS
}

// WritesToModel reports whether installing this pack fully (with --apply) would
// modify the .mpr. Callers use it to decide whether to demand confirmation.
func (p Pack) WritesToModel() bool { return len(p.Installs.MDL) > 0 }

// List returns every pack in the FS, sorted by name. A directory without a
// readable manifest is an error rather than a skip: a pack that silently does
// not appear is indistinguishable from one that was never vendored.
func List(fsys fs.FS) ([]Pack, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("reading pack root: %w", err)
	}
	var packs []Pack
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := Load(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		packs = append(packs, p)
	}
	sort.Slice(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
	return packs, nil
}

// Load reads one pack's manifest.
func Load(fsys fs.FS, dir string) (Pack, error) {
	raw, err := fs.ReadFile(fsys, path.Join(dir, ManifestName))
	if err != nil {
		return Pack{}, fmt.Errorf("pack %q has no readable %s: %w", dir, ManifestName, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return Pack{}, fmt.Errorf("pack %q: parsing %s: %w", dir, ManifestName, err)
	}
	if m.Name == "" {
		return Pack{}, fmt.Errorf("pack %q: %s has no name", dir, ManifestName)
	}
	if m.Name != dir {
		// The directory is what the user types; the manifest name is what
		// everything else keys on. Letting them drift makes `skill remove
		// <name>` unable to find what `skill add <name>` wrote.
		return Pack{}, fmt.Errorf("pack %q: %s declares name %q; they must match", dir, ManifestName, m.Name)
	}
	return Pack{Manifest: m, Dir: dir}, nil
}

// Result reports what an Install did.
type Result struct {
	Pack    string
	Written []string // relative paths written or updated
	Pruned  []string // relative paths removed because the pack no longer ships them
	Skipped []string // relative paths already identical
}

// Changed reports whether anything moved on disk.
func (r Result) Changed() bool { return len(r.Written) > 0 || len(r.Pruned) > 0 }

// Install copies one pack into destDir/<pack-name>/, preserving the pack's
// directory structure, and removes files the pack no longer ships.
//
// destDir is the skills directory of the target project (e.g. .claude/skills).
func Install(fsys fs.FS, name, destDir string) (Result, error) {
	pack, err := Load(fsys, name)
	if err != nil {
		return Result{}, err
	}
	res := Result{Pack: pack.Name}
	target := filepath.Join(destDir, pack.Name)

	shipped := map[string]bool{}

	err = fs.WalkDir(fsys, pack.Dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Relative to the pack root — NOT d.Name(). See the package comment.
		rel, err := filepath.Rel(pack.Dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		shipped[rel] = true

		want, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		dst := filepath.Join(target, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if have, readErr := os.ReadFile(dst); readErr == nil && string(have) == string(want) {
			res.Skipped = append(res.Skipped, rel)
			return nil
		}
		if err := os.WriteFile(dst, want, 0o644); err != nil {
			return err
		}
		res.Written = append(res.Written, rel)
		return nil
	})
	if err != nil {
		return res, fmt.Errorf("installing pack %q: %w", name, err)
	}

	pruned, err := prune(target, shipped)
	if err != nil {
		return res, fmt.Errorf("pruning pack %q: %w", name, err)
	}
	res.Pruned = pruned

	sort.Strings(res.Written)
	sort.Strings(res.Skipped)
	return res, nil
}

// prune removes files under root that the pack no longer ships, then removes any
// directory left empty by that. Files the pack still ships are left alone.
func prune(root string, shipped map[string]bool) ([]string, error) {
	var removed []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil // nothing installed yet
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if shipped[rel] {
			return nil
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		removed = append(removed, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := removeEmptyDirs(root); err != nil {
		return nil, err
	}
	sort.Strings(removed)
	return removed, nil
}

// removeEmptyDirs deletes directories left empty by a prune, deepest first. The
// pack root itself is kept — an installed pack with no files is still installed,
// and removing the root would make Remove and Install disagree.
func removeEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() && p != root {
			dirs = append(dirs, p)
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Deepest first, so a directory emptied by removing its subdirectories is
	// itself removed in the same pass.
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			if err := os.Remove(d); err != nil {
				return err
			}
		}
	}
	return nil
}

// Remove deletes an installed pack from destDir.
func Remove(destDir, name string) (bool, error) {
	// Guard against a name that would escape the skills directory. `skill
	// remove ../../etc` must not be a path traversal.
	//
	// The dot entries need naming explicitly: ".." survives
	// `name == filepath.Base(filepath.Clean(name))`, because Base("..") is "..".
	// A guard built only from that check accepts the single input that matters
	// most here, and RemoveAll would then take the whole skills directory.
	if name == "" || name == "." || name == ".." ||
		strings.ContainsRune(name, filepath.Separator) || strings.ContainsRune(name, '/') {
		return false, fmt.Errorf("invalid pack name %q", name)
	}
	target := filepath.Join(destDir, name)
	if _, err := os.Stat(target); errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err := os.RemoveAll(target); err != nil {
		return false, err
	}
	return true, nil
}

// Installed reports which packs are present in destDir, by directory name.
func Installed(destDir string) ([]string, error) {
	entries, err := os.ReadDir(destDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(destDir, e.Name(), ManifestName)); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
