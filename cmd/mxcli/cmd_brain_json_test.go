// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/cmd/mxcli/brain"
)

// The types marshalling correctly is only half of it. The other half is that
// nothing else reaches stdout — one stray Println (a "Nothing staged.", a
// progress line, the PoC banner) and the output stops being JSON while still
// looking fine to a person. So this runs the commands as a caller would and
// parses what comes back.
func TestBrainCommandsEmitParseableJSONOnStdout(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	store := brain.NewStore(dir)
	if _, err := store.Init(); err != nil {
		t.Fatal(err)
	}
	queue := brain.NewQueue(dir)
	decision, err := brain.NewEntry("Orders are committed by Finance, not Sales", []string{"@Sales.Order"}, brainTestDay())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := queue.Append(decision); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		args []string
		want []string // top-level keys the contract promises
	}{
		{"staged", []string{"brain", "staged", "-p", mpr, "--json"}, []string{"staged", "count"}},
		{"show", []string{"brain", "show", "-p", mpr, "--json"}, []string{"shards"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runBrainForTest(t, tc.args)

			var got map[string]any
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("`mxcli %v` did not emit JSON on stdout: %v\ngot:\n%s", tc.args, err, out)
			}
			for _, k := range tc.want {
				if _, ok := got[k]; !ok {
					t.Errorf("no %q key in the output of `mxcli %v`: %s", k, tc.args, out)
				}
			}
		})
	}

	// Control: without --json the same commands print for a person, and that
	// output is deliberately NOT JSON. Without this, a change that made every
	// command emit JSON unconditionally would pass the assertions above.
	out := runBrainForTest(t, []string{"brain", "staged", "-p", mpr})
	var any0 map[string]any
	if json.Unmarshal([]byte(out), &any0) == nil {
		t.Errorf("`brain staged` without --json emitted JSON; the human output was replaced rather than added to:\n%s", out)
	}
}

// An empty queue must still be an object with a count, not the words "Nothing
// staged." A dispatcher's whole question is whether a slice staged anything,
// and it has to be able to ask that when the answer is no — which is the case
// where a human-shaped message would break the parse.
func TestBrainStagedJSONIsStillJSONWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	mpr := filepath.Join(dir, "App.mpr")
	if err := os.WriteFile(mpr, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	out := runBrainForTest(t, []string{"brain", "staged", "-p", mpr, "--json"})

	var got struct {
		Count  int   `json:"count"`
		Staged []any `json:"staged"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("an empty queue did not emit JSON: %v\ngot:\n%s", err, out)
	}
	if got.Count != 0 || len(got.Staged) != 0 {
		t.Errorf("empty queue reported %d staged: %s", got.Count, out)
	}
}

// runBrainForTest executes a command through the real root, so the persistent
// --json flag and PersistentPreRun are exercised exactly as a caller gets them.
//
// rootCmd is a package global and cobra keeps flag values between runs, so
// --json is reset first: without that, every run after a --json one inherits it
// and the control below (human output is NOT JSON) passes for the wrong reason.
func runBrainForTest(t *testing.T, args []string) string {
	t.Helper()
	if err := rootCmd.PersistentFlags().Set("json", "false"); err != nil {
		t.Fatal(err)
	}
	globalJSONFlag = false

	out, err := captureStdout(t, func() error {
		rootCmd.SetArgs(args)
		return rootCmd.Execute()
	})
	if err != nil {
		t.Fatalf("`mxcli %v` failed: %v\n%s", args, err, out)
	}
	return out
}

func brainTestDay() time.Time { return time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC) }
