// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"strings"
	"testing"
)

// A screenshot answers a textual question with an image. The PNG on disk is
// free; reading it into context is the whole cost, and from then on it is
// re-charged on every later model call (ako/mxcli#614).
//
// These cover the formatting, which is where the value is: the verdict has to
// be short enough to beat the picture and specific enough to replace it.

func TestVerdictIsOneLineWhenThePageIsHealthy(t *testing.T) {
	got := formatPageVerdict("/p/customers", PageSignals{
		Title: "Customers", Headings: []string{"Customer overview"},
		TextLen: 812, Rows: 12,
	})
	if n := strings.Count(strings.TrimRight(got, "\n"), "\n") + 1; n != 1 {
		t.Errorf("healthy page rendered %d lines, want 1:\n%s", n, got)
	}
	for _, want := range []string{"/p/customers", "Customers", "rows=12"} {
		if !strings.Contains(got, want) {
			t.Errorf("verdict omits %q:\n%s", want, got)
		}
	}
}

// The blank-page tell. This is the symptom that cost ~40 calls in the session
// the proposal came from, and no screenshot shows WHY — the console error does.
func TestBlankPageIsCalledOutWithItsConsoleError(t *testing.T) {
	got := formatPageVerdict("/p/customers", PageSignals{
		Title:   "Customers",
		TextLen: 0,
		ConsoleErrors: []string{
			"TypeError: Cannot read properties of undefined (reading 'items')",
		},
	})
	if !strings.Contains(got, "NO VISIBLE TEXT") {
		t.Errorf("a page with no body text is not flagged:\n%s", got)
	}
	if !strings.Contains(got, "Cannot read properties of undefined") {
		t.Errorf("the console error is not surfaced; it is the thing a screenshot cannot show:\n%s", got)
	}
}

// An error banner is visible in a screenshot, so the verdict must not be worse
// than the picture it replaces.
func TestVisibleErrorBannersAreReported(t *testing.T) {
	got := formatPageVerdict("/", PageSignals{
		Title: "App", TextLen: 200,
		Alerts: []string{"An error occurred, please contact your system administrator"},
	})
	if !strings.Contains(got, "contact your system administrator") {
		t.Errorf("a visible alert banner is not reported:\n%s", got)
	}
}

// THE CONTROL on brevity. A verdict that grows with the page defeats its own
// purpose — it has to stay cheap on a page with a hundred rows and a long body.
func TestVerdictStaysShortOnALargePage(t *testing.T) {
	long := make([]string, 40)
	for i := range long {
		long[i] = "a heading that is quite long and would bloat the verdict badly"
	}
	got := formatPageVerdict("/p/big", PageSignals{
		Title: "Big", Headings: long, TextLen: 400000, Rows: 5000,
		ConsoleErrors: long,
	})
	if len(got) > 600 {
		t.Errorf("verdict is %d bytes on a large page; it must stay far cheaper than a "+
			"screenshot (~1,500 tokens) or there is no point:\n%s", len(got), got)
	}
}
