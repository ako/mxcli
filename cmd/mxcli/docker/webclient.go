// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// webclient.go builds the browser client bundle (web/dist/index.js) for a
// deployment. mxbuild's serve Deploy target writes the client *source*
// (web/index.js, web/pages/*, web/widgets/*) and a self-contained
// web/rollup.config.mjs, but it does NOT run the rollup step that bundles them
// into web/dist/. A standalone-served app therefore 404s on /dist/index.js and
// renders blank. This runs mxbuild's own bundled rollup runner to close that gap.
//
// The runner (modeler/tools/node/rollup-runner.mjs) with NODE_ENV=production does
// a one-shot rollup() + bundle.write(config.output) into web/dist. It resolves
// `rollup` from tools/node/node_modules (relative to the runner) and loads
// rollup.config.mjs from its working directory (the deployment web dir).

// WebClientOptions configures BuildWebClient.
type WebClientOptions struct {
	// DeployDir is the deployment directory; its web/ child holds the client
	// source and rollup.config.mjs, and receives web/dist/.
	DeployDir string
	// MxBuildPath is <cache>/modeler/mxbuild; the node tooling is resolved from
	// its sibling tools/node directory.
	MxBuildPath string
	// Timeout bounds the bundle build (default 5m).
	Timeout time.Duration
	// Stdout receives a short progress line (default discarded).
	Stdout io.Writer
}

// resolveNodeTooling returns the bundled node binary and rollup-runner.mjs paths
// from an mxbuild binary path (<cache>/modeler/mxbuild -> <cache>/modeler/tools/node).
func resolveNodeTooling(mxbuildPath string) (nodeBin, runner string, err error) {
	toolsNode := filepath.Join(filepath.Dir(mxbuildPath), "tools", "node")
	runner = filepath.Join(toolsNode, "rollup-runner.mjs")
	if _, err := os.Stat(runner); err != nil {
		return "", "", fmt.Errorf("rollup runner not found at %s (incomplete mxbuild?): %w", runner, err)
	}
	nodeBin = findNodeBinary(toolsNode)
	if nodeBin == "" {
		return "", "", fmt.Errorf("bundled node binary not found under %s", toolsNode)
	}
	return nodeBin, runner, nil
}

// findNodeBinary locates mxbuild's bundled node under tools/node/<platform>/.
// It prefers the GOOS/GOARCH-matched directory, then falls back to any platform
// dir containing a node binary (so a cache for one arch still resolves cleanly).
func findNodeBinary(toolsNode string) string {
	exe := "node"
	if runtime.GOOS == "windows" {
		exe = "node.exe"
	}
	archAlias := map[string]string{"amd64": "x64", "arm64": "arm64", "386": "x86"}
	arch := archAlias[runtime.GOARCH]
	if arch == "" {
		arch = runtime.GOARCH
	}
	osAlias := map[string]string{"darwin": "darwin", "linux": "linux", "windows": "win"}
	goos := osAlias[runtime.GOOS]
	if goos == "" {
		goos = runtime.GOOS
	}
	// Preferred exact match, then common alternates, then a glob fallback.
	candidates := []string{
		filepath.Join(toolsNode, goos+"-"+arch, exe),
		filepath.Join(toolsNode, exe),
	}
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c
		}
	}
	matches, _ := filepath.Glob(filepath.Join(toolsNode, "*", exe))
	for _, m := range matches {
		if fi, err := os.Stat(m); err == nil && !fi.IsDir() {
			return m
		}
	}
	return ""
}

