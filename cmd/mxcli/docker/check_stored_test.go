// SPDX-License-Identifier: Apache-2.0

// ako/mxcli#568 and #646: `docker check` ran `mx update-widgets` on its copy
// before `mx check`, so it validated a widget-normalised model, never the
// stored one. A CE0463 the stored project had was cleared on the copy and the
// check reported 0 errors, while `mxcli run --local` built the project as stored
// and failed:
//
//	| command                                   | result            |
//	| mxcli docker check                        | 0 errors          |
//	| mxcli docker check --no-update-widgets    | 3 errors (CE0463) |
//	| mxcli run --local                         | build fails, same 3 |
//
// docker check now checks the stored project by default. When that finds
// CE0463, it re-checks the same temporary copy after update-widgets and labels
// each CE0463 as a stale widget definition (`mxcli fix widgets` clears it) or
// as one that persists. The old behaviour is --update-widgets.
package docker

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ce0463Line is how mx check prints a CE0463 (measured on 11.13.0). The text
// does not name the page, so two widgets of the same name on different pages
// print the same line.
func ce0463Line(element string) string {
	return `[error] [CE0463] "The definition of this widget has changed. Update this widget by right-clicking it ` +
		`and selecting 'Update widget', or select 'Update all widgets' to update all widgets in the app." at ` + element
}

// stubStaleWidgets makes the stubbed mx behave like a project whose stored
// widgets need a resync: mx check reports every line in stale and persist until
// update-widgets has run on the copy it is given, and only persist afterwards,
// plus other errors either way. It records each invocation.
func stubStaleWidgets(t *testing.T, stale, persist, other []string) (calls *[]string) {
	t.Helper()
	var seen []string
	origResolve, origUW, origCheck := resolveMxForCheck, updateWidgetsCmd, mxCheckCmd
	t.Cleanup(func() { resolveMxForCheck, updateWidgetsCmd, mxCheckCmd = origResolve, origUW, origCheck })

	marker := func(mprPath string) string { return filepath.Join(filepath.Dir(mprPath), "widgets-updated") }
	resolveMxForCheck = func(string, string) (string, error) { return "mx", nil }
	updateWidgetsCmd = func(_, mprPath string, w, _ io.Writer) error {
		seen = append(seen, "update-widgets")
		fmt.Fprintln(w, "Updating widgets...")
		return os.WriteFile(marker(mprPath), nil, 0o644)
	}
	mxCheckCmd = func(_, mprPath string, _ []string, w, _ io.Writer) error {
		seen = append(seen, "check")
		fmt.Fprintln(w, "Checking app for errors...")
		var lines []string
		if _, err := os.Stat(marker(mprPath)); err != nil {
			for _, e := range stale {
				lines = append(lines, ce0463Line(e))
			}
		}
		for _, e := range persist {
			lines = append(lines, ce0463Line(e))
		}
		lines = append(lines, other...)
		for _, l := range lines {
			fmt.Fprintln(w, l)
		}
		fmt.Fprintf(w, "The app contains: %d errors.\n", len(lines))
		if len(lines) > 0 {
			return fmt.Errorf("exit status 1")
		}
		return nil
	}
	return &seen
}

