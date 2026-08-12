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
