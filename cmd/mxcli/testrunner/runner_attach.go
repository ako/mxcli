// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"io"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// attachedApp is a running app someone else owns, reached through its handshake.
//
// It satisfies the same two needs bootForTests does — a client for the endpoint,
// and a way to apply a model change — but owns neither the runtime nor the build
// server, so it must never stop them.
type attachedApp struct {
	client *endpointClient
	serve  *docker.ServeServer
	ctrl   *docker.RuntimeController
	hs     *Handshake
}

// attach connects to an app already hosting the test endpoint.
func attach(opts RunOptions, w io.Writer) (*attachedApp, error) {
	hs, err := ReadHandshake(opts.ProjectPath)
	if err != nil {
		return nil, err
	}

	client := newEndpointClient(hs.AppPort, hs.Token)
	if err := client.ping(); err != nil {
		return nil, fmt.Errorf("the app on port %d is not answering the test endpoint: %w\n"+
			"  The handshake looks live, so the app may still be starting. Retry in a moment.", hs.AppPort, err)
	}

	fmt.Fprintf(w, "Attached to the app on port %d (pid %d) — no boot needed.\n", hs.AppPort, hs.PID)
	// The dev app's database is the developer's, and these tests are about to
	// write to it. That is the whole trade --attach makes, so say it every time
	// rather than burying it in the docs.
	fmt.Fprintln(w, "  NOTE: tests run against the running app's database, not a scratch one.")

	return &attachedApp{
		client: client,
		serve:  &docker.ServeServer{Host: "127.0.0.1", Port: hs.ServePort},
		// The admin password, not the endpoint token — different secrets.
		ctrl: docker.NewRuntimeController(docker.M2EEOptions{
			Host:  "127.0.0.1",
			Port:  hs.AdminPort,
			Token: hs.AdminPass,
		}),
		hs: hs,
	}, nil
}

// applyModelChange rebuilds the project through the dev loop's serve server and
// applies the result through its admin API.
//
// Driving the other process's services rather than requiring it to be in
// --watch: both are plain loopback APIs, so an attach can apply its own
// injections deterministically instead of waiting to see whether someone else's
// watcher noticed. A dev loop that *is* watching may also rebuild — harmless,
// since both produce the same deployment from the same source.
func (a *attachedApp) endpoint() *endpointClient { return a.client }

// adminOptions reaches the attached app's admin API — the same plane
// applyModelChange drives, and where @verify's OQL runs.
func (a *attachedApp) adminOptions() docker.M2EEOptions {
	return docker.M2EEOptions{
		Host:   "127.0.0.1",
		Port:   a.hs.AdminPort,
		Token:  a.hs.AdminPass,
		Direct: true,
	}
}

func (a *attachedApp) applyModelChange(projectPath string) (string, error) {
	build, err := a.serve.Build(docker.BuildRequest{Target: docker.TargetDeploy, ProjectFilePath: projectPath})
	if err != nil {
		return "", fmt.Errorf("rebuilding through the attached app's build server on port %d: %w", a.hs.ServePort, err)
	}
	if !build.OK() {
		return "", fmt.Errorf("build failed: %s", build.Message)
	}
	// No restart callback: the runtime belongs to the other process. A structural
	// change is refused rather than half-applied — see the error below.
	action, err := a.ctrl.ApplyBuild(build, nil)
	if err != nil {
		return action.String(), err
	}
	if action == docker.ActionRestart {
		return action.String(), fmt.Errorf("this change needs a runtime restart (an entity or association changed), " +
			"which --attach cannot do — the runtime belongs to the 'mxcli run --local' process.\n" +
			"  Restart that process, or drop --attach to run against a scratch runtime.")
	}
	return action.String(), nil
}

// runAttached injects the test microflows into the already-running app, runs the
// suite, and removes them again.
//
// It deliberately does not touch the endpoint or the after-startup setting: the
// dev loop installed those and will remove them when it exits. An attach only
// owns the test microflows it adds.
func runAttached(opts RunOptions, suite *TestSuite, timeout time.Duration, w io.Writer) (*SuiteResult, error) {
	app, err := attach(opts, w)
	if err != nil {
		return nil, err
	}

	// Only the generated test microflows are ours to remove.
	injected := suite
	finish := func(result *SuiteResult, runErr error) (*SuiteResult, error) {
		fmt.Fprintln(w, "Cleaning up...")
		cleanupErr := runMDLCommands(opts.ProjectPath, dropTestFlows(injected))
		if cleanupErr == nil {
			// Leave the app serving a model that matches the project on disk;
			// otherwise the developer's next page load still runs the test flows.
			if _, err := app.applyModelChange(opts.ProjectPath); err != nil {
				fmt.Fprintf(w, "  note: the app is still serving the test microflows until its next rebuild: %v\n", err)
			}
			fmt.Fprintln(w, "  test microflows removed")
		} else {
			reportCleanup(w, cleanupErr)
		}
		if runErr != nil {
			return nil, runErr
		}
		if cleanupErr != nil {
			return result, fmt.Errorf("cleanup failed, project left modified: %w", cleanupErr)
		}
		return result, nil
	}

	fmt.Fprintln(w, "Injecting test microflows...")
	if err := execMDLScript(opts.ProjectPath, GenerateTestFlows(suite), "mxtest-flows-*.mdl"); err != nil {
		return finish(nil, fmt.Errorf("injecting test microflows: %w", err))
	}
	if _, err := app.applyModelChange(opts.ProjectPath); err != nil {
		return finish(nil, err)
	}

	if opts.Watch {
		return runAttachedWatch(opts, app, suite, timeout, w, finish, func(s *TestSuite) { injected = s })
	}

	result, err := runSuite(app.client, app.adminOptions(), suite, opts, w)
	if err != nil {
		return finish(nil, err)
	}
	result, err = finish(result, nil)
	if result != nil {
		PrintResults(w, result, opts.Color)
		if jerr := writeJUnit(opts, result, w); jerr != nil && err == nil {
			err = jerr
		}
	}
	return result, err
}

// dropTestFlows returns the DROP statements for a suite's generated microflows.
func dropTestFlows(suite *TestSuite) []string {
	if suite == nil {
		return nil
	}
	cmds := make([]string, 0, len(suite.Tests))
	for _, tc := range suite.Tests {
		cmds = append(cmds, "DROP MICROFLOW "+testFlowName(tc))
	}
	return cmds
}