// The reported symptom: a project whose stored widgets carry CE0463 that a
// resync clears. The default must fail on it, as MxBuild and `run --local` do.
func TestCheck_ChecksTheStoredProjectByDefault(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t,
		[]string{"Tree node 'tnCustomer'", "Tree node 'tnPart'", "Data grid 2 'dgRequests'"}, nil, nil)
	t.Setenv("TMPDIR", t.TempDir())

	var out bytes.Buffer
	err := Check(CheckOptions{ProjectPath: mprPath, Stdout: &out, Stderr: io.Discard})
	got := out.String()
	if err == nil {
		t.Fatalf("docker check passed a project whose stored model has 3 CE0463 errors "+
			"(the #568 false pass: it checked a widget-normalised copy):\n%s", got)
	}
	if !strings.Contains(got, "The app contains: 3 errors.") {
		t.Errorf("the stored project's own check result is not reported:\n%s", got)
	}
	if len(*calls) == 0 || (*calls)[0] != "check" {
		t.Errorf("the first mx step must be a check of the project as stored, got %v", *calls)
	}
	for _, el := range []string{"Tree node 'tnCustomer'", "Tree node 'tnPart'", "Data grid 2 'dgRequests'"} {
		want := "at " + el + ": " + staleCE0463Label
		if !strings.Contains(got, want) {
			t.Errorf("CE0463 at %s not labelled %q:\n%s", el, staleCE0463Label, got)
		}
	}
	if !strings.Contains(got, "mxcli fix widgets -p "+mprPath) {
		t.Errorf("output does not name the command that clears them for this project:\n%s", got)
	}
	if !strings.Contains(got, "After update-widgets: 0 errors.") {
		t.Errorf("output does not say what the check reports once the widgets are resynced:\n%s", got)
	}
}

// A CE0463 a resync does not clear is not a stale definition, and must not be
// labelled as one. Same-named widgets on different pages print identical lines,
// so the labels are counted per line.
func TestCheck_LabelsCE0463ThatPersistsAfterUpdateWidgets(t *testing.T) {
	mprPath := v1Fixture(t)
	stubStaleWidgets(t,
		[]string{"Data grid 2 'dataGrid2_1'", "Data grid 2 'dataGrid2_1'", "Combo box 'cb'"},
		[]string{"Data grid 2 'dataGrid2_1'", "Image 'img'"},
		[]string{`[error] [CE0117] "Error(s) in expression." at Microflow 'ACT_X'`})
	t.Setenv("TMPDIR", t.TempDir())

	var out bytes.Buffer
	if err := Check(CheckOptions{ProjectPath: mprPath, Stdout: &out, Stderr: io.Discard}); err == nil {
		t.Fatalf("Check passed a project with errors:\n%s", out.String())
	}
	got := out.String()
	for _, want := range []string{
		"at Data grid 2 'dataGrid2_1' (2 of 3): " + staleCE0463Label,
		"at Data grid 2 'dataGrid2_1' (1 of 3): " + persistingCE0463Label,
		"at Combo box 'cb': " + staleCE0463Label,
		"at Image 'img': " + persistingCE0463Label,
		"3 of 5 are stale widget definitions; 2 persist after update-widgets",
		"    mxcli fix widgets -p " + mprPath,
		"As stored: 6 errors. After update-widgets: 3 errors.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "at ACT_X") || strings.Contains(got, "Microflow 'ACT_X': ") {
		t.Errorf("a non-CE0463 error was labelled:\n%s", got)
	}
}

// The re-check costs a second mx check, so it runs only when there is a CE0463
// to explain: a clean project, or one whose errors are all something else, is
// checked once and update-widgets never runs.
func TestCheck_NoCE0463NoSecondCheck(t *testing.T) {
	for _, tc := range []struct {
		name    string
		other   []string
		wantErr bool
	}{
		{"clean", nil, false},
		{"other errors only", []string{`[error] [CE0117] "Error(s) in expression." at Microflow 'ACT_X'`}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mprPath := v1Fixture(t)
			calls := stubStaleWidgets(t, nil, nil, tc.other)
			t.Setenv("TMPDIR", t.TempDir())
			var out bytes.Buffer
			err := Check(CheckOptions{ProjectPath: mprPath, Stdout: &out, Stderr: io.Discard})
			if (err != nil) != tc.wantErr {
				t.Fatalf("Check error = %v, want error %v\n%s", err, tc.wantErr, out.String())
			}
			if strings.Join(*calls, ",") != "check" {
				t.Errorf("mx steps = %v, want one check and no update-widgets", *calls)
			}
			if strings.Contains(out.String(), "update-widgets") {
				t.Errorf("output talks about update-widgets although nothing needed it:\n%s", out.String())
			}
		})
	}
}

