// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/mendixlabs/mxcli/sdk/mpr"
)

// runlocal.go is the `mxcli run --local` orchestrator: a warm, Docker-free dev
// loop. It keeps a mxbuild --serve process and a standalone runtime hot, so a
// model change rebuilds incrementally (~1s) and is applied by a hot reload_model
// (page/microflow/text) or a runtime restart (entity/view/association) — the
// serve build's restartRequired flag decides which. See
// docs/11-proposals/PROPOSAL_mxcli_dev_warm_loop.md.

// LocalRunOptions configures RunLocal.
type LocalRunOptions struct {
	// ProjectPath is the .mpr file.
	ProjectPath string
	// DeployDir is where the serve Deploy target writes (default <projectDir>/deployment).
	DeployDir string
	// MxBuildPath overrides mxbuild resolution (optional).
	MxBuildPath string
	// AppPort / AdminPort / ServePort default to 8080 / 8090 / 6543.
	AppPort   int
	AdminPort int
	ServePort int
	// AdminPass is the M2EE admin password (default is a fixed local-dev value).
	AdminPass string
	// DB is the Postgres the runtime connects to (devcontainer defaults applied).
	DB DBConfig
	// Watch keeps running, rebuilding+applying on every project change.
	Watch bool
	// PollInterval is how often Watch checks for changes (default 1s).
	PollInterval time.Duration
	Stdout       io.Writer
	Stderr       io.Writer
}

// defaultLocalAdminPass is the admin password for a local dev runtime. The admin
// API binds to 127.0.0.1 only, so a fixed value is acceptable for local use.
const defaultLocalAdminPass = "mxcli-local-dev"

func (o *LocalRunOptions) applyDefaults() {
	if o.DeployDir == "" {
		o.DeployDir = filepath.Join(filepath.Dir(o.ProjectPath), "deployment")
	}
	if o.AppPort == 0 {
		o.AppPort = 8080
	}
	if o.AdminPort == 0 {
		o.AdminPort = 8090
	}
	if o.ServePort == 0 {
		o.ServePort = 6543
	}
	if o.AdminPass == "" {
		o.AdminPass = defaultLocalAdminPass
	}
	if o.PollInterval == 0 {
		o.PollInterval = time.Second
	}
	if o.DB.Type == "" {
		o.DB.Type = "PostgreSQL"
	}
	if o.DB.Host == "" {
		o.DB.Host = "127.0.0.1:5432"
	}
	if o.DB.User == "" {
		o.DB.User = "mendix"
	}
	if o.DB.Password == "" {
		o.DB.Password = "mendix"
	}
	if o.DB.Name == "" {
		o.DB.Name = deriveDBName(o.ProjectPath)
	}
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
}

// deriveDBName turns a project file name into a safe Postgres database name:
// lowercased, non-alphanumerics collapsed to underscores, leading digit prefixed.
func deriveDBName(projectPath string) string {
	base := strings.TrimSuffix(filepath.Base(projectPath), filepath.Ext(projectPath))
	var b strings.Builder
	for _, r := range strings.ToLower(base) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		return "mxlocal"
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "db_" + name
	}
	return name
}

// ensureMxBuildRuntimeSibling makes the downloaded runtime available as a
// runtime/ sibling of the mxbuild cache's modeler/ dir. mxbuild's Deploy/serve
// javac step resolves the Mendix API from there; without it compilation fails
// with "package com.mendix.* does not exist". It is a no-op if the sibling
// already exists (symlink or real dir). Mirrors ensurePADFiles' link pattern.
func ensureMxBuildRuntimeSibling(version string, w io.Writer) error {
	mxbuildDir, err := MxBuildCacheDir(version)
	if err != nil {
		return err
	}
	dst := filepath.Join(mxbuildDir, "runtime")
	if _, err := os.Stat(dst); err == nil {
		return nil // already present (bundled or linked previously)
	}
	runtimeDir, err := RuntimeCacheDir(version)
	if err != nil {
		return err
	}
	src := filepath.Join(runtimeDir, "runtime")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("runtime not found at %s (run 'mxcli setup mxbuild' / DownloadRuntime first): %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("linking runtime into mxbuild cache: %w", err)
	}
	fmt.Fprintf(w, "  Linked runtime into mxbuild cache: %s -> %s\n", dst, src)
	return nil
}

