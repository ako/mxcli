// SPDX-License-Identifier: Apache-2.0

// Package playwright runs playwright-cli test scripts (.test.sh) against a Mendix app.
package playwright

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// VerifyOptions configures the playwright verify runner.
type VerifyOptions struct {
	// ProjectPath is the path to the .mpr file.
	ProjectPath string

	// TestFiles is the list of .test.sh file paths or directories.
	TestFiles []string

	// BaseURL is the Mendix app URL (default: http://localhost:8080).
	BaseURL string

	// Timeout per script execution.
	Timeout time.Duration

	// JUnitOutput is the path for JUnit XML output (empty = no file output).
	JUnitOutput string

	// Color enables colored console output.
	Color bool

	// Verbose shows script stdout/stderr.
	Verbose bool

	// SkipHealthCheck skips the app reachability check.
	SkipHealthCheck bool

	// KeepOpen leaves the browser session open after the run instead of closing
	// it, so the next verify (the agentic generate->verify->fix loop) reuses a
	// warm, still-authenticated browser instead of cold-launching Chromium.
	KeepOpen bool

	// Stdout for output messages.
	Stdout io.Writer

	// Stderr for error output.
	Stderr io.Writer
}

// ScriptResult represents the outcome of running one test script.
type ScriptResult struct {
	Name     string
	Path     string
	Status   Status
	Output   string
	Message  string
	Duration time.Duration
}

// SuiteResult holds results for all scripts in a verify run.
type SuiteResult struct {
	Name     string
	Scripts  []ScriptResult
	Started  time.Time
	Duration time.Duration
}

// Status represents the outcome of a script.
type Status int

const (
	StatusPass Status = iota
	StatusFail
	StatusError
	StatusSkip
)

func (s Status) String() string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusError:
		return "ERROR"
	case StatusSkip:
		return "SKIP"
	default:
		return "UNKNOWN"
	}
}

// PassCount returns the number of passing scripts.
func (sr *SuiteResult) PassCount() int {
	n := 0
	for _, s := range sr.Scripts {
		if s.Status == StatusPass {
			n++
		}
	}
	return n
}

// FailCount returns the number of failing scripts.
func (sr *SuiteResult) FailCount() int {
	n := 0
	for _, s := range sr.Scripts {
		if s.Status == StatusFail || s.Status == StatusError {
			n++
		}
	}
	return n
}

// AllPassed returns true if all scripts passed.
func (sr *SuiteResult) AllPassed() bool {
	return sr.FailCount() == 0
}

