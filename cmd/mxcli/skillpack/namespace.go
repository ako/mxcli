// SPDX-License-Identifier: Apache-2.0

package skillpack

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// A Mendix pluggable widget id looks like `acme.widget.web.vegachart.VegaChart`.
// The first segment identifies whoever built it, and it is the only part a
// consuming project has any business changing — `widget.web` is convention and
// the tail comes from the widget itself.
//
// Getting this wrong is not a build error. Two apps whose widgets share an id
// are two apps claiming the same widget, and what you see is a widget that
// resolves to somebody else's build.

var namespaceInvalid = regexp.MustCompile(`[^a-z0-9]+`)

// NormalizeNamespace turns a human-supplied name into a legal id segment:
// lowercase, alphanumeric, no leading digit.
//
// A leading digit is rejected rather than silently prefixed, because a namespace
// the user did not choose is exactly as wrong as one that does not fit — and
// they would find out at build time with the id already baked into pages.
func NormalizeNamespace(in string) (string, error) {
	s := namespaceInvalid.ReplaceAllString(strings.ToLower(strings.TrimSpace(in)), "")
	if s == "" {
		return "", fmt.Errorf("namespace %q has no usable characters; "+
			"pass --namespace with a name like 'acme'", in)
	}
	if s[0] >= '0' && s[0] <= '9' {
		return "", fmt.Errorf("namespace %q starts with a digit, which a widget id cannot; "+
			"pass --namespace with a name starting with a letter", in)
	}
	return s, nil
}

// NamespaceFromProject derives a namespace from the project's .mpr filename.
//
// This is a default, not a guess to be trusted silently: callers print what was
// derived so a project called `App1112` does not quietly become the vendor
// prefix of a widget somebody ships elsewhere.
func NamespaceFromProject(mprPath string) (string, error) {
	base := filepath.Base(mprPath)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" || base == "." {
		return "", fmt.Errorf("cannot derive a namespace from %q; pass --namespace", mprPath)
	}
	return NormalizeNamespace(base)
}

// Vars builds the substitution set for a pack installed into a project.
//
// projectPath is written into the widget's package.json as the build's output
// target, relative to where the widget source lands.
func Vars(namespace, projectPath string) map[string]string {
	return map[string]string{
		"NAMESPACE":      namespace,
		"NAMESPACE_PATH": strings.ReplaceAll(namespace, ".", "/"),
		"PROJECT_PATH":   projectPath,
	}
}

// WidgetID is the full pluggable-widget id a pack's widget will carry once
// installed under the given namespace. Printed on install so the id a page must
// reference is never something the user has to reconstruct by hand.
func WidgetID(namespace, tail string) string {
	return namespace + ".widget.web." + tail
}
