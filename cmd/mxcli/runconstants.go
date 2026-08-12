// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// constantOverridesFor reads the running configuration's constant values from a
// project. A project that cannot be read yields no overrides and a note — a run
// must not be blocked by this, since every run before it applied none.
func constantOverridesFor(projectPath, configuration string) constantOverrides {
	b := newBackendFactory()()
	if err := b.Connect(projectPath); err != nil {
		return constantOverrides{Values: map[string]string{}, Note: "could not read the project's settings: " + err.Error()}
	}
	defer func() { _ = b.Disconnect() }()

	ps, err := b.GetProjectSettings()
	if err != nil {
		return constantOverrides{Values: map[string]string{}, Note: "could not read the project's settings: " + err.Error()}
	}
	return resolveConstantOverrides(ps, configuration)
}

// Constant values set per *configuration* never reached a locally-run app.
//
// mxbuild writes <deployDir>/model/config.json with each constant's **default**
// value, and that map is what the standalone runtime is handed as
// MicroflowConstants — the configuration's overrides are not in it. So
//
//	alter settings constant 'Encryption.EncryptionKey' value '…' in configuration 'Default';
//
// executed, reported success, survived a round-trip through `describe settings`,
// and then did nothing: the app ran with the constant's default. Measured in
// mxcli-chat FINDINGS §33, where an app ran for hours with an empty encryption
// key while the model said otherwise. Nothing failed — a wrong constant is not a
// build error — which is what makes a silent no-op the worst shape for this bug.
//
// Studio Pro runs a *configuration*; so does this now. resolveConstantOverrides
// picks one and returns its shared constant values, for the caller to merge over
// the defaults at boot.
//
// Two things it deliberately does not do:
//
//   - A **private** override carries no value in the model at all (the stored
//     node is a Settings$PrivateValue marker — the value lives on the developer's
//     workstation). Applying it would blank the constant, so it is skipped and
//     named. "" here means "not in the model", never "overridden with empty".
//   - It does not guess when the project has several configurations and none is
//     obviously the one to run. Applying the wrong environment's database URL or
//     API key silently is worse than applying none.
type constantOverrides struct {
	Configuration string            // the configuration whose values these are
	Values        map[string]string // constant qualified name -> value
	Private       []string          // overrides whose value is not in the model
	Note          string            // why nothing was applied, when Values is empty
}

// resolveConstantOverrides chooses a configuration and reads its shared constant
// values. want names one explicitly; empty means "pick the obvious one".
func resolveConstantOverrides(ps *model.ProjectSettings, want string) constantOverrides {
	out := constantOverrides{Values: map[string]string{}}
	if ps == nil || ps.Configuration == nil || len(ps.Configuration.Configurations) == 0 {
		out.Note = "the project has no configurations"
		return out
	}
	cfgs := ps.Configuration.Configurations

	var chosen *model.ServerConfiguration
	switch {
	case want != "":
		for _, c := range cfgs {
			if strings.EqualFold(c.Name, want) {
				chosen = c
				break
			}
		}
		if chosen == nil {
			out.Note = fmt.Sprintf("no configuration named %q (have: %s)", want, configurationNames(cfgs))
			return out
		}
	case len(cfgs) == 1:
		chosen = cfgs[0]
	default:
		for _, c := range cfgs {
			if strings.EqualFold(c.Name, "Default") {
				chosen = c
				break
			}
		}
		if chosen == nil {
			// Several configurations and no "Default": which one this run means is
			// the user's call, not a guess worth making silently.
			out.Note = fmt.Sprintf("the project has %d configurations and none is named \"Default\" (%s); "+
				"pass --configuration to choose one", len(cfgs), configurationNames(cfgs))
			return out
		}
	}

	out.Configuration = chosen.Name
	for _, cv := range chosen.ConstantValues {
		if cv == nil {
			continue
		}
		if cv.IsPrivate {
			out.Private = append(out.Private, cv.ConstantId)
			continue
		}
		out.Values[cv.ConstantId] = cv.Value
	}
	sort.Strings(out.Private)
	return out
}

func configurationNames(cfgs []*model.ServerConfiguration) string {
	names := make([]string, 0, len(cfgs))
	for _, c := range cfgs {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// reportConstantOverrides says what will be applied before the app boots.
//
// It prints even when nothing is applied. The failure this fixes was invisible
// precisely because the run said nothing about constants either way, so silence
// has to stop meaning "your override is in effect".
func reportConstantOverrides(w io.Writer, o constantOverrides) {
	switch {
	case len(o.Values) > 0:
		names := make([]string, 0, len(o.Values))
		for k := range o.Values {
			names = append(names, k)
		}
		sort.Strings(names)
		fmt.Fprintf(w, "Applying %d constant value(s) from configuration %q: %s\n",
			len(names), o.Configuration, strings.Join(names, ", "))
	case o.Configuration != "":
		fmt.Fprintf(w, "Configuration %q sets no constant values; using each constant's default.\n", o.Configuration)
	case o.Note != "":
		fmt.Fprintf(w, "Using each constant's default value (%s).\n", o.Note)
	}
	if len(o.Private) > 0 {
		fmt.Fprintf(w, "  %d override(s) are private, so their value is not in the model and the default is used:\n    %s\n",
			len(o.Private), strings.Join(o.Private, "\n    "))
	}
}
