// SPDX-License-Identifier: Apache-2.0

// A `--local` test run booted with each constant's DEFAULT, because the headless
// boot never carried the configuration's values. The same suite under `--attach`
// runs against an app `run --local` booted, which does apply them — so the two
// modes silently disagreed on every constant a test touched. Nothing errored: a
// constant resolving to the wrong value is not an error, it is just a different
// assertion. docs/11-proposals/PROPOSAL_constant_values.md slice 1.
package testrunner

import (
	"io"
	"testing"
)

func TestLocalAppOptions_CarriesTheConstantOverrides(t *testing.T) {
	opts := RunOptions{
		ProjectPath:       "/tmp/app/App.mpr",
		ConstantOverrides: map[string]string{"MyModule.ApiKey": "from-the-configuration"},
	}

	got := localAppOptions(opts, "/tmp/app/.mxcli/test-runtime.log", nil, io.Discard)

	if got.ConstantOverrides["MyModule.ApiKey"] != "from-the-configuration" {
		t.Fatalf("ConstantOverrides = %v, want the configuration's value — without it a "+
			"--local run asserts against the constant's default while --attach asserts "+
			"against the configuration's", got.ConstantOverrides)
	}
}

// Both --local runners boot through this, so both have to carry the values. The
// endpoint runner adds the token to the environment and the legacy runner does
// not; that is the only difference between them.
func TestLocalAppOptions_SameForBothRunners(t *testing.T) {
	opts := RunOptions{
		ProjectPath:       "/tmp/app/App.mpr",
		ConstantOverrides: map[string]string{"A.B": "v"},
	}

	endpoint := localAppOptions(opts, "log", []string{endpointTokenEnv + "=tok"}, io.Discard)
	legacy := localAppOptions(opts, "log", nil, io.Discard)

	if endpoint.ConstantOverrides["A.B"] != "v" || legacy.ConstantOverrides["A.B"] != "v" {
		t.Errorf("endpoint=%v legacy=%v, want both to carry the value",
			endpoint.ConstantOverrides, legacy.ConstantOverrides)
	}
	if endpoint.AppPort != legacy.AppPort || endpoint.DB.Name != legacy.DB.Name {
		t.Errorf("the runners disagree on ports/database: %+v vs %+v", endpoint, legacy)
	}
	if len(endpoint.Env) != 1 || len(legacy.Env) != 0 {
		t.Errorf("env differs from expectation: endpoint=%v legacy=%v", endpoint.Env, legacy.Env)
	}
}

// The scratch database is what lets a `run --local` dev loop keep serving the
// same project while tests run. Sharing the option builder must not lose it.
func TestLocalAppOptions_UsesAScratchDatabase(t *testing.T) {
	got := localAppOptions(RunOptions{ProjectPath: "/tmp/app/App.mpr"}, "log", nil, io.Discard)
	if want := "app" + localTestDBSuffix; got.DB.Name != want {
		t.Errorf("DB.Name = %q, want %q", got.DB.Name, want)
	}
	if !got.EnsureDB {
		t.Error("EnsureDB is false; the scratch database would have to exist already")
	}
}
