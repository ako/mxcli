// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// webclient_supervisor.go owns the incremental web client bundler for the
// lifetime of a `run --local --watch` loop (#971). Two things went wrong while the
// loop held a bare *WebClientWatcher:
//
//   - A bundler that exited stayed dead. Every later change waited on it, got
//     "watcher exited", and was skipped — the edit never reached the browser and
//     only restarting `run --local` recovered.
//   - A recovery re-bundle (a dangling chunk, or /dist/index.js gone after a
//     runtime restart) ran a one-shot production bundle in the same web/
//     directory while the watcher was still running, so two rollup processes
//     wrote web/dist at once.
//
// The supervisor is the one place a bundler is started or stopped, so both are
// structural: Rebundle replaces the watcher (stop, then start — never both
// running), and EnsureAlive restarts one that has exited, with backoff.

// clientBundler is the part of *WebClientWatcher the watch loop drives — a seam
// so the lifecycle is testable with fakes.
type clientBundler interface {
	Generation() int
	WaitForRebuild(sinceGen int, settle, buildTimeout time.Duration) (bool, error)
	Exited() bool
	Log() string
	Stop() error
}

// bundlerSupervisor keeps one incremental bundler alive.
type bundlerSupervisor struct {
	// start launches a fresh bundler and blocks until its first bundle. A nil
	// bundler with a nil error means the deployment needs none.
	start func() (clientBundler, error)
	out   io.Writer

	cur  clientBundler // the live bundler; nil when none is running
	want bool          // the deployment has an incremental bundler to keep alive

	failures int       // consecutive failed starts
	nextTry  time.Time // no restart attempt before this (backoff)
	now      func() time.Time
}

// newBundlerSupervisor wraps the bundler the boot started. initial may be nil
// (Mendix 11.14+ or the classic client): the supervisor then has nothing to keep
// alive and every method is a no-op.
func newBundlerSupervisor(initial *WebClientWatcher, start func() (*WebClientWatcher, error), out io.Writer) *bundlerSupervisor {
	s := &bundlerSupervisor{
		start: func() (clientBundler, error) {
			wc, err := start()
			if wc == nil {
				return nil, err // a typed nil must not become a non-nil interface
			}
			return wc, err
		},
		out: out,
		now: time.Now,
	}
	if initial != nil {
		s.cur, s.want = initial, true
	}
	return s
}

// Generation is the live bundler's success count, 0 when none is running.
func (s *bundlerSupervisor) Generation() int {
	if s == nil || s.cur == nil {
		return 0
	}
	return s.cur.Generation()
}

// WaitForRebuild waits on the live bundler; with none it has nothing to wait for.
func (s *bundlerSupervisor) WaitForRebuild(sinceGen int, settle, buildTimeout time.Duration) (bool, error) {
	if s == nil || s.cur == nil {
		return false, nil
	}
	return s.cur.WaitForRebuild(sinceGen, settle, buildTimeout)
}

// Wanted reports whether this deployment has an incremental bundler at all.
func (s *bundlerSupervisor) Wanted() bool { return s != nil && s.want }

// Restart replaces the bundler: the old one is stopped and reaped BEFORE the new
// one starts, so two bundlers never write web/dist at once. The new bundler's
// first build is a full bundle of the current source (re-globbing web/pages), so
// it doubles as the recovery re-bundle. Returns once that bundle has landed.
func (s *bundlerSupervisor) Restart() error {
	if s == nil || !s.want {
		return nil
	}
	if s.cur != nil {
		_ = s.cur.Stop()
		s.cur = nil
	}
	next, err := s.start()
	if err != nil {
		if next != nil {
			_ = next.Stop()
		}
		s.failures++
		s.nextTry = s.now().Add(bundlerRestartBackoff(s.failures))
		return err
	}
	if next == nil {
		// The deployment no longer has a bundler to keep (it changed shape, e.g.
		// an upgrade to a Mendix that bundles itself). Nothing to supervise.
		s.want = false
		return nil
	}
	s.cur = next
	s.failures = 0
	s.nextTry = time.Time{}
	return nil
}

// Rebundle is the recovery re-bundle under --watch: a fresh bundler, never a
// second one alongside the first.
func (s *bundlerSupervisor) Rebundle() error { return s.Restart() }

// EnsureAlive restarts the bundler when it has exited (or a previous start
// failed). It reports whether it restarted one. Within the backoff window after
// a failed start it does not retry and returns an error saying when it will.
func (s *bundlerSupervisor) EnsureAlive() (bool, error) {
	if s == nil || !s.want {
		return false, nil
	}
	if s.cur != nil && !s.cur.Exited() {
		return false, nil
	}
	if wait := s.nextTry.Sub(s.now()); wait > 0 {
		return false, fmt.Errorf("web client bundler is down after %d failed restart(s); next attempt in %s",
			s.failures, wait.Round(time.Second))
	}
	if s.cur != nil {
		fmt.Fprintf(s.out, "  web client bundler exited unexpectedly; restarting it...%s\n", indentTail(s.cur.Log(), 10))
	} else {
		fmt.Fprintf(s.out, "  restarting web client bundler (attempt %d)...\n", s.failures+1)
	}
	if err := s.Restart(); err != nil {
		return false, fmt.Errorf("restarting web client bundler (next attempt in %s): %w",
			bundlerRestartBackoff(s.failures), err)
	}
	if s.want {
		fmt.Fprintln(s.out, "  web client bundler restarted")
	}
	return s.want, nil
}

