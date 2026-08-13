// SPDX-License-Identifier: Apache-2.0

// Layer 1 of docs/11-proposals/PROPOSAL_constant_values.md: `--constant
// Module.Name=value`, for a value that should reach one run and nothing else.
// Every route mxcli offered before this wrote to the model, and therefore to
// git, so setting an API key for a single run meant committing it (or
// remembering to revert).
package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

var knownTwo = map[string]bool{"A.Key": true, "A.Url": true}

func TestParseConstantFlags(t *testing.T) {
	got, err := parseConstantFlags([]string{"A.Key=sk-123", "A.Url=https://x/y=z"})
	if err != nil {
		t.Fatalf("parseConstantFlags: %v", err)
	}
	if got["A.Key"] != "sk-123" {
		t.Errorf("A.Key = %q", got["A.Key"])
	}
	// Only the FIRST "=" separates; a value may contain more.
	if got["A.Url"] != "https://x/y=z" {
		t.Errorf("A.Url = %q, want the whole value after the first '='", got["A.Url"])
	}
}

// An empty value is legitimate ("run with this constant blank"); a missing "="
// is not — it almost certainly means the value was meant to be the next
// argument, and quietly setting the constant to "" is the exact class of silent
// wrong value this feature exists to prevent.
func TestParseConstantFlags_RejectsMalformed(t *testing.T) {
	if got, err := parseConstantFlags([]string{"A.Key="}); err != nil || got["A.Key"] != "" {
		t.Errorf("A.Key= should set an empty value, got %v / %v", got, err)
	}
	for _, bad := range []string{"A.Key", "NoDots=v", "A.B.C=v", ".Key=v", "A.=v"} {
		if _, err := parseConstantFlags([]string{bad}); err == nil {
			t.Errorf("parseConstantFlags(%q) accepted it", bad)
		}
	}
	if _, err := parseConstantFlags([]string{"A.Key=1", "A.Key=2"}); err == nil {
		t.Error("the same constant given twice was accepted; which one wins is a coin flip")
	}
}

func TestResolveConstantChain_FlagWinsOverTheConfiguration(t *testing.T) {
	ps := settingsWith(cfg("Default", shared("A.Key", "from-configuration"), shared("A.Url", "u")))

	got, err := resolveConstantChain(ps, "", map[string]string{"A.Key": "from-the-flag"}, nil, knownTwo)
	if err != nil {
		t.Fatalf("resolveConstantChain: %v", err)
	}
	if got.Values["A.Key"] != "from-the-flag" {
		t.Errorf("A.Key = %q, want the flag to win", got.Values["A.Key"])
	}
	if got.From["A.Key"] != layerFlag {
		t.Errorf("A.Key came from %q, want %q", got.From["A.Key"], layerFlag)
	}
	// A constant the flag does not name keeps the configuration's value.
	if got.Values["A.Url"] != "u" || got.From["A.Url"] != layerConfiguration {
		t.Errorf("A.Url = %q from %q, want the configuration's", got.Values["A.Url"], got.From["A.Url"])
	}
}

// The runtime ignores a MicroflowConstants entry naming no constant, so a typo
// would be accepted, reported as applied, and do nothing — the mxcli-chat §33
// shape, reintroduced by the flag meant to fix it.
func TestResolveConstantChain_RefusesAnUnknownConstant(t *testing.T) {
	ps := settingsWith(cfg("Default"))

	_, err := resolveConstantChain(ps, "", map[string]string{"A.Keyy": "v"}, nil, knownTwo)
	if err == nil {
		t.Fatal("a constant that does not exist was accepted")
	}
	if !strings.Contains(err.Error(), "A.Keyy") {
		t.Errorf("the error does not name the constant: %v", err)
	}
}