// Verify runs playwright-cli test scripts and collects results.
func Verify(opts VerifyOptions) (*SuiteResult, error) {
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}
	projectDir := ""
	if opts.ProjectPath != "" {
		projectDir = filepath.Dir(opts.ProjectPath)
	}

	// Step 1: Discover test scripts
	scripts, err := discoverScripts(opts.TestFiles)
	if err != nil {
		return nil, fmt.Errorf("discovering test scripts: %w", err)
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("no .test.sh scripts found in the provided paths")
	}
	fmt.Fprintf(w, "Found %d test script(s)\n", len(scripts))

	// Step 2: Check playwright-cli is available
	if _, err := exec.LookPath("playwright-cli"); err != nil {
		return nil, fmt.Errorf("playwright-cli not found in PATH (install with: npm install -g @playwright/cli@latest)")
	}

	// Step 3: Health check
	if !opts.SkipHealthCheck {
		fmt.Fprintf(w, "Checking app at %s...\n", baseURL)
		if err := healthCheck(baseURL); err != nil {
			return nil, fmt.Errorf("app not reachable at %s: %w", baseURL, err)
		}
		fmt.Fprintf(w, "  App is reachable\n")
	}

	// Step 4: Open browser session, or reuse a live one. A previous run with
	// --keep-open leaves the session warm; reusing it skips the ~1-2s Chromium
	// cold start and keeps any saved auth, which is the point of the shared
	// PLAYWRIGHT_CLI_SESSION. (Shared with `mxcli playwright open`.)
	browser := readBrowserName(opts.ProjectPath)
	if browser == "" {
		browser = "chromium"
	}
	if err := ensureSession(baseURL, browser, w); err != nil {
		return nil, err
	}

	// Step 5: Run each script
	result := &SuiteResult{
		Name:    "playwright-verify",
		Started: time.Now(),
	}

	for i, script := range scripts {
		name := scriptName(script)
		fmt.Fprintf(w, "  [%d/%d] %s... ", i+1, len(scripts), name)

		sr := runScript(script, projectDir, timeout, opts.Verbose, w)
		result.Scripts = append(result.Scripts, sr)

		if opts.Color {
			switch sr.Status {
			case StatusPass:
				fmt.Fprintf(w, "\033[32mPASS\033[0m (%s)\n", sr.Duration.Round(time.Millisecond))
			case StatusFail:
				fmt.Fprintf(w, "\033[31mFAIL\033[0m (%s)\n", sr.Duration.Round(time.Millisecond))
			default:
				fmt.Fprintf(w, "\033[31mERROR\033[0m (%s)\n", sr.Duration.Round(time.Millisecond))
			}
		} else {
			fmt.Fprintf(w, "%s (%s)\n", sr.Status, sr.Duration.Round(time.Millisecond))
		}

		// On failure, take a screenshot for debugging
		if sr.Status != StatusPass {
			screenshotFile := strings.TrimSuffix(filepath.Base(script), ".test.sh") + "-failure.png"
			if err := runPlaywrightCLI("screenshot", "--filename="+screenshotFile); err == nil {
				fmt.Fprintf(w, "         Screenshot saved: %s\n", screenshotFile)
			}
			if sr.Message != "" {
				fmt.Fprintf(w, "         %s\n", sr.Message)
			}
		}
	}

	// Step 6: Close browser, unless the caller wants it left warm for the next
	// run. (A script that ends with its own `playwright-cli close` still tears
	// down the shared session — that is script-authoring, not the runner.)
	if opts.KeepOpen {
		fmt.Fprintf(w, "Leaving browser session open (--keep-open); reuse it with the next verify.\n")
	} else {
		runPlaywrightCLI("close")
	}

	result.Duration = time.Since(result.Started)

	// Step 7: Print summary
	PrintResults(w, result, opts.Color)

	// Step 8: Write JUnit XML if requested
	if opts.JUnitOutput != "" {
		f, err := os.Create(opts.JUnitOutput)
		if err != nil {
			return result, fmt.Errorf("creating JUnit output: %w", err)
		}
		defer f.Close()
		if err := WriteJUnitXML(f, result); err != nil {
			return result, fmt.Errorf("writing JUnit XML: %w", err)
		}
		fmt.Fprintf(w, "JUnit XML written to: %s\n", opts.JUnitOutput)
	}

	return result, nil
}

// ListScripts discovers and prints test scripts without executing them.
func ListScripts(paths []string, w io.Writer) error {
	scripts, err := discoverScripts(paths)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "Found %d test script(s):\n", len(scripts))
	for _, s := range scripts {
		fmt.Fprintf(w, "  %s\n", s)
	}
	return nil
}

// discoverScripts finds all .test.sh files in the given paths.
func discoverScripts(paths []string) ([]string, error) {
	var scripts []string
	seen := make(map[string]bool)

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}

		if info.IsDir() {
			entries, err := os.ReadDir(p)
			if err != nil {
				return nil, fmt.Errorf("reading directory %s: %w", p, err)
			}
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(e.Name(), ".test.sh") {
					full := filepath.Join(p, e.Name())
					if !seen[full] {
						scripts = append(scripts, full)
						seen[full] = true
					}
				}
			}
		} else if strings.HasSuffix(p, ".test.sh") {
			abs, _ := filepath.Abs(p)
			if !seen[abs] {
				scripts = append(scripts, abs)
				seen[abs] = true
			}
		}
	}

	sort.Strings(scripts)
	return scripts, nil
}

// runScript executes a single .test.sh script and returns the result.
// projectDir is used as the working directory so playwright-cli can find
// the session socket opened by the runner. Falls back to the script's
// parent directory if projectDir is empty.
func runScript(path string, projectDir string, timeout time.Duration, verbose bool, w io.Writer) ScriptResult {
	start := time.Now()
	name := scriptName(path)

	cmd := exec.Command("bash", path)
	cmd.Env = os.Environ()

	var stdout, stderr bytes.Buffer
	if verbose {
		cmd.Stdout = io.MultiWriter(&stdout, w)
		cmd.Stderr = io.MultiWriter(&stderr, w)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	// Use project directory as CWD so playwright-cli finds the session
	// socket and config. Fall back to script's parent if no project set.
	if projectDir != "" {
		cmd.Dir = projectDir
	} else {
		cmd.Dir = filepath.Dir(path)
	}

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return ScriptResult{
			Name:     name,
			Path:     path,
			Status:   StatusError,
			Message:  fmt.Sprintf("failed to start: %v", err),
			Duration: time.Since(start),
		}
	}

	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		duration := time.Since(start)
		output := stdout.String() + stderr.String()

		if err != nil {
			// Extract last meaningful line as error message
			msg := lastNonEmptyLine(output)
			if msg == "" {
				msg = err.Error()
			}
			return ScriptResult{
				Name:     name,
				Path:     path,
				Status:   StatusFail,
				Output:   output,
				Message:  msg,
				Duration: duration,
			}
		}

		return ScriptResult{
			Name:     name,
			Path:     path,
			Status:   StatusPass,
			Output:   output,
			Duration: duration,
		}

	case <-time.After(timeout):
		cmd.Process.Kill()
		return ScriptResult{
			Name:     name,
			Path:     path,
			Status:   StatusError,
			Message:  fmt.Sprintf("timeout after %s", timeout),
			Duration: timeout,
		}
	}
}

