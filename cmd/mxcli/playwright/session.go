// SPDX-License-Identifier: Apache-2.0

package playwright

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// SessionOptions configures the lifecycle subcommands (open / status).
type SessionOptions struct {
	// BaseURL is the Mendix app URL to open / attach to.
	BaseURL string
	// ProjectPath is the .mpr path, used to read the browser from
	// .playwright/cli.config.json.
	ProjectPath string
	// Stdout for output messages.
	Stdout io.Writer
}

// ensureSession opens a playwright-cli session at baseURL, or reuses a live one
// already on the same origin — re-navigating (goto) so a rebuilt app loads fresh
// rather than keeping the pre-rebuild DOM/JS. Shared by Verify and OpenSession
// so both get identical open/reuse behavior.
func ensureSession(baseURL, browser string, w io.Writer) error {
	if sessionAlive(baseURL) {
		fmt.Fprintf(w, "Reusing browser session; reloading %s...\n", baseURL)
		if err := runPlaywrightCLI("goto", baseURL); err != nil {
			return fmt.Errorf("reloading reused session: %w", err)
		}
		return nil
	}
	fmt.Fprintf(w, "Opening browser session (%s)...\n", browser)
	if err := runPlaywrightCLI("--browser", browser, "open", baseURL); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	return nil
}

// OpenSession launches or attaches the browser session for interactive use, so
// an agent can keep one warm browser across turns instead of letting each
// `verify` own the lifecycle.
func OpenSession(opts SessionOptions) error {
	w := opts.Stdout
	if w == nil {
		w = os.Stdout
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	browser := readBrowserName(opts.ProjectPath)
	if browser == "" {
		browser = "chromium"
	}
	if err := ensureSession(baseURL, browser, w); err != nil {
		return err
	}
	fmt.Fprintf(w, "Session ready at %s\n", baseURL)
	return nil
}

// SessionStatus reports whether a browser session is live and what page it is
// on — what an agent calls to decide whether it needs to open or navigate.
func SessionStatus(w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}
	name := os.Getenv("PLAYWRIGHT_CLI_SESSION")
	if name == "" {
		name = "(default)"
	}
	out, err := runPlaywrightCLIOutput("eval", "() => location.href")
	if err != nil {
		fmt.Fprintf(w, "Session %s: not open\n", name)
		return nil
	}
	fmt.Fprintf(w, "Session %s: live\n", name)
	if page := parseEvalResult(out); page != "" {
		fmt.Fprintf(w, "  Page: %s\n", page)
	}
	return nil
}

// CloseSession tears down the current browser session, or every session when
// all is true.
func CloseSession(all bool, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}
	arg := "close"
	if all {
		arg = "close-all"
	}
	if err := runPlaywrightCLI(arg); err != nil {
		return fmt.Errorf("closing session: %w", err)
	}
	if all {
		fmt.Fprintln(w, "Closed all browser sessions.")
	} else {
		fmt.Fprintln(w, "Closed browser session.")
	}
	return nil
}

// parseEvalResult extracts the value a `playwright-cli eval` call printed. The
// CLI prints the return value under a "### Result" heading; fall back to the
// last non-empty line if that heading isn't present, and strip surrounding
// quotes. Kept tolerant of formatting so status display degrades gracefully.
func parseEvalResult(out string) string {
	body := out
	if _, after, found := strings.Cut(out, "### Result"); found {
		body = after
	}
	line := firstNonEmptyLine(body)
	return strings.Trim(line, "\"'")
}

// firstNonEmptyLine returns the first non-empty, trimmed line of s.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}