// --update-widgets is the old behaviour, kept for whoever wants it: normalise
// the copy, then check only that. On the #568 project it reports 0 errors and
// passes — with a note saying what that hides.
func TestCheck_UpdateWidgetsFlagChecksTheNormalisedCopy(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t, []string{"Tree node 'tnCustomer'"}, nil, nil)
	t.Setenv("TMPDIR", t.TempDir())

	var out bytes.Buffer
	if err := Check(CheckOptions{ProjectPath: mprPath, UpdateWidgetsFirst: true, Stdout: &out, Stderr: io.Discard}); err != nil {
		t.Fatalf("--update-widgets: %v\n%s", err, out.String())
	}
	if strings.Join(*calls, ",") != "update-widgets,check" {
		t.Errorf("mx steps = %v, want update-widgets then one check", *calls)
	}
	got := out.String()
	if !strings.Contains(got, "The app contains: 0 errors.") {
		t.Errorf("--update-widgets did not check the normalised copy:\n%s", got)
	}
	if !strings.Contains(got, "normalised on a temporary copy") {
		t.Errorf("output does not say the check ran on a normalised copy:\n%s", got)
	}
}

// --no-update-widgets keeps working: it checks the project as stored, as the
// default now does, and never runs update-widgets — not even to label.
func TestCheck_NoUpdateWidgetsChecksStoredWithoutRecheck(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t, []string{"Tree node 'tnCustomer'"}, nil, nil)
	t.Setenv("TMPDIR", t.TempDir())

	var out bytes.Buffer
	if err := Check(CheckOptions{ProjectPath: mprPath, SkipUpdateWidgets: true, Stdout: &out, Stderr: io.Discard}); err == nil {
		t.Fatalf("--no-update-widgets passed a project with a stored CE0463:\n%s", out.String())
	}
	if strings.Join(*calls, ",") != "check" {
		t.Errorf("mx steps = %v, want one check and no update-widgets", *calls)
	}
}

func TestCheck_UpdateWidgetsFlagsAreExclusive(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t, nil, nil, nil)
	err := Check(CheckOptions{ProjectPath: mprPath, SkipUpdateWidgets: true, UpdateWidgetsFirst: true, Stdout: io.Discard, Stderr: io.Discard})
	if err == nil {
		t.Fatal("Check accepted both --no-update-widgets and --update-widgets")
	}
	if len(*calls) != 0 {
		t.Errorf("mx ran although the options contradict each other: %v", *calls)
	}
}

// mendixlabs/mxcli#1629 in the re-check: update-widgets refuses a project of
// another patch version. The verdict is still the stored project's, and the
// output says the CE0463s could not be told apart rather than labelling them.
func TestCheck_FailedRecheckLeavesCE0463Unlabelled(t *testing.T) {
	mprPath := v1Fixture(t)
	calls := stubStaleWidgets(t, []string{"Tree node 'tnCustomer'"}, nil, nil)
	updateWidgetsCmd = func(_, _ string, w, _ io.Writer) error {
		*calls = append(*calls, "update-widgets")
		fmt.Fprintln(w, "Project is in different Mendix version - 10.24.13.86719")
		return fmt.Errorf("exit status 4")
	}
	t.Setenv("TMPDIR", t.TempDir())

	var out bytes.Buffer
	if err := Check(CheckOptions{ProjectPath: mprPath, Stdout: &out, Stderr: io.Discard}); err == nil {
		t.Fatalf("Check passed a project with a stored CE0463:\n%s", out.String())
	}
	got := out.String()
	if strings.Contains(got, staleCE0463Label) || strings.Contains(got, persistingCE0463Label) {
		t.Errorf("CE0463 labelled although update-widgets failed:\n%s", got)
	}
	for _, want := range []string{"Project is in different Mendix version", "could not tell"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if strings.Join(*calls, ",") != "check,update-widgets" {
		t.Errorf("mx steps = %v, want the check, then the failed update-widgets and no re-check", *calls)
	}
}