// pingTCP dials host (host:port) to check reachability within timeout.
func pingTCP(hostPort string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", hostPort, timeout)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}

// projectMTime returns the newest mtime under the project directory, ignoring
// the deployment dir and VCS metadata. It is the change signal for Watch: it
// covers both MPR v1 (single .mpr file) and v2 (metadata + mprcontents/).
func projectMTime(projectDir, deployDir string) time.Time {
	var newest time.Time
	deployAbs, _ := filepath.Abs(deployDir)
	_ = filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "deployment" {
				return fs.SkipDir
			}
			if abs, _ := filepath.Abs(path); abs == deployAbs {
				return fs.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

// RunLocal boots the warm local dev loop: resolve tooling, start mxbuild --serve
// and a standalone runtime, do the first build+apply, then (with Watch) rebuild
// and hot-apply on every project change until interrupted.
func RunLocal(opts LocalRunOptions) error {
	opts.applyDefaults()
	w, stderr := opts.Stdout, opts.Stderr

	// 1. Detect the project's Mendix version.
	fmt.Fprintln(w, "Detecting project version...")
	reader, err := mpr.Open(opts.ProjectPath)
	if err != nil {
		return fmt.Errorf("opening project: %w", err)
	}
	pv := reader.ProjectVersion()
	reader.Close()
	version := pv.ProductVersion
	fmt.Fprintf(w, "  Mendix version: %s\n", version)

	// 2. Ensure mxbuild + runtime are cached, and linked for the serve javac step.
	fmt.Fprintln(w, "Ensuring MxBuild and runtime are available...")
	if _, err := DownloadMxBuild(version, w); err != nil {
		return fmt.Errorf("setting up mxbuild: %w", err)
	}
	installPath, err := resolveRuntimeInstall(version, w)
	if err != nil {
		return fmt.Errorf("setting up runtime: %w", err)
	}

	// 3. Check the database is reachable (we don't provision it here).
	if err := pingTCP(opts.DB.Host, 3*time.Second); err != nil {
		return fmt.Errorf("database not reachable at %s: %w\n"+
			"  Start Postgres and create the '%s' database (user %q), or pass --db-* flags.",
			opts.DB.Host, err, opts.DB.Name, opts.DB.User)
	}

	// 4. Start the warm build server.
	fmt.Fprintln(w, "Starting mxbuild --serve...")
	serve, err := StartServe(ServeOptions{
		Version: version,
		Host:    "127.0.0.1",
		Port:    opts.ServePort,
	})
	if err != nil {
		return fmt.Errorf("starting mxbuild serve: %w", err)
	}
	defer serve.Stop()

	// 5. First build (cold — loads the model).
	fmt.Fprintln(w, "Building (first build is cold, ~10-15s)...")
	build, err := serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: opts.ProjectPath})
	if err != nil {
		return fmt.Errorf("initial build: %w", err)
	}
	if !build.OK() {
		return fmt.Errorf("initial build failed: %s\n%s", build.Message, string(build.Raw))
	}

	// 6. Boot the runtime against the fresh deployment.
	rt, err := StartLocalRuntime(LocalRuntimeOptions{
		DeployDir:   opts.DeployDir,
		InstallPath: installPath,
		// JavaHome left empty: StartLocalRuntime resolves JDK 21.
		AppPort:   opts.AppPort,
		AdminPort: opts.AdminPort,
		AdminPass: opts.AdminPass,
		DB:        opts.DB,
		Stdout:    w,
		Stderr:    stderr,
	})
	if err != nil {
		return err
	}
	defer rt.Stop()

	fmt.Fprintf(w, "\nApp is running at %s\n", rt.AppURL())

	// 7. Stay up until interrupted. With --watch, rebuild + hot-apply on every
	// project change; otherwise just keep the runtime serving.
	if opts.Watch {
		return watchAndApply(opts, serve, rt)
	}
	fmt.Fprintln(w, "(run with --watch to rebuild and hot-apply on changes; Ctrl-C to stop)")
	waitForInterrupt()
	fmt.Fprintln(w, "\nShutting down...")
	return nil
}

// resolveRuntimeInstall returns the directory to use as MX_INSTALL_PATH (its
// runtime/ child holds the launcher and Mendix libraries), downloading the
// runtime only when nothing usable is cached. It also makes the runtime visible
// to mxbuild's serve javac step as a runtime/ sibling of modeler/.
//
// Preference order:
//  1. the dedicated runtime cache (~/.mxcli/runtime/{v}), if it has the launcher;
//  2. a runtime/ already inside the mxbuild cache (bundled or previously linked);
//  3. download the runtime, then link it into the mxbuild cache.
func resolveRuntimeInstall(version string, w io.Writer) (string, error) {
	if p := CachedRuntimePath(version); p != "" {
		if err := ensureMxBuildRuntimeSibling(version, w); err != nil {
			return "", err
		}
		return p, nil
	}
	mxbuildDir, err := MxBuildCacheDir(version)
	if err != nil {
		return "", err
	}
	launcher := filepath.Join(mxbuildDir, "runtime", "launcher", "runtimelauncher.jar")
	if fi, err := os.Stat(launcher); err == nil && !fi.IsDir() {
		// The mxbuild cache already carries a usable runtime/; use it directly.
		return mxbuildDir, nil
	}
	if _, err := DownloadRuntime(version, w); err != nil {
		return "", err
	}
	if err := ensureMxBuildRuntimeSibling(version, w); err != nil {
		return "", err
	}
	return CachedRuntimePath(version), nil
}

// waitForInterrupt blocks until the process receives SIGINT or SIGTERM.
func waitForInterrupt() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh
}

