// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"io"
	"path/filepath"

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
		// A scratch deployment tree, for the same reason as the scratch database:
		// the default is <project dir>/deployment, which is what a concurrent
		// `mxcli run --local` is serving the browser bundle from. Rebuilding it
		// headless leaves that app serving a 404 for /dist/index.js — 200 and a
		// blank page, with nothing reported at either end (FINDINGS §62).
		DeployDir: filepath.Join(filepath.Dir(opts.ProjectPath), ".mxcli", localTestDeployDir),
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
