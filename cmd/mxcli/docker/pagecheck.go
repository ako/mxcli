// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// pagecheck.go answers, in text, the question a screenshot is usually taken to
// answer: did this page render, and is anything wrong with it.
//
// A PNG costs roughly 1,500 tokens to read and a verdict about 100 — and the
// PNG's cost is not a one-off, because an image read into a conversation is
// re-charged on every later model call (ako/mxcli#614, and
// docs/11-proposals/PROPOSAL_agent_loop_efficiency.md). The picture is also
// strictly *less* informative for this question: a console error, which is the
// usual cause of a page that renders blank, does not appear in one.
//
// It uses the Playwright already required by --screenshot, driven through node
// rather than the CLI, because the Playwright CLI has no text-dump subcommand.

// PageSignals is what one page load tells us.
type PageSignals struct {
	Title         string   `json:"title"`
	Headings      []string `json:"headings"`
	Alerts        []string `json:"alerts"`        // visible Mendix error/warning banners
	TextLen       int      `json:"textLen"`       // body innerText length; 0 is the blank-page tell
	Rows          int      `json:"rows"`          // grid / list rows rendered
	ConsoleErrors []string `json:"consoleErrors"` // what a screenshot cannot show
}

// pageProbeJS runs in node with the global Playwright. It reports signals and
// never throws for a bad page — a page that fails to render is the thing being
// measured, not an error in measuring it.
const pageProbeJS = `
const { chromium } = require('playwright');
(async () => {
  const [url, storage, waitMs] = [process.argv[2], process.argv[3], parseInt(process.argv[4] || '4000', 10)];
  const b = await chromium.launch();
  const ctx = await b.newContext(storage ? { storageState: storage } : {});
  const p = await ctx.newPage();
  const consoleErrors = [];
  p.on('console', m => { if (m.type() === 'error') consoleErrors.push(m.text()); });
  p.on('pageerror', e => consoleErrors.push(String(e && e.message || e)));
  const out = { title: '', headings: [], alerts: [], textLen: 0, rows: 0, consoleErrors };
  try {
    await p.goto(url, { waitUntil: 'domcontentloaded' });
    await p.waitForTimeout(waitMs);
    out.title = await p.title();
    out.headings = await p.$$eval('h1,h2', ns => ns.map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
    out.alerts = await p.$$eval('.alert-danger,.alert-warning,.mx-validation-message',
      ns => ns.map(n => (n.innerText||'').trim()).filter(Boolean).slice(0, 3));
    out.textLen = (await p.$eval('body', n => n.innerText || '')).trim().length;
    out.rows = await p.$$eval('.mx-datagrid tbody tr, .mx-listview > ul > li, [role="row"]', ns => ns.length);
  } catch (e) {
    out.consoleErrors.push('probe: ' + String(e && e.message || e));
  }
  console.log(JSON.stringify(out));
  await b.close();
})();
`

// CheckPage loads url and returns its signals. storage is an optional Playwright
// storage-state file, for pages behind a login.
func CheckPage(url, storage string, waitMs int, timeout time.Duration) (PageSignals, error) {
	var sig PageSignals

	node, err := exec.LookPath("node")
	if err != nil {
		return sig, fmt.Errorf("node not found; the page check needs the same Node that Playwright uses")
	}
	f, err := os.CreateTemp("", "mxcli-pagecheck-*.js")
	if err != nil {
		return sig, err
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(pageProbeJS); err != nil {
		f.Close()
		return sig, err
	}
	f.Close()

	if waitMs == 0 {
		waitMs = 4000
	}
	if timeout == 0 {
		timeout = 90 * time.Second
	}
	cmd := exec.Command(node, f.Name(), url, storage, fmt.Sprint(waitMs))
	cmd.Env = append(os.Environ(), "NODE_PATH="+globalNodeModules())
	out := &syncBuffer{}
	cmd.Stdout, cmd.Stderr = out, out

	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return sig, fmt.Errorf("launching the page check: %w", err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return sig, fmt.Errorf("page check failed: %w\n%s", err, out.String())
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		<-done
		return sig, fmt.Errorf("page check timed out after %s", timeout)
	}

	// The probe prints one JSON line; Playwright may print noise before it.
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	last := lines[len(lines)-1]
	if err := json.Unmarshal([]byte(last), &sig); err != nil {
		return sig, fmt.Errorf("page check produced no verdict: %w\n%s", err, out.String())
	}
	return sig, nil
}

// globalNodeModules resolves the global node_modules so `require('playwright')`
// works from a temp file. `npm root -g` is authoritative; the common install
// path is the fallback when npm is absent.
func globalNodeModules() string {
	if npm, err := exec.LookPath("npm"); err == nil {
		if out, err := exec.Command(npm, "root", "-g").Output(); err == nil {
			if p := strings.TrimSpace(string(out)); p != "" {
				return p
			}
		}
	}
	return "/usr/lib/node_modules"
}

// formatPageVerdict renders signals as the line that replaces the screenshot.
//
// Brevity is the point, so everything unbounded is clamped: the verdict must
// stay far cheaper than the image even on a page with hundreds of rows and a
// wall of console noise, or there is no reason to prefer it.
func formatPageVerdict(label string, s PageSignals) string {
	var b strings.Builder
	fmt.Fprintf(&b, "page %s", label)
	if s.Title != "" {
		fmt.Fprintf(&b, "  title=%q", clip(s.Title, 60))
	}
	if len(s.Headings) > 0 {
		fmt.Fprintf(&b, "  h=%q", clip(s.Headings[0], 60))
	}
	if s.Rows > 0 {
		fmt.Fprintf(&b, "  rows=%d", s.Rows)
	}
	if s.TextLen == 0 {
		b.WriteString("  NO VISIBLE TEXT")
	} else {
		fmt.Fprintf(&b, "  text=%d", s.TextLen)
	}
	fmt.Fprintf(&b, "  console-errors=%d\n", len(s.ConsoleErrors))

	// Detail lines, capped. These are what make the verdict actionable rather
	// than merely cheap, and the console error is the one a picture cannot give.
	for _, a := range firstN(s.Alerts, 2) {
		fmt.Fprintf(&b, "  ALERT  %s\n", clip(a, 140))
	}
	for _, e := range firstN(s.ConsoleErrors, 2) {
		fmt.Fprintf(&b, "  ERR    %s\n", clip(e, 140))
	}
	return b.String()
}

func firstN(ss []string, n int) []string {
	if len(ss) > n {
		return ss[:n]
	}
	return ss
}

func clip(s string, n int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
