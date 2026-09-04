// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A widget whose package is NOT in widgets/ must not be told to run
// `widget init`. That command scans widgets/, so for a widget Studio Pro
// bundles rather than installs it can never help — which is what cost the
// reporter of mendixlabs/mxcli#1036 a debugging session.
func TestMissingWidgetMessage_UninstalledWidgetDoesNotRecommendWidgetInit(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := missingWidgetMessage(dir, "com.mendix.widget.web.fileuploader.FileUploader")

	if strings.Contains(got, "run 'mxcli widget init") {
		t.Errorf("recommends a command that cannot work:\n%s", got)
	}
	if !strings.Contains(got, "cannot help") {
		t.Errorf("does not say why widget init is not the remedy:\n%s", got)
	}
	if !strings.Contains(got, "com.mendix.widget.web.fileuploader.FileUploader") {
		t.Errorf("does not name the widget:\n%s", got)
	}
}

// The control: when the package IS installed, `widget init` is exactly the
// right remedy and must still be named. Without this, the test above passes
// against a build that simply deleted the recommendation.
func TestMissingWidgetMessage_InstalledWidgetStillRecommendsWidgetInit(t *testing.T) {
	// FindMPK PARSES each .mpk rather than matching on its name, so this needs
	// a real package — the fixture project ships Accordion among 33 others.
	// (An empty file named after the widget is silently skipped, which is what
	// the first version of this control got wrong.)
	dir := filepath.Join("..", "..", "testdata", "expr-checker")
	if _, err := os.Stat(filepath.Join(dir, "widgets")); err != nil {
		t.Skipf("fixture widgets/ not available: %v", err)
	}

	got := missingWidgetMessage(dir, "com.mendix.widget.web.accordion.Accordion")

	if !strings.Contains(got, "mxcli widget init") {
		t.Errorf("installed package should still point at widget init:\n%s", got)
	}
	if !strings.Contains(got, "not extracted yet") {
		t.Errorf("does not say the package is present but unextracted:\n%s", got)
	}
}