// A flag can be used precisely BECAUSE a private override has no value in the
// model. Reporting the constant as "private, default used" afterwards would
// contradict what the app is about to do.
func TestResolveConstantChain_FlagOverridesAPrivateValueAndStopsReportingIt(t *testing.T) {
	ps := settingsWith(cfg("Default", private("A.Key"), private("A.Url")))

	got, err := resolveConstantChain(ps, "", map[string]string{"A.Key": "v"}, nil, knownTwo)
	if err != nil {
		t.Fatalf("resolveConstantChain: %v", err)
	}
	if got.Values["A.Key"] != "v" {
		t.Errorf("the flag did not override the private value: %v", got.Values)
	}
	if len(got.Private) != 1 || got.Private[0] != "A.Url" {
		t.Errorf("Private = %v, want only the one the flag did not cover", got.Private)
	}
}

// With no flags the chain has to behave exactly as the configuration-only
// resolution did — including refusing to guess between configurations.
func TestResolveConstantChain_WithoutFlagsMatchesTheConfigurationResolution(t *testing.T) {
	ps := settingsWith(cfg("Acceptance", shared("A.Key", "acc")), cfg("Production", shared("A.Key", "prod")))

	got, err := resolveConstantChain(ps, "", nil, nil, knownTwo)
	if err != nil {
		t.Fatalf("resolveConstantChain: %v", err)
	}
	if len(got.Values) != 0 || !strings.Contains(got.Note, "--configuration") {
		t.Errorf("got %+v, want no values and the hint", got)
	}
}

// An unknown name is refused even when the project has no configurations at
// all: the check is against what the project DECLARES, not against what some
// configuration happens to override.
func TestResolveConstantChain_ValidatesAgainstDeclaredConstantsNotOverrides(t *testing.T) {
	if _, err := resolveConstantChain(&model.ProjectSettings{}, "", map[string]string{"A.Nope": "v"}, nil, knownTwo); err == nil {
		t.Error("an unknown constant was accepted because no configuration was present")
	}
	if _, err := resolveConstantChain(&model.ProjectSettings{}, "", map[string]string{"A.Key": "v"}, nil, knownTwo); err != nil {
		t.Errorf("a declared constant was refused: %v", err)
	}
}

// Silence used to mean "your override is in effect" when it was not. Every
// outcome has to print something, and now also say which layer won.
func TestReportConstantChain_SaysSomethingInEveryCase(t *testing.T) {
	cases := []constantChain{
		{Configuration: "Default", Values: map[string]string{"A.B": "v"}, From: map[string]constantLayer{"A.B": layerConfiguration}},
		{Values: map[string]string{"A.B": "v"}, From: map[string]constantLayer{"A.B": layerFlag}},
		{Configuration: "Default", Values: map[string]string{}},
		{Values: map[string]string{}, Note: "the project has no configurations"},
		{Configuration: "Default", Values: map[string]string{}, Private: []string{"A.P"}},
	}
	for i, c := range cases {
		var buf bytes.Buffer
		reportConstantChain(&buf, c)
		if strings.TrimSpace(buf.String()) == "" {
			t.Errorf("case %d printed nothing: %+v", i, c)
		}
	}
}

func TestReportConstantChain_NamesTheLayer(t *testing.T) {
	var buf bytes.Buffer
	reportConstantChain(&buf, constantChain{
		Configuration: "Default",
		Values:        map[string]string{"A.Key": "sk-SECRETVALUE", "A.Url": "https://SECRETHOST"},
		From:          map[string]constantLayer{"A.Key": layerFlag, "A.Url": layerConfiguration},
	})
	out := buf.String()
	if !strings.Contains(out, "A.Key") || !strings.Contains(out, "--constant") {
		t.Errorf("the flag-set constant is not attributed to --constant:\n%s", out)
	}
	if !strings.Contains(out, `configuration "Default"`) {
		t.Errorf("the configuration-set constant is not attributed:\n%s", out)
	}
	// A constant can hold an API key, so the report names constants and layers
	// and never prints a value.
	for _, secret := range []string{"sk-SECRETVALUE", "SECRETHOST"} {
		if strings.Contains(out, secret) {
			t.Errorf("the report printed the value %q; only names and layers belong here:\n%s", secret, out)
		}
	}
}

