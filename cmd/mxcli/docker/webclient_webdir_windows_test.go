// SPDX-License-Identifier: Apache-2.0

//go:build windows

package docker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The mechanism behind #1342, on a real process: a bundler whose working
// directory is deployment/web keeps Windows from removing web/, with the same
// "being used by another process" the serve build reported, and Stop releases
// it. BuildReleasingWebDir rests on both halves; if either stops holding, its
// retry is either unnecessary or useless.
func TestWebClientWatcher_WorkingDirPinsWebDirUntilStop(t *testing.T) {
	web := filepath.Join(t.TempDir(), "deployment", "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	// "grandchild" announces itself once running: the child opens its working
	// directory during its own initialisation, after Start has returned, so
	// removing web/ straight after Start races it.
	cmd := helperCmd(t, "grandchild")
	cmd.Dir = web // as StartWebClientWatch runs the rollup runner
	setProcessGroup(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	wc, err := launchWatcher(cmd, &syncBuffer{}, stdout)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = wc.Stop() })

	for start := time.Now(); !strings.Contains(wc.Log(), "grandchild-started"); time.Sleep(20 * time.Millisecond) {
		if time.Since(start) > 10*time.Second {
			t.Fatalf("helper never announced itself:\n%s", wc.Log())
		}
	}

	// Control: with the process up, web/ cannot go.
	err = os.Remove(web)
	if err == nil {
		t.Fatal("removed deployment/web while a process had it as working directory; the #1342 premise no longer holds")
	}
	if !strings.Contains(err.Error(), "being used by another process") {
		t.Fatalf("remove failed with %v, want the sharing violation #1342 reported", err)
	}

	if err := wc.Stop(); err != nil {
		t.Fatal(err)
	}
	// The kernel drops the handle as the process is torn down; allow a moment.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err = os.Remove(web); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("deployment/web still pinned after Stop: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