// watchAndApply polls the project for changes and applies each rebuild until the
// user interrupts (Ctrl-C). StartLocalRuntime already resolved the JVM; here we
// only rebuild via serve and let the RuntimeController decide reload vs restart.
func watchAndApply(opts LocalRunOptions, serve *ServeServer, rt *LocalRuntime) error {
	w := opts.Stdout
	projectDir := filepath.Dir(opts.ProjectPath)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	fmt.Fprintln(w, "Watching for changes (Ctrl-C to stop)...")
	last := projectMTime(projectDir, opts.DeployDir)
	ticker := time.NewTicker(opts.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sigCh:
			fmt.Fprintln(w, "\nShutting down...")
			return nil
		case <-ticker.C:
			now := projectMTime(projectDir, opts.DeployDir)
			if !now.After(last) {
				continue
			}
			last = now
			fmt.Fprintln(w, "Change detected, rebuilding...")
			start := time.Now()
			build, err := serve.Build(BuildRequest{Target: TargetDeploy, ProjectFilePath: opts.ProjectPath})
			if err != nil {
				fmt.Fprintf(opts.Stderr, "  build error: %v\n", err)
				continue
			}
			if !build.OK() {
				fmt.Fprintf(opts.Stderr, "  build failed: %s\n", build.Message)
				continue
			}
			action, err := rt.Controller().ApplyBuild(build, rt.Restart)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "  apply (%s) failed: %v\n", action, err)
				continue
			}
			// Refresh last after the apply so edits made during the build are caught.
			last = projectMTime(projectDir, opts.DeployDir)
			fmt.Fprintf(w, "  applied via %s in %s -> %s\n", action, time.Since(start).Round(time.Millisecond), rt.AppURL())
		}
	}
}