// scriptName extracts a readable name from a script path.
func scriptName(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, ".test.sh")
	return name
}

// lastNonEmptyLine returns the last non-empty line from output.
func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

// healthCheck verifies the app is reachable at the given URL.
func healthCheck(baseURL string) error {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}

// readBrowserName reads browserName from .playwright/cli.config.json relative
// to the project file. Returns empty string if the file doesn't exist or
// browserName is not set.
func readBrowserName(projectPath string) string {
	if projectPath == "" {
		return ""
	}
	configPath := filepath.Join(filepath.Dir(projectPath), ".playwright", "cli.config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return ""
	}
	var config struct {
		Browser struct {
			BrowserName string `json:"browserName"`
		} `json:"browser"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ""
	}
	return config.Browser.BrowserName
}

// runPlaywrightCLI runs a playwright-cli command.
func runPlaywrightCLI(args ...string) error {
	cmd := exec.Command("playwright-cli", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// runPlaywrightCLIOutput runs a playwright-cli command and returns its combined
// stdout+stderr, for probes that need to inspect the output.
func runPlaywrightCLIOutput(args ...string) (string, error) {
	cmd := exec.Command("playwright-cli", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// originOf returns the scheme://host[:port] origin of a URL, or "" if rawURL is
// not an absolute URL. Matches what the browser's location.origin reports.
func originOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

// sessionAlive reports whether a playwright-cli session is already open AND on
// the target origin, so the runner can reuse it instead of opening a new one.
// It asks the page for its own origin: if there is no live session the eval
// fails, and if the session is on a different app the origin won't match — in
// both cases the caller opens fresh, so there is no false reuse. Substring
// matching keeps this tolerant of the CLI's exact eval output formatting.
func sessionAlive(baseURL string) bool {
	origin := originOf(baseURL)
	if origin == "" {
		return false
	}
	out, err := runPlaywrightCLIOutput("eval", "() => location.origin")
	if err != nil {
		return false
	}
	return strings.Contains(out, origin)
}

// PrintResults writes a human-readable summary.
func PrintResults(w io.Writer, result *SuiteResult, color bool) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Playwright Verify Results\n")
	fmt.Fprintf(w, "%s\n", strings.Repeat("=", 60))

	for _, s := range result.Scripts {
		var statusStr string
		if color {
			switch s.Status {
			case StatusPass:
				statusStr = "\033[32mPASS\033[0m"
			case StatusFail:
				statusStr = "\033[31mFAIL\033[0m"
			case StatusError:
				statusStr = "\033[31mERROR\033[0m"
			case StatusSkip:
				statusStr = "\033[33mSKIP\033[0m"
			}
		} else {
			statusStr = s.Status.String()
		}

		fmt.Fprintf(w, "  %s  %s", statusStr, s.Name)
		if s.Duration > 0 {
			fmt.Fprintf(w, " (%s)", s.Duration.Round(time.Millisecond))
		}
		fmt.Fprintln(w)

		if s.Message != "" && s.Status != StatusPass {
			fmt.Fprintf(w, "         %s\n", s.Message)
		}
	}

	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 60))
	total := len(result.Scripts)
	fmt.Fprintf(w, "Total: %d  Passed: %d  Failed: %d",
		total, result.PassCount(), result.FailCount())
	if result.Duration > 0 {
		fmt.Fprintf(w, "  Time: %s", result.Duration.Round(time.Millisecond))
	}
	fmt.Fprintln(w)

	if result.AllPassed() {
		if color {
			fmt.Fprintf(w, "\033[32mAll scripts passed.\033[0m\n")
		} else {
			fmt.Fprintln(w, "All scripts passed.")
		}
	} else {
		if color {
			fmt.Fprintf(w, "\033[31mSome scripts failed.\033[0m\n")
		} else {
			fmt.Fprintln(w, "Some scripts failed.")
		}
	}
}