// AwaitRebuild waits for the incremental re-bundle a serve build triggered, and
// recovers when the bundler cannot deliver it. Two failures used to drop the
// change outright (#971):
//
//   - the bundler exited (killed, crashed) — every later change then failed with
//     "watcher exited" too;
//   - the incremental build failed on a transient state of the source the serve
//     build was rewriting, e.g. "ENOTDIR: stat …/web/pages/<Page>.js/package.json"
//     after an entity was added — and the next change failed the same way.
//
// Either way a fresh bundler bundles the source as it now stands, so it is
// restarted once; only when that fails too is the error the caller's. errOut
// receives the incremental failure the recovery is answering.
func (s *bundlerSupervisor) AwaitRebuild(sinceGen int, settle, buildTimeout time.Duration, errOut io.Writer) (bool, error) {
	rebuilt, err := s.WaitForRebuild(sinceGen, settle, buildTimeout)
	if err == nil || !s.Wanted() {
		return rebuilt, err
	}
	fmt.Fprintf(errOut, "  incremental web client rebuild failed: %s\n", firstLine(err.Error()))
	fmt.Fprintln(s.out, "  re-bundling with a fresh web client bundler...")
	if rerr := s.Restart(); rerr != nil {
		return false, fmt.Errorf("fresh bundler failed too (next attempt in %s): %w", bundlerRestartBackoff(s.failures), rerr)
	}
	return s.want, nil
}

// BuildReleasingWebDir runs a serve build, and when it fails because the
// bundler holds deployment/web open, stops the bundler and builds again (#1342).
//
// Windows refuses to delete a directory that is any process's working
// directory, and the bundler's is web/ — it has to be: mxbuild's pages plugin
// resolves page paths against cwd. A page or microflow edit rewrites files
// inside web/ and is unaffected, but a domain model change makes mxbuild
// recreate web/ itself, and that failed on every attempt with "The process
// cannot access the file '…\deployment\web' because it is being used by
// another process". Killing the bundler by hand did not help either: the watch
// loop's EnsureAlive restarted it just before the next build.
//
// After a successful retry a fresh bundler is started; its first build is a
// full bundle of the recreated web/, so rebundled tells the caller there is no
// incremental rebuild left to wait for. When the retry fails too, the bundler
// is left down for EnsureAlive to restart on the next change. On POSIX the
// directory is removed regardless and the retry never fires.
func (s *bundlerSupervisor) BuildReleasingWebDir(build func() (*BuildResult, error)) (res *BuildResult, rebundled bool, err error) {
	res, err = build()
	if err != nil || !s.Wanted() || s.cur == nil || !webDirInUse(res) {
		return res, false, err
	}
	fmt.Fprintln(s.out, "  deployment/web is held open by the web client bundler; stopping it and rebuilding...")
	s.Stop()
	res, err = build()
	if err != nil || !res.OK() {
		return res, false, err
	}
	if rerr := s.Restart(); rerr != nil {
		fmt.Fprintf(s.out, "  restarting web client bundler failed (next attempt in %s): %v\n",
			bundlerRestartBackoff(s.failures), rerr)
		return res, false, nil
	}
	return res, s.want, nil
}

// webDirInUsePath matches a quoted path that is deployment/web or lies under
// it, in either separator style and in JSON-escaped form.
var webDirInUsePath = regexp.MustCompile(`[\\/]web([\\/][^'"]*)?['"]`)

// webDirInUse reports whether a failed serve build failed on a sharing
// violation for deployment/web or a file under it.
func webDirInUse(res *BuildResult) bool {
	if res == nil || res.OK() {
		return false
	}
	text := res.Message + "\n" + string(res.Raw)
	return strings.Contains(text, "being used by another process") && webDirInUsePath.MatchString(text)
}

// firstLine is s up to its first newline.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Stop stops the live bundler, if any.
func (s *bundlerSupervisor) Stop() {
	if s == nil || s.cur == nil {
		return
	}
	_ = s.cur.Stop()
	s.cur = nil
}

// bundlerRestartBackoff is the wait after the n-th consecutive failed start:
// 2s, 4s, 8s ... capped at a minute, so a bundler that cannot start does not
// cost a full start attempt on every rebuild.
func bundlerRestartBackoff(failures int) time.Duration {
	if failures <= 0 {
		return 0
	}
	d := 2 * time.Second
	for i := 1; i < failures && d < time.Minute; i++ {
		d *= 2
	}
	if d > time.Minute {
		d = time.Minute
	}
	return d
}

// clientRebundler picks the recovery re-bundle for ensureClientServed. Under
// --watch with a live incremental bundler it is the supervisor's Rebundle — a
// one-shot BuildWebClient there would run a second rollup on web/dist alongside
// the watcher (#971). Otherwise it is the one-shot build.
func clientRebundler(sup *bundlerSupervisor, opts WebClientOptions) func() error {
	if sup.Wanted() {
		return sup.Rebundle
	}
	return func() error { return oneShotWebClientBuild(opts) }
}

// oneShotWebClientBuild is BuildWebClient, as a var so a test can observe when a
// one-shot bundle runs.
var oneShotWebClientBuild = BuildWebClient

// indentTail formats the last n lines of a log for appending to a message.
func indentTail(log string, n int) string {
	log = strings.TrimRight(log, "\n")
	if log == "" {
		return ""
	}
	lines := strings.Split(log, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return "\n    " + strings.Join(lines, "\n    ")
}
