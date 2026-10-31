// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ako/mxcli#1832: where a flow's layout style comes from.

func writeLayoutConfig(t *testing.T, dir, body string) string {
	t.Helper()
	p := filepath.Join(dir, flowLayoutConfigName)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFlowLayoutConfig(t *testing.T) {
	for _, c := range []struct {
		name, body string
		want       flowLayout
		wantErr    string
	}{
		{"empty file is the default", "", flowLayout{}, ""},
		{"preset", "flows:\n  preset: true-right-false-down\n", flowLayout{IfElse: IfElseColumn, CaseSplit: CaseSplitColumns}, ""},
		{"option", "flows:\n  if-else: true-right-else-column\n", flowLayout{IfElse: IfElseColumn}, ""},
		{"option overrides preset", "flows:\n  preset: true-right-false-down\n  if-else: lanes\n", flowLayout{CaseSplit: CaseSplitColumns}, ""},
		{"unknown preset", "flows:\n  preset: zigzag\n", flowLayout{}, `unknown layout preset "zigzag"`},
		{"unknown option", "flows:\n  if-els: lanes\n", flowLayout{}, `unknown layout option "if-els"`},
		{"unknown value", "flows:\n  if-else: column\n", flowLayout{}, `unknown value "column"`},
		{"unknown top-level key", "flow:\n  preset: happy-line\n", flowLayout{}, "field flow not found"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, err := LoadFlowLayoutConfig(writeLayoutConfig(t, t.TempDir(), c.body))
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("err = %v, want it to mention %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if *got != c.want {
				t.Fatalf("got %+v, want %+v", *got, c.want)
			}
		})
	}
}

func TestFindFlowLayoutConfig_PrefersClaudeDir(t *testing.T) {
	dir := t.TempDir()
	if got := FindFlowLayoutConfig(dir); got != "" {
		t.Fatalf("no file: got %q", got)
	}
	root := writeLayoutConfig(t, dir, "")
	if got := FindFlowLayoutConfig(dir); got != root {
		t.Fatalf("got %q, want %q", got, root)
	}
	if err := os.Mkdir(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	claude := writeLayoutConfig(t, filepath.Join(dir, ".claude"), "")
	if got := FindFlowLayoutConfig(dir); got != claude {
		t.Fatalf("got %q, want %q", got, claude)
	}
}

func TestFlowLayoutMarker(t *testing.T) {
	column := flowLayout{IfElse: IfElseColumn}
	const line = "@mxcli-layout: if-else=true-right-else-column"

	for _, c := range []struct {
		name   string
		doc    string
		layout flowLayout
		want   string
	}{
		{"default leaves undocumented flow alone", "", flowLayout{}, ""},
		{"default leaves documentation byte for byte", "Submits.\r\n  trailing  ", flowLayout{}, "Submits.\r\n  trailing  "},
		{"style on an undocumented flow", "", column, line},
		{"style appended", "Submits the order.", column, "Submits the order.\n" + line},
		{"style replaced, not duplicated", "Submits.\n" + line, column, "Submits.\n" + line},
		{"older marker replaced", "Submits.\n@mxcli-layout: if-else=lanes", column, "Submits.\n" + line},
		{"default removes the marker", "Submits.\n" + line, flowLayout{}, "Submits."},
		{"default removes a lone marker", line, flowLayout{}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := withFlowLayoutMarker(c.doc, c.layout)
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			// What is written reads back as the style it records.
			back, found, err := flowLayoutFromDocumentation(got)
			if err != nil || back != c.layout || found != (c.layout != flowLayout{}) {
				t.Fatalf("read back %+v found=%v err=%v, want %+v", back, found, err, c.layout)
			}
			// Writing it again changes nothing (ADR-0008).
			if again := withFlowLayoutMarker(got, c.layout); again != got {
				t.Fatalf("second write %q differs from first %q", again, got)
			}
		})
	}
}

func TestResolveFlowLayout(t *testing.T) {
	dir := t.TempDir()
	writeLayoutConfig(t, dir, "flows:\n  preset: true-right-false-down\n")
	ctx := &ExecContext{MprPath: filepath.Join(dir, "app.mpr"), Output: &strings.Builder{}}

	// Unmarked flow: the project's style, and the marker is written.
	l, doc, err := resolveFlowLayout(ctx, "Submits.", true)
	if err != nil || l.IfElse != IfElseColumn || !strings.HasSuffix(doc, "if-else=true-right-else-column") {
		t.Fatalf("unmarked: %+v %q %v", l, doc, err)
	}

	// A flow recording the default (no line can say so) is unmarked; one recording
	// a style keeps it whatever the project says.
	ctx2 := &ExecContext{MprPath: filepath.Join(t.TempDir(), "app.mpr"), Output: &strings.Builder{}}
	l, doc, err = resolveFlowLayout(ctx2, "Submits.\n@mxcli-layout: if-else=true-right-else-column", true)
	if err != nil || l.IfElse != IfElseColumn || !strings.Contains(doc, flowLayoutMarker) {
		t.Fatalf("recorded style in a default project: %+v %q %v", l, doc, err)
	}

	// layout flows ignores the record and re-styles.
	l, doc, err = resolveFlowLayout(ctx2, "Submits.\n@mxcli-layout: if-else=true-right-else-column", false)
	if err != nil || l.IfElse != IfElseLanes || doc != "Submits." {
		t.Fatalf("re-style to default: %+v %q %v", l, doc, err)
	}

	// An unreadable marker is warned about and treated as absent — never an error.
	out := &strings.Builder{}
	ctx3 := &ExecContext{MprPath: filepath.Join(t.TempDir(), "app.mpr"), Output: out}
	l, doc, err = resolveFlowLayout(ctx3, "Submits.\n@mxcli-layout: zigzag=yes", true)
	if err != nil || l.IfElse != IfElseLanes || doc != "Submits." || !strings.Contains(out.String(), "Warning") {
		t.Fatalf("bad marker: %+v %q %v, output %q", l, doc, err, out.String())
	}

	// A broken project config is an error, not a silent default.
	bad := t.TempDir()
	writeLayoutConfig(t, bad, "flows:\n  preset: zigzag\n")
	if _, _, err := resolveFlowLayout(&ExecContext{MprPath: filepath.Join(bad, "app.mpr")}, "", true); err == nil {
		t.Fatal("broken config: no error")
	}
}