// Layer 2 — the machine store. It sits between --constant and the
// configuration: higher than the shared value the team committed, lower than
// what this invocation asked for.
func TestResolveConstantChain_MachineStoreBeatsConfigurationAndLosesToFlag(t *testing.T) {
	ps := settingsWith(cfg("Default", shared("A.Key", "shared"), shared("A.Url", "shared-url")))
	machine := map[string]string{"A.Key": "machine", "A.Url": "machine-url"}

	got, err := resolveConstantChain(ps, "", map[string]string{"A.Key": "flag"}, machine, knownTwo)
	if err != nil {
		t.Fatalf("resolveConstantChain: %v", err)
	}
	if got.Values["A.Key"] != "flag" || got.From["A.Key"] != layerFlag {
		t.Errorf("A.Key = %q from %q, want the flag to win", got.Values["A.Key"], got.From["A.Key"])
	}
	if got.Values["A.Url"] != "machine-url" || got.From["A.Url"] != layerMachine {
		t.Errorf("A.Url = %q from %q, want the machine store to beat the configuration",
			got.Values["A.Url"], got.From["A.Url"])
	}
}

// A stale machine entry is the user's own file, not a typo they can fix by
// rerunning. Refusing would make every run fail until the file was hand-edited,
// over a value the project stopped declaring — so it is skipped and named,
// unlike a --constant flag, which IS refused.
func TestResolveConstantChain_SkipsAndNamesAStaleMachineEntry(t *testing.T) {
	machine := map[string]string{"A.Key": "v", "A.Removed": "old"}

	got, err := resolveConstantChain(settingsWith(cfg("Default")), "", nil, machine, knownTwo)
	if err != nil {
		t.Fatalf("a stale machine entry should not fail the run: %v", err)
	}
	if got.Values["A.Key"] != "v" {
		t.Errorf("the still-valid entry was dropped: %v", got.Values)
	}
	if _, applied := got.Values["A.Removed"]; applied {
		t.Error("a value for a constant the project no longer declares was applied")
	}
	if len(got.Stale) != 1 || got.Stale[0] != "A.Removed" {
		t.Errorf("Stale = %v, want it named so the user can clean it up", got.Stale)
	}
}

// The private-override note must not survive ANY layer above it, not just a
// flag: a machine value means the default is not what runs either.
func TestResolveConstantChain_MachineValueAlsoClearsThePrivateNote(t *testing.T) {
	ps := settingsWith(cfg("Default", private("A.Key"), private("A.Url")))

	got, err := resolveConstantChain(ps, "", nil, map[string]string{"A.Key": "v"}, knownTwo)
	if err != nil {
		t.Fatalf("resolveConstantChain: %v", err)
	}
	if len(got.Private) != 1 || got.Private[0] != "A.Url" {
		t.Errorf("Private = %v, want only the one no layer covered", got.Private)
	}
}

func TestReportConstantChain_NamesTheMachineLayerAndStaleEntries(t *testing.T) {
	var buf bytes.Buffer
	reportConstantChain(&buf, constantChain{
		Values: map[string]string{"A.Key": "SECRETVALUE"},
		From:   map[string]constantLayer{"A.Key": layerMachine},
		Stale:  []string{"A.Removed"},
	})
	out := buf.String()
	if !strings.Contains(out, string(layerMachine)) {
		t.Errorf("the machine layer is not named:\n%s", out)
	}
	if !strings.Contains(out, "A.Removed") {
		t.Errorf("the stale entry is not reported:\n%s", out)
	}
	if strings.Contains(out, "SECRETVALUE") {
		t.Errorf("the report printed a machine-local value:\n%s", out)
	}
}
