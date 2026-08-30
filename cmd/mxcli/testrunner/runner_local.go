// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// Local test runs deliberately do not share the dev loop's ports, database or
// DEPLOYMENT DIRECTORY: `mxcli run --local` may well be serving the same project
// (that is the point of the warm loop), and a test run must neither refuse to
// start because of it, nor write its fixtures into the database the developer is
// looking at, nor rebuild the directory the running app serves the browser from.
//
// The deployment directory was the one shared resource left off this list, and
// it is the one the BROWSER reads. A headless test boot does not bundle the web
// client, so its build left the running app serving Mendix's SPA shell over a
// 404 for /dist/index.js — HTTP 200, blank page, no error at either end
// (mxcli-formula1 FINDINGS §62).
const (
	localTestAppPort   = 8081
	localTestAdminPort = 8091
	localTestServePort = 6544
	// localTestDBSuffix is appended to the project's local database name.
	localTestDBSuffix = "_test"
	// localTestDeployDir is the test boot's own deployment tree, under the
	// project's .mxcli/ — gitignored, and already where the test runtime log
	// lives, so a scratch build tree does not surface as untracked files.
	localTestDeployDir = "deployment-test"
)

// runLocalAndCapture builds the project and boots mxcli's own runtime — no
// Docker — then reads the test runner's output out of the runtime log.
func runLocalAndCapture(opts RunOptions, timeout time.Duration, w io.Writer) (string, error) {
	logPath := filepath.Join(filepath.Dir(opts.ProjectPath), ".mxcli", "test-runtime.log")
	// The log is appended across runs, so remember where this run starts.
	offset := fileSize(logPath)

	fmt.Fprintln(w, "Starting local runtime (no Docker)...")
	// The runner reports through an after-startup microflow, so its LOG output is
	// produced DURING the start action — before the runtime's own log subscriber
	// is attached. What carries it is the JVM console tee, which is live from
	// spawn. Verified on 11.12.1; registering the subscriber early instead is not
	// an option, the runtime rejects it pre-start with a LoggingException. If a
	// future runtime stops echoing to the console the failure is loud, not
	// silent: unseen tests are reported as errors.
	app, err := docker.StartLocalApp(localAppOptions(opts, logPath, nil, w))
	if err != nil {
		// A failing test IS a failed boot: the generated runner returns false, so
		// the runtime's after-startup action fails and `start` reports an error.
		// That is a normal test outcome, not a broken run — if the log shows the
		// runner reached a verdict, hand it back and let the results speak. (The
		// Docker path gets this for free by only ever reading the container log.)
		tail := readFrom(logPath, offset)
		if runnerReportedVerdict(tail) {
			return tail, nil
		}
		if tail != "" {
			return tail, fmt.Errorf("local runtime: %w", err)
		}
		return "", fmt.Errorf("local runtime: %w", err)
	}
	defer app.Stop()

	fmt.Fprintf(w, "Waiting for test execution (timeout: %s)...\n", timeout)
	return waitForTestLog(logPath, offset, timeout, w, opts.Verbose)
}

// waitForTestLog polls the runtime log from offset until the run reports a
// terminal marker or the timeout expires, returning everything this run wrote.
//
// Polling rather than following: the after-startup microflow normally completes
// inside the start action, so the output is usually already on disk by the time
// this is called — the loop exists for the case where it is not.
func waitForTestLog(path string, offset int64, timeout time.Duration, w io.Writer, verbose bool) (string, error) {
	deadline := time.Now().Add(timeout)
	echoed := 0

	for {
		content := readFrom(path, offset)

		if verbose {
			lines := splitLines(content)
			for ; echoed < len(lines); echoed++ {
				fmt.Fprintln(w, lines[echoed])
			}
		}

		if done, failMsg := scanTestLog(content); done {
			if failMsg != "" {
				return content, fmt.Errorf("runtime failed: %s", failMsg)
			}
			return content, nil
		}

		if time.Now().After(deadline) {
			return content, fmt.Errorf("timeout after %s waiting for test completion", timeout)
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// scanTestLog reports whether the log has reached a terminal state, and the
// failure line when the runtime failed rather than the tests completing. The
// markers match the Docker path's, so both modes stop on the same conditions.
func scanTestLog(content string) (done bool, failMsg string) {
	for _, line := range splitLines(content) {
		switch {
		case strings.Contains(line, "Error starting runtime"),
			strings.Contains(line, "Critical error"),
			strings.Contains(line, "After startup microflow should return a boolean"):
			return true, line
		case strings.Contains(line, "Successfully ran after-startup-action"),
			runnerReportedVerdict(line):
			return true, ""
		}
	}
	return false, ""
}

// runnerReportedVerdict reports whether the test runner got far enough to
// produce results — either it finished, or the runtime said the after-startup
// action failed, which is what a failing test looks like from outside.
func runnerReportedVerdict(content string) bool {
	return strings.Contains(content, "MXTEST:END:") ||
		strings.Contains(content, "after-startup-action failed") ||
		strings.Contains(content, "After-startup action failed")
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}

// fileSize returns the file's current size, or 0 when it does not exist yet.
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// readFrom returns the file's content from offset onward, or "" if unreadable.
func readFrom(path string, offset int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	var b strings.Builder
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		b.WriteString(sc.Text())
		b.WriteByte('\n')
	}
	return b.String()
}
