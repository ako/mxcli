// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ignoreStatus is what we could establish about the machine store's path.
type ignoreStatus int

const (
	ignoreConfirmed  ignoreStatus = iota // git says the path is ignored
	ignoreUnverified                     // no git, or not a repository — cannot tell
	ignoreNotIgnored                     // git says the path WOULD be committed
)

// gitignoreEntry is what mxcli adds, with the reason attached. `.mxcli/` also
// holds the run/test handshake and logs, none of which belong in a repository.
const gitignoreEntry = "\n# mxcli working directory: machine-local constant values (may contain\n" +
	"# secrets), the run/test handshake, and runtime logs. Never commit these.\n.mxcli/\n"

// ensureStoreIgnored makes the machine store's directory git-ignored, then
// checks that it actually is.
//
// Both halves matter. `mxcli init` writes a .gitignore only when the project has
// none, and a Mendix project usually already has one — so the entry this layer's
// whole promise rests on ("never committed, never shared") could simply be
// absent. Adding it is not enough either: a later negation rule, or a path
// already tracked, can defeat it, and the only authority on that is git itself.
//
// A caller writing a secret must refuse on ignoreNotIgnored. Unverified is not
// the same thing — a project outside version control has nothing to leak into.
func ensureStoreIgnored(projectPath string) (ignoreStatus, error) {
	dir := filepath.Dir(projectPath)
	path := filepath.Join(dir, ".gitignore")

	body, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		if err := os.WriteFile(path, []byte(strings.TrimPrefix(gitignoreEntry, "\n")), 0o644); err != nil {
			return ignoreUnverified, fmt.Errorf("creating %s: %w", path, err)
		}
	case err != nil:
		return ignoreUnverified, fmt.Errorf("reading %s: %w", path, err)
	case !mentionsMxcliDir(string(body)):
		content := string(body)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		if err := os.WriteFile(path, []byte(content+gitignoreEntry), 0o644); err != nil {
			return ignoreUnverified, fmt.Errorf("appending to %s: %w", path, err)
		}
	}
	return checkIgnored(dir, filepath.Join(dir, ".mxcli", "x")), nil
}

// mentionsMxcliDir reports whether a .gitignore already covers `.mxcli/`. It is
// deliberately a cheap check for "did we already add our line" — whether the
// path is REALLY ignored is git's answer, not this one's.
func mentionsMxcliDir(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		switch strings.TrimSpace(line) {
		case ".mxcli/", ".mxcli", "/.mxcli/", "/.mxcli":
			return true
		}
	}
	return false
}

// checkIgnored asks git whether a path is ignored. Exit 0 means ignored, 1 means
// not, anything else (no git, not a repository) means we cannot tell.
func checkIgnored(dir, path string) ignoreStatus {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	cmd.Dir = dir
	err := cmd.Run()
	if err == nil {
		return ignoreConfirmed
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok && exitErr.ExitCode() == 1 {
		return ignoreNotIgnored
	}
	return ignoreUnverified
}

func asExitError(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}
