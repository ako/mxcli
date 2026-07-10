// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"path/filepath"
	"testing"
)

// runesToStrings flattens completion candidates for readable assertions.
func runesToStrings(rs [][]rune) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestCompleteScriptPath drives the EXECUTE SCRIPT path completer over a small
// on-disk tree, checked from the temp dir so relative paths resolve.
func TestCompleteScriptPath(t *testing.T) {
	tmp := t.TempDir()
	// mdl-examples/ (dir), mdl-notes.txt (file), other.mdl (file), .hidden (file)
	if err := os.MkdirAll(filepath.Join(tmp, "mdl-examples", "bug-tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"mdl-notes.txt", "other.mdl", ".hidden"} {
		if err := os.WriteFile(filepath.Join(tmp, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "mdl-examples", "a.mdl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	c := &mdlCompleter{}

	tests := []struct {
		name    string
		line    string
		wantOK  bool
		want    []string
		wantOff int
	}{
		{
			name:    "single dir match completes with trailing slash",
			line:    `execute script "mdl-e`,
			wantOK:  true,
			want:    []string{"xamples/"},
			wantOff: len("mdl-e"),
		},
		{
			name:   "prefix shared by dir and file offers both",
			line:   `execute script 'mdl-`,
			wantOK: true,
			// alphabetical: mdl-examples/ then mdl-notes.txt
			want:    []string{"examples/", "notes.txt"},
			wantOff: len("mdl-"),
		},
		{
			name:    "descend into a directory",
			line:    `EXECUTE SCRIPT "mdl-examples/`,
			wantOK:  true,
			want:    []string{"a.mdl", "bug-tests/"},
			wantOff: 0,
		},
		{
			name:    "partial basename inside a directory",
			line:    `execute script "mdl-examples/bug`,
			wantOK:  true,
			want:    []string{"-tests/"},
			wantOff: len("bug"),
		},
		{
			name:   "no opening quote still completes",
			line:   `execute script other`,
			wantOK: true,
			want:   []string{".mdl"},
		},
		{
			name:   "hidden files skipped unless dot typed",
			line:   `execute script "`,
			wantOK: true,
			// .hidden excluded; alphabetical of the visible entries
			want: []string{"mdl-examples/", "mdl-notes.txt", "other.mdl"},
		},
		{
			name:   "dot fragment reveals hidden files",
			line:   `execute script ".h`,
			wantOK: true,
			want:   []string{"idden"},
		},
		{
			name:   "closed quote stops completion",
			line:   `execute script "other.mdl"`,
			wantOK: false,
		},
		{
			name:   "unrelated command is not a script path",
			line:   `show entities`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, off, ok := c.completeScriptPath(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if gs := runesToStrings(got); !equalStrings(gs, tt.want) {
				t.Errorf("completions = %q, want %q", gs, tt.want)
			}
			if tt.wantOff != 0 && off != tt.wantOff {
				t.Errorf("offset = %d, want %d", off, tt.wantOff)
			}
		})
	}
}
