// SPDX-License-Identifier: Apache-2.0

package skillpack

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// A Java package declaration is a class's identity, exactly as a widget id is:
// two projects whose classes share a package are two projects claiming the same
// class, and the symptom is a compile error inside somebody else's module. So
// Java ships with {{MODULE}} / {{MODULE_PATH}} placeholders and `skill add`
// substitutes the destination module's names, on the same three rules the widget
// path already follows — placeholders rather than a real name, a whitelist
// rather than a scan, and drift in either direction refusing the install.

// GeneratedActionsDir is the one subdirectory of a pack's Java that is NOT
// placed.
//
// mxcli writes the action classes itself from the MDL's `CREATE JAVA ACTION …
// AS $$ … $$` bodies, so placing the pack's copies too means two sources of
// truth for the same four files, and applying the MDL immediately overwrites
// what the pack just wrote. They stay in the pack directory to be read.
const GeneratedActionsDir = "actions"

var moduleNameInvalid = regexp.MustCompile(`[^A-Za-z0-9_]+`)

// NormalizeModule validates a Mendix module name and derives its javasource
// package segment.
//
// Mendix lowercases the module name for the package, which is why the two are
// derived from one value here rather than asked for separately — a pack whose
// `package` line and directory disagree does not compile, and the error names
// neither.
func NormalizeModule(in string) (name, pkg string, err error) {
	name = moduleNameInvalid.ReplaceAllString(strings.TrimSpace(in), "")
	if name == "" {
		return "", "", fmt.Errorf("module name %q has no usable characters; "+
			"pass --module with a Mendix module name like ODataPushdown", in)
	}
	if name[0] >= '0' && name[0] <= '9' {
		return "", "", fmt.Errorf("module name %q starts with a digit, which a Java package cannot", in)
	}
	return name, strings.ToLower(name), nil
}

// ModuleVars returns the substitution values for a pack that places Java.
func ModuleVars(module string) (map[string]string, error) {
	name, pkg, err := NormalizeModule(module)
	if err != nil {
		return nil, err
	}
	return map[string]string{"MODULE": name, "MODULE_PATH": pkg}, nil
}

// JavaResult reports what placing a pack's Java did.
type JavaResult struct {
	Dest     string   // javasource/<pkg>, for reporting
	Written  []string // paths written, relative to Dest
	Skipped  []string // already byte-identical
	Refused  []string // present and different — left alone
	Excluded []string // shipped but deliberately not placed (generated actions)
}

// Changed reports whether anything moved on disk.
func (r JavaResult) Changed() bool { return len(r.Written) > 0 }

// InstallJava places a pack's Java sources into projectDir/javasource/<pkg>/.
//
// A file that already exists and differs is REFUSED, never overwritten. This is
// the guard-don't-drop rule the theme package already follows (ADR-0005): from
// the outside, a locally fixed helper and a stale copy look identical, and
// silently replacing 882 lines of somebody's edited parser is not a trade this
// should make on their behalf. The refusal names the files so the choice stays
// with whoever knows which side is right.
func InstallJava(fsys fs.FS, name, projectDir string, opts Options) (JavaResult, error) {
	pack, err := Load(fsys, name)
	if err != nil {
		return JavaResult{}, err
	}
	var res JavaResult
	if len(pack.Installs.Java) == 0 {
		return res, nil
	}
	pkg := opts.Vars["MODULE_PATH"]
	if pkg == "" {
		return res, fmt.Errorf("pack %q places Java, which needs the owning Mendix module; pass --module", name)
	}

	rewrites := map[string]bool{}
	for _, f := range pack.Rewrite.Files {
		rewrites[filepath.ToSlash(f)] = true
	}

	dest := filepath.Join(projectDir, "javasource", pkg)
	res.Dest = filepath.Join("javasource", pkg)

	for _, dir := range pack.Installs.Java {
		root := path.Join(pack.Dir, filepath.ToSlash(dir))
		if _, err := fs.Stat(fsys, root); err != nil {
			return res, fmt.Errorf("pack %q: installs.java names %q, which the pack does not ship: %w", name, dir, err)
		}
		err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)

			// The MDL owns these; see GeneratedActionsDir.
			if top, _, _ := strings.Cut(rel, "/"); top == GeneratedActionsDir {
				res.Excluded = append(res.Excluded, rel)
				return nil
			}

			want, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			// Declared relative to the PACK root, which is where the manifest
			// speaks from — not relative to the java directory being walked.
			if packRel := path.Join(filepath.ToSlash(dir), rel); rewrites[packRel] {
				if want, err = substitute(packRel, want, opts.Vars); err != nil {
					return err
				}
			}

			dst := filepath.Join(dest, filepath.FromSlash(rel))
			if have, readErr := os.ReadFile(dst); readErr == nil {
				if string(have) == string(want) {
					res.Skipped = append(res.Skipped, rel)
				} else {
					res.Refused = append(res.Refused, rel)
				}
				return nil
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(dst, want, 0o644); err != nil {
				return err
			}
			res.Written = append(res.Written, rel)
			return nil
		})
		if err != nil {
			return res, fmt.Errorf("placing pack %q Java: %w", name, err)
		}
	}
	sort.Strings(res.Written)
	sort.Strings(res.Skipped)
	sort.Strings(res.Refused)
	sort.Strings(res.Excluded)
	return res, nil
}
