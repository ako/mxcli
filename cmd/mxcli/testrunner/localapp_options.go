// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"io"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// localAppOptions builds the headless boot for a `--local` test run, shared by
// the endpoint runner and the legacy after-startup runner.
//
// The two runners differ only in what reaches the runtime's environment and in
// which log they read, so everything else — ports, the scratch database, and
// the constant values the app runs with — is decided in one place. It was the
// constants that made this worth sharing: they were absent from both runners,
// so a suite saw a different value under `--local` than the same suite saw
// under `--attach` (which runs against an app `run --local` booted, with the
// configuration's values applied). Nothing errored; the assertion just ran
// against the wrong constant. See docs/11-proposals/PROPOSAL_constant_values.md.
func localAppOptions(opts RunOptions, logPath string, env []string, w io.Writer) docker.LocalAppOptions {
	return docker.LocalAppOptions{
		ProjectPath: opts.ProjectPath,
		AppPort:     localTestAppPort,
		AdminPort:   localTestAdminPort,
		ServePort:   localTestServePort,
		// DeployDir is deliberately left at its default, <project dir>/deployment.
		// It is shared with a concurrent `mxcli run --local` — unlike the ports and
		// the database — because mxbuild writes the deployment there and has no
		// option to move it, so a scratch tree is one nothing populates
		// (mxcli-ledger §150). The damage that sharing used to do, a headless boot
		// deleting the browser bundle the running app serves, is undone by
		// StartLocalApp carrying the bundle across the boot (FINDINGS §62).
		DB: docker.DBConfig{
			// A scratch database, so a `run --local` dev loop can keep serving the
			// same project while the tests run.
			Name: docker.DeriveDBName(opts.ProjectPath) + localTestDBSuffix,
		},
		EnsureDB:          true,
		SkipBuild:         opts.SkipBuild,
		Env:               env,
		ConstantOverrides: opts.ConstantOverrides,
		RuntimeLogPath:    logPath,
		Stdout:            w,
		Stderr:            w,
	}
}