// BuildWebClient bundles the deployment's browser client into web/dist. Call it
// after each serve Deploy build. It is a no-op when the build already produced a
// bundle (Mendix 11.14+); otherwise it runs mxbuild's rollup runner. Returns an
// error if the web dir, tooling, or rollup build fails, or if there is neither a
// bundle nor a config to build one from.
func BuildWebClient(opts WebClientOptions) error {
	w := opts.Stdout
	if w == nil {
		w = io.Discard
	}
	webDir := filepath.Join(opts.DeployDir, "web")
	if fi, err := os.Stat(filepath.Join(webDir, "rollup.config.mjs")); err != nil || fi.IsDir() {
		// Mendix 11.14 closed this gap upstream: its build writes web/dist/
		// itself and no longer emits a rollup config, because there is nothing
		// left to configure. Measured on a blank app, same build target:
		//
		//   11.13.0   rollup.config.mjs PRESENT   dist/index.js ABSENT
		//   11.14.0   rollup.config.mjs ABSENT    dist/index.js PRESENT
		//
		// The old gate tested for the config, so on 11.14 it failed on the
		// absence of a file whose purpose had been served — fatally, at both
		// call sites, for EVERY 11.14 app (ako/mxcli-ledger #146).
		//
		// Gate on the gap instead of on the shape one version happened to leave
		// behind: if the bundle is already there, this step has nothing to do.
		// The config is still required when it is NOT there, because then the
		// rollup run is the only thing that can produce it.
		if WebClientBundled(opts.DeployDir) {
			fmt.Fprintln(w, "  Web client already bundled by mxbuild; skipping rollup step")
			return nil
		}
		return fmt.Errorf("no rollup.config.mjs and no bundle at %s\n"+
			"  Mendix 11.13 and earlier emit a rollup config for mxcli to run; 11.14+ writes\n"+
			"  the bundle itself. Neither is present, so the build did not produce a client:\n"+
			"  run a serve Deploy build first (or delete deployment/ if it was built by an\n"+
			"  older Mendix version).",
			webClientBundlePath(opts.DeployDir))
	}
	nodeBin, runner, err := resolveNodeTooling(opts.MxBuildPath)
	if err != nil {
		return err
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	start := time.Now()
	cmd := exec.Command(nodeBin, runner)
	cmd.Dir = webDir
	cmd.Env = append(os.Environ(),
		"NODE_ENV=production",
		"MX_WEB_CLIENT_BUILD_LOG="+filepath.Join(opts.DeployDir, "log", "web-client-build.log"),
	)
	log := &syncBuffer{}
	cmd.Stdout = log
	cmd.Stderr = log

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching web client build: %w", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("web client build failed: %w\n%s", err, log.String())
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return fmt.Errorf("web client build timed out after %s", timeout)
	}

	if !WebClientBundled(opts.DeployDir) {
		return fmt.Errorf("web client build reported success but %s is missing:\n%s",
			webClientBundlePath(opts.DeployDir), log.String())
	}
	fmt.Fprintf(w, "  Web client bundled in %s\n", time.Since(start).Round(time.Millisecond))
	return nil
}

// webClientBundlePath is the one file whose absence is the black screen: the
// shell loads, paints the theme's background, and never starts the client.
func webClientBundlePath(deployDir string) string {
	return filepath.Join(deployDir, "web", "dist", "index.js")
}

// WebClientBundled reports whether the deployment currently has a browser
// bundle to serve.
func WebClientBundled(deployDir string) bool {
	fi, err := os.Stat(webClientBundlePath(deployDir))
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

// EnsureWebClientBundle re-bundles when the bundle is missing, and reports
// whether it had to.
//
// Bundling before the runtime boots is not enough. The boot runs Gradle
// `clean-custom-classes compile package`, and when Gradle has work to do — a new
// Java action, a full recompile — its package pass repopulates deployment/web
// and takes dist/ with it, deleting the bundle written seconds earlier by a
// previous step of the same command. Nothing reports this: `mxcli check` passes,
// the build succeeds, the runtime logs nothing, `curl /` returns 200 with a valid
// HTML shell, and every OData service answers. Only a browser sees the black
// screen, which is how it survives restarts (mxcli-formula1 §35).
//
// So the bundle is verified *after* the boot rather than trusted from before it.
// When Gradle had nothing to do the check is a stat and costs nothing, which is
// why this is a guard rather than a reordering — the bundle still exists before
// the boot for the common case where the app is reachable immediately.
func EnsureWebClientBundle(opts WebClientOptions) (bool, error) {
	return ensureWebClientBundle(opts.DeployDir, opts.Stdout, func() error {
		return BuildWebClient(opts)
	})
}

// ReportLostWebClientBundle says so when a boot destroyed a bundle that existed
// before it, and reports whether it did.
//
// This is the second way into §35: `mxcli test --local` boots the same way and
// its Gradle package pass wipes the bundle too, so a test run between a `run
// --local` and a browser leaves the app serving a black screen even though
// nothing was rebuilt. Tests are headless and do not need the bundle, and
// re-bundling would cost ~30s on a loop whose whole point is two seconds — so
// this warns with the remedy instead of paying for it uninvited.
func ReportLostWebClientBundle(deployDir string, hadBundle bool, w io.Writer) bool {
	if w == nil || !hadBundle || WebClientBundled(deployDir) {
		return false
	}
	fmt.Fprintf(w, "Note: this boot's packaging step removed the browser bundle at %s.\n"+
		"  The app will render a blank page until it is rebuilt — re-run 'mxcli run --local' to restore it.\n",
		webClientBundlePath(deployDir))
	return true
}

// ensureWebClientBundle holds the decision, separated from the node invocation
// so it can be tested without mxbuild's tooling.
func ensureWebClientBundle(deployDir string, w io.Writer, bundle func() error) (bool, error) {
	if w == nil {
		w = io.Discard
	}
	if WebClientBundled(deployDir) {
		return false, nil
	}
	fmt.Fprintln(w, "Web client bundle was removed by the boot's packaging step; re-bundling...")
	if err := bundle(); err != nil {
		return true, fmt.Errorf("re-bundling web client after boot: %w\n"+
			"  The app is running but will render a blank page until %s exists.",
			err, webClientBundlePath(deployDir))
	}
	return true, nil
}
