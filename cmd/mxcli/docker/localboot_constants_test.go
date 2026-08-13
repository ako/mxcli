// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

// mxcli-chat FINDINGS §33: the configuration's constant values have to reach the
// runtime, and §33's caveat: they must be MERGED over mxbuild's resolved
// defaults, never replace them. `--runtime-setting MicroflowConstants={…}` has
// the replacing shape, which drops every constant the configuration is silent
// about — and the app 530s on the first microflow that reads one.
func TestMergeConstantOverrides(t *testing.T) {
	defaults := map[string]string{
		"Encryption.EncryptionKey": "",
		"App.Timeout":              "30",
		"App.BaseUrl":              "http://localhost",
	}
	merged := mergeConstantOverrides(defaults, map[string]string{
		"Encryption.EncryptionKey": "95d6",
	})

	if merged["Encryption.EncryptionKey"] != "95d6" {
		t.Errorf("the override did not win: %q", merged["Encryption.EncryptionKey"])
	}
	if merged["App.Timeout"] != "30" || merged["App.BaseUrl"] != "http://localhost" {
		t.Errorf("defaults the configuration is silent about were dropped: %v — this is the replace bug", merged)
	}
	if len(merged) != 3 {
		t.Errorf("merged has %d entries, want 3: %v", len(merged), merged)
	}
	// The caller's map must not be mutated: the same defaults are read once per
	// boot and a restart re-uses them.
	if defaults["Encryption.EncryptionKey"] != "" {
		t.Error("mergeConstantOverrides mutated the defaults map it was given")
	}
}

// No overrides is the pre-existing behaviour and must stay allocation-free of
// surprises: the defaults go through untouched.
func TestMergeConstantOverrides_NoOverrides(t *testing.T) {
	defaults := map[string]string{"A.B": "v"}
	if got := mergeConstantOverrides(defaults, nil); got["A.B"] != "v" || len(got) != 1 {
		t.Errorf("got %v, want the defaults unchanged", got)
	}
}

// StartLocalApp is the headless boot behind `mxcli test --local`. Its options
// have to reach LocalRuntimeOptions or the app runs with each constant's
// default while `--attach` runs with the configuration's — the divergence in
// docs/11-proposals/PROPOSAL_constant_values.md slice 1.
func TestLocalAppOptions_ForwardsToTheRuntime(t *testing.T) {
	opts := LocalAppOptions{
		DeployDir:         "/tmp/app/deployment",
		AppPort:           8081,
		AdminPort:         8091,
		AdminPass:         "pass",
		DB:                DBConfig{Name: "app_test"},
		RuntimeLogPath:    "/tmp/app/.mxcli/test-runtime.log",
		Env:               []string{"MXCLI_TEST_TOKEN=tok"},
		ConstantOverrides: map[string]string{"MyModule.ApiKey": "v"},
	}

	rt := opts.runtimeOptions("/install/path")

	if rt.ConstantOverrides["MyModule.ApiKey"] != "v" {
		t.Fatalf("ConstantOverrides = %v, want the value to reach the runtime", rt.ConstantOverrides)
	}
	// The fields that were already forwarded stay forwarded: this mapping is the
	// single place a runtime option can be dropped without anything failing.
	if rt.DeployDir != opts.DeployDir || rt.AppPort != opts.AppPort || rt.AdminPort != opts.AdminPort ||
		rt.AdminPass != opts.AdminPass || rt.DB.Name != opts.DB.Name ||
		rt.RuntimeLogPath != opts.RuntimeLogPath || len(rt.Env) != 1 {
		t.Errorf("a field was dropped in the mapping: %+v", rt)
	}
	if rt.InstallPath != "/install/path" {
		t.Errorf("InstallPath = %q", rt.InstallPath)
	}
}

// ApplyConstants is the live path: change a running app's constants without a
// restart. Both calls are load-bearing — update_configuration is STAGED (the app
// keeps its old values and still answers result:0) and only reload_model applies
// them, measured on 11.12.1. A version that sent only the first call would report
// success and change nothing.
func TestApplyConstants_SendsTheConfigurationAndThenReloads(t *testing.T) {
	var actions []string
	var sent map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		actions = append(actions, body.Action)
		if body.Action == "update_configuration" {
			sent = body.Params
		}
		_, _ = w.Write([]byte(`{"result":0,"feedback":{}}`))
	}))
	defer srv.Close()

	boot := map[string]any{
		"BasePath":           "/deploy",
		"DatabaseName":       "app",
		"MicroflowConstants": map[string]string{"A.Old": "stale"},
	}
	if err := ApplyConstants(m2eeFor(srv), boot, map[string]string{"A.New": "fresh"}); err != nil {
		t.Fatalf("ApplyConstants: %v", err)
	}

	if len(actions) != 2 || actions[0] != "update_configuration" || actions[1] != "reload_model" {
		t.Fatalf("actions = %v, want update_configuration then reload_model — without the "+
			"reload the call is staged and the app keeps the old value", actions)
	}
	// The rest of the boot payload is re-sent, because the admin API has no
	// read-back: anything not sent is simply gone from the configuration.
	if sent["BasePath"] != "/deploy" || sent["DatabaseName"] != "app" {
		t.Errorf("the boot payload was not carried through: %v", sent)
	}
	if got, ok := sent["MicroflowConstants"].(map[string]any); !ok || got["A.New"] != "fresh" {
		t.Errorf("MicroflowConstants = %v, want the new map", sent["MicroflowConstants"])
	}
	if got := sent["MicroflowConstants"].(map[string]any); got["A.Old"] != nil {
		t.Errorf("the old constants map survived: %v", got)
	}
}

// The caller's map must not be mutated, and neither must the stored boot config —
// a dev loop re-reads its own BootConfig on every apply.
func TestApplyConstants_DoesNotMutateTheBootConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"result":0,"feedback":{}}`))
	}))
	defer srv.Close()

	boot := map[string]any{"BasePath": "/deploy", "MicroflowConstants": map[string]string{"A.Old": "stale"}}
	if err := ApplyConstants(m2eeFor(srv), boot, map[string]string{"A.New": "fresh"}); err != nil {
		t.Fatalf("ApplyConstants: %v", err)
	}
	got, _ := boot["MicroflowConstants"].(map[string]string)
	if got["A.Old"] != "stale" {
		t.Errorf("the caller's boot config was mutated: %v", boot)
	}
}

func TestApplyConstants_RefusesWithoutABootConfig(t *testing.T) {
	if err := ApplyConstants(M2EEOptions{}, nil, map[string]string{"A.B": "v"}); err == nil {
		t.Error("applying with no boot payload was accepted; it would blank the configuration")
	}
}

func m2eeFor(srv *httptest.Server) M2EEOptions {
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	return M2EEOptions{Host: u.Hostname(), Port: port, Token: "pass", Direct: true}
}
