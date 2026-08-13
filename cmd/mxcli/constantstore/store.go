// SPDX-License-Identifier: Apache-2.0

// Package constantstore holds constant values that belong to THIS MACHINE and
// must not reach version control.
//
// Mendix has this concept — a configuration value marked private — but from
// 10.9 it is encrypted per user account by Studio Pro, which is Windows/Mac
// only. In a Linux devcontainer or an agent session there is no Studio Pro and
// no such store: nothing can write it and nothing can read it. So for a
// headless run the one safe slot is unreachable, and a constant's default and a
// shared configuration override both go to git.
//
// This is mxcli's own equivalent, and is labelled as mxcli's rather than
// pretending to be Mendix's: a gitignored file next to the project, mode 0600.
// That is the same bar as ~/.mxcli/auth.json — file permissions, not
// encryption. See docs/11-proposals/PROPOSAL_constant_values.md.
package constantstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// FileName is the store's name inside the project's .mxcli directory.
const FileName = "constants.json"

// currentVersion is the on-disk schema version. It exists so a later shape
// (per-configuration values, say) can be added without a reader of this one
// silently misreading it.
const currentVersion = 1

// Store is a project's machine-local constant values.
type Store struct {
	Version   int               `json:"version"`
	Constants map[string]string `json:"constants"`
}

// Path is the store's location for a project.
func Path(projectPath string) string {
	return filepath.Join(filepath.Dir(projectPath), ".mxcli", FileName)
}

// Load reads the store for a project. A missing file is not an error — it is
// the normal case, and means "this machine sets no constants".
func Load(projectPath string) (*Store, error) {
	body, err := os.ReadFile(Path(projectPath))
	if os.IsNotExist(err) {
		return &Store{Version: currentVersion, Constants: map[string]string{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", Path(projectPath), err)
	}
	var s Store
	if err := json.Unmarshal(body, &s); err != nil {
		// Named, not swallowed: silently treating a corrupt store as empty would
		// boot the app with different values than the author set, which is the
		// whole failure class this feature exists to remove.
		return nil, fmt.Errorf("%s is not valid JSON: %w", Path(projectPath), err)
	}
	if s.Version > currentVersion {
		return nil, fmt.Errorf("%s was written by a newer mxcli (version %d, this build understands %d)",
			Path(projectPath), s.Version, currentVersion)
	}
	if s.Constants == nil {
		s.Constants = map[string]string{}
	}
	return &s, nil
}

// Save writes the store back, atomically and 0600.
//
// A store with no constants left is REMOVED rather than written empty, so
// unsetting the last value leaves no file behind claiming to configure
// something.
func Save(projectPath string, s *Store) error {
	path := Path(projectPath)
	if len(s.Constants) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", path, err)
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	s.Version = currentVersion
	body, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	// Temp file + rename so a reader can never see a half-written store, and
	// 0600 from the moment it exists rather than after a chmod — the window
	// matters when the value is an API key.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publishing %s: %w", path, err)
	}
	return nil
}

// Names returns the constants this store sets, sorted.
func (s *Store) Names() []string {
	names := make([]string, 0, len(s.Constants))
	for k := range s.Constants {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
