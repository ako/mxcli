// SPDX-License-Identifier: Apache-2.0

// mxcli-chat FINDINGS §33: `alter settings constant … in configuration 'Default'`
// executed, reported success, round-tripped through `describe settings` — and
// never reached the running app, because mxbuild writes each constant's
// *default* into deployment/model/config.json and that map is what the runtime
// is handed. The app ran for hours with an empty encryption key.
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

func settingsWith(cfgs ...*model.ServerConfiguration) *model.ProjectSettings {
	return &model.ProjectSettings{
		Configuration: &model.ConfigurationSettings{Configurations: cfgs},
	}
}

func cfg(name string, values ...*model.ConstantValue) *model.ServerConfiguration {
	return &model.ServerConfiguration{Name: name, ConstantValues: values}
}

func shared(id, value string) *model.ConstantValue {
	return &model.ConstantValue{ConstantId: id, Value: value}
}

func private(id string) *model.ConstantValue {
	return &model.ConstantValue{ConstantId: id, IsPrivate: true}
}

func TestResolveConstantOverrides_ReadsTheConfigurationsValues(t *testing.T) {
	ps := settingsWith(cfg("Default", shared("Encryption.EncryptionKey", "95d6")))
	got := resolveConstantOverrides(ps, "")
	if got.Configuration != "Default" {
		t.Errorf("configuration = %q, want Default", got.Configuration)
	}
	if got.Values["Encryption.EncryptionKey"] != "95d6" {
		t.Errorf("values = %v, want the override — this is the whole bug", got.Values)
	}
}

// The only configuration is the one this run means, whatever it is called.
func TestResolveConstantOverrides_UsesTheOnlyConfiguration(t *testing.T) {
	ps := settingsWith(cfg("Whatever", shared("A.B", "v")))
	if got := resolveConstantOverrides(ps, ""); got.Configuration != "Whatever" || got.Values["A.B"] != "v" {
		t.Errorf("got %+v, want the sole configuration applied", got)
	}
}

// With several configurations and no "Default", picking one silently could apply
// production's database URL or API key to a local run. Refuse and say so.
func TestResolveConstantOverrides_DoesNotGuessBetweenConfigurations(t *testing.T) {
	ps := settingsWith(cfg("Acceptance", shared("A.B", "acc")), cfg("Production", shared("A.B", "prod")))
	got := resolveConstantOverrides(ps, "")
	if len(got.Values) != 0 {
		t.Errorf("applied %v without being told which configuration to run", got.Values)
	}
	if !strings.Contains(got.Note, "--configuration") {
		t.Errorf("note does not say how to resolve it: %q", got.Note)
	}
	// ...and naming one resolves it.
	if got := resolveConstantOverrides(ps, "Production"); got.Values["A.B"] != "prod" {
		t.Errorf("--configuration Production gave %+v", got)
	}
}

func TestResolveConstantOverrides_NamesAnUnknownConfiguration(t *testing.T) {
	ps := settingsWith(cfg("Default"), cfg("Production"))
	got := resolveConstantOverrides(ps, "Staging")
	if len(got.Values) != 0 {
		t.Errorf("applied values for a configuration that does not exist: %v", got.Values)
	}
	for _, want := range []string{"Staging", "Default", "Production"} {
		if !strings.Contains(got.Note, want) {
			t.Errorf("note %q should name %q", got.Note, want)
		}
	}
}

// A private override's value is not in the model — the stored node is a marker.
// Applying it would blank the constant, which is worse than leaving the default.
func TestResolveConstantOverrides_SkipsPrivateOverridesAndNamesThem(t *testing.T) {
	ps := settingsWith(cfg("Default", shared("A.Shared", "v"), private("A.Private")))
	got := resolveConstantOverrides(ps, "")
	if _, applied := got.Values["A.Private"]; applied {
		t.Error("a private override was applied; its value is not in the model, so this blanks the constant")
	}
	if len(got.Private) != 1 || got.Private[0] != "A.Private" {
		t.Errorf("private overrides not reported: %v", got.Private)
	}
	if got.Values["A.Shared"] != "v" {
		t.Error("the shared override in the same configuration was dropped too")
	}
}

func TestResolveConstantOverrides_ProjectWithNoConfigurations(t *testing.T) {
	if got := resolveConstantOverrides(&model.ProjectSettings{}, ""); len(got.Values) != 0 || got.Note == "" {
		t.Errorf("got %+v, want no values and a reason", got)
	}
}

// Silence used to mean "your override is in effect" when it was not. Every
// outcome has to print something.
func TestReportConstantOverrides_SaysSomethingInEveryCase(t *testing.T) {
	cases := []constantOverrides{
		{Configuration: "Default", Values: map[string]string{"A.B": "v"}},
		{Configuration: "Default", Values: map[string]string{}},
		{Values: map[string]string{}, Note: "the project has no configurations"},
		{Configuration: "Default", Values: map[string]string{}, Private: []string{"A.P"}},
	}
	for i, c := range cases {
		var buf bytes.Buffer
		reportConstantOverrides(&buf, c)
		if strings.TrimSpace(buf.String()) == "" {
			t.Errorf("case %d printed nothing: %+v", i, c)
		}
	}
}
