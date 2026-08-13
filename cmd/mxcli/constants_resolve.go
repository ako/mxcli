// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/mendixlabs/mxcli/model"
)

// constantLayer names where a resolved constant value came from. It exists so a
// run can SAY which layer won: with more than one place to set a value, "the
// value is X" stops being useful on its own, and the whole family of bugs this
// resolves (mxcli-chat FINDINGS §33) is values arriving from somewhere other
// than where the author looked.
type constantLayer string

const (
	layerFlag          constantLayer = "--constant"
	layerConfiguration constantLayer = "configuration"
)

// constantChain is the resolved set of values to hand a booting app, plus the
// layer each one came from.
//
// A constant absent from Values keeps its default, which the deployment already
// carries — mxbuild writes every constant's default into
// deployment/model/config.json, and mxcli layers over that rather than
// replacing it. See docs/11-proposals/PROPOSAL_constant_values.md.
type constantChain struct {
	Configuration string                   // the configuration whose values these are
	Values        map[string]string        // constant qualified name -> value
	From          map[string]constantLayer // and which layer set it
	Private       []string                 // overrides whose value is not in the model
	Note          string                   // why no configuration contributed
}

// parseConstantFlags turns repeated `--constant Module.Name=value` into a map.
//
// A missing "=" is an error rather than a value of "": `--constant M.C` almost
// certainly means the author expected the next argument to be the value, and
// silently setting the constant to empty is exactly the kind of quiet wrong
// value this whole feature exists to stop.
func parseConstantFlags(flags []string) (map[string]string, error) {
	out := make(map[string]string, len(flags))
	for _, f := range flags {
		name, value, found := strings.Cut(f, "=")
		name = strings.TrimSpace(name)
		if !found {
			return nil, fmt.Errorf("--constant %q: expected Module.Name=value", f)
		}
		if strings.Count(name, ".") != 1 || strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".") {
			return nil, fmt.Errorf("--constant %q: %q is not a qualified constant name (Module.Name)", f, name)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("--constant %s given more than once", name)
		}
		out[name] = value
	}
	return out, nil
}

// resolveConstantChain layers `--constant` values over a configuration's shared
// values. known is the set of constants the project declares; a flag naming
// anything else is refused.
//
// Refusing an unknown name is the point of passing `known` at all. The runtime
// ignores a MicroflowConstants entry that matches no constant, so a typo would
// otherwise be accepted, reported as applied, and do nothing — the §33 failure
// shape, reintroduced by the very flag meant to fix it.
func resolveConstantChain(ps *model.ProjectSettings, want string, flags map[string]string, known map[string]bool) (constantChain, error) {
	base := resolveConstantOverrides(ps, want)
	chain := constantChain{
		Configuration: base.Configuration,
		Values:        map[string]string{},
		From:          map[string]constantLayer{},
		Private:       base.Private,
		Note:          base.Note,
	}
	for name, value := range base.Values {
		chain.Values[name] = value
		chain.From[name] = layerConfiguration
	}

	var unknown []string
	for name := range flags {
		if known != nil && !known[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return constantChain{}, fmt.Errorf("no constant named %s in this project\n"+
			"  hint: 'mxcli -p <app.mpr> -c \"show constant values\"' lists them; the runtime ignores "+
			"a value for a constant that does not exist, so this would have been applied to nothing",
			strings.Join(unknown, ", "))
	}

	for name, value := range flags {
		chain.Values[name] = value
		chain.From[name] = layerFlag
	}
	// A flag overrides a private configuration value too, and then the default is
	// NOT what runs — so the constant must stop being reported as private-and-
	// defaulted, or the report contradicts what the app will do.
	if len(flags) > 0 && len(chain.Private) > 0 {
		kept := chain.Private[:0]
		for _, name := range chain.Private {
			if _, overridden := flags[name]; !overridden {
				kept = append(kept, name)
			}
		}
		chain.Private = kept
	}
	return chain, nil
}

// constantChainFor reads a project and resolves the values a run should use.
//
// A project that cannot be read yields no values and a note — a run must not be
// blocked by this, since every run before configurations were applied used the
// defaults anyway. The exception is when `--constant` was passed: the author
// named something specific, and applying it unvalidated (or dropping it
// silently) are both worse than saying the project could not be read.
func constantChainFor(projectPath, configuration string, flags map[string]string) (constantChain, error) {
	b := newBackendFactory()()
	if err := b.Connect(projectPath); err != nil {
		return unreadableProject(err, flags)
	}
	defer func() { _ = b.Disconnect() }()

	ps, err := b.GetProjectSettings()
	if err != nil {
		return unreadableProject(err, flags)
	}
	known, err := knownConstantNames(b)
	if err != nil {
		return unreadableProject(err, flags)
	}
	return resolveConstantChain(ps, configuration, flags, known)
}

func unreadableProject(err error, flags map[string]string) (constantChain, error) {
	if len(flags) > 0 {
		return constantChain{}, fmt.Errorf("could not read the project's constants to apply --constant: %w", err)
	}
	return constantChain{
		Values: map[string]string{},
		From:   map[string]constantLayer{},
		Note:   "could not read the project's settings: " + err.Error(),
	}, nil
}

// knownConstantNames returns the qualified names of every constant the project
// declares. Folders do not appear in a qualified name, so the hierarchy is used
// only to find each constant's module.
func knownConstantNames(b backend.FullBackend) (map[string]bool, error) {
	constants, err := b.ListConstants()
	if err != nil {
		return nil, err
	}
	h, err := executor.NewContainerHierarchyFromBackend(b)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(constants))
	for _, c := range constants {
		if module := h.GetModuleName(h.FindModuleID(c.ContainerID)); module != "" {
			out[module+"."+c.Name] = true
		}
	}
	return out, nil
}

// reportConstantChain says what will be applied before the app boots, and from
// where.
//
// It prints even when nothing is applied. The failure this fixes was invisible
// precisely because the run said nothing about constants either way, so silence
// has to stop meaning "your override is in effect".
func reportConstantChain(w io.Writer, c constantChain) {
	switch {
	case len(c.Values) > 0:
		names := make([]string, 0, len(c.Values))
		width := 0
		for k := range c.Values {
			names = append(names, k)
			if len(k) > width {
				width = len(k)
			}
		}
		sort.Strings(names)
		fmt.Fprintf(w, "Applying %d constant value(s):\n", len(names))
		for _, n := range names {
			from := string(c.From[n])
			if c.From[n] == layerConfiguration {
				from = fmt.Sprintf("configuration %q", c.Configuration)
			}
			fmt.Fprintf(w, "  %-*s  %s\n", width, n, from)
		}
	case c.Configuration != "":
		fmt.Fprintf(w, "Configuration %q sets no constant values; using each constant's default.\n", c.Configuration)
	case c.Note != "":
		fmt.Fprintf(w, "Using each constant's default value (%s).\n", c.Note)
	}
	if len(c.Private) > 0 {
		fmt.Fprintf(w, "  %d override(s) are private, so their value is not in the model and the default is used:\n    %s\n",
			len(c.Private), strings.Join(c.Private, "\n    "))
	}
}
