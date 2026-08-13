// SPDX-License-Identifier: Apache-2.0

package docker

import "testing"

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
