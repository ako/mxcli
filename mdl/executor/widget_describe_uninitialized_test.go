// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectWithInstalledFieldset is a fresh clone: Fieldset's .mpk is in widgets/
// and .mxcli/ — gitignored — does not exist.
func projectWithInstalledFieldset(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "widgets"), 0o755); err != nil {
		t.Fatal(err)
	}
	const mpk = "com.mendix.widget.web.Fieldset.mpk"
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "expr-checker", "widgets", mpk))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "widgets", mpk), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "App.mpr")
}

// ako/mxcli#663. On a project that never ran `mxcli widget init`:
//
//	$ mxcli -p App.mpr -c "DESCRIBE WIDGET fieldset"
//	Error: unknown widget "fieldset" — use an MDL keyword (barcodescanner, …
//
// while page authoring accepted `fieldset` and DESCRIBE PAGE emitted it. The
// page builder and the validator both generate the project's definitions from
// its installed .mpk before reading them; DescribeWidget read only whatever was
// already in .mxcli/widgets/, so it knew the nine embedded widgets.
func TestDescribeWidget_InstalledWidgetNeedsNoWidgetInit(t *testing.T) {
	desc, err := DescribeWidget("fieldset", projectWithInstalledFieldset(t))
	if err != nil {
		t.Fatalf("DESCRIBE WIDGET fieldset on an installed-but-uninitialised project: %v", err)
	}
	if desc.WidgetID != "com.mendix.widget.web.fieldset.Fieldset" {
		t.Errorf("WidgetID = %q, want com.mendix.widget.web.fieldset.Fieldset", desc.WidgetID)
	}
	if desc.MDLName != "fieldset" {
		t.Errorf("MDLName = %q, want fieldset", desc.MDLName)
	}
}

// Control: in the same project a name that is genuinely not installed must still
// be an error — and the error must not send the reader down a path that cannot
// succeed. The old message suggested an unquoted widget id, which is a parse
// error in DESCRIBE WIDGET; the quoted form is the one the grammar accepts.
func TestDescribeWidget_UnknownWidgetErrorNamesAWorkingForm(t *testing.T) {
	_, err := DescribeWidget("fieldsett", projectWithInstalledFieldset(t))
	if err == nil {
		t.Fatal("want an error for a widget that is not installed, got none")
	}
	msg := err.Error()
	if !strings.Contains(msg, "fieldset") || !strings.Contains(msg, "'com.mendix.widget.web.") {
		t.Errorf("error should list the installed keyword and a quoted widget id, got: %s", msg)
	}
}

// Without a project only the embedded widgets are known, and the error should
// say that a project is what brings the rest in.
func TestDescribeWidget_UnknownWidgetWithoutProjectPointsAtTheProject(t *testing.T) {
	_, err := DescribeWidget("fieldset", "")
	if err == nil {
		t.Fatal("want an error with no project, got none")
	}
	if !strings.Contains(err.Error(), "-p") {
		t.Errorf("error should say a project (-p) is needed for installed widgets, got: %s", err)
	}
}
