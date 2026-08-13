// SPDX-License-Identifier: Apache-2.0

// The dev-loop handshake is how `mxcli constant set --apply` finds the app a
// `mxcli run --local` is serving, and — crucially — the configuration payload
// that runtime was booted with. The admin API has no read-back, so a second
// process cannot ask what the configuration is; it can only be told.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func handshakeProject(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "App.mpr")
}

func TestDevLoopHandshake_RoundTrip(t *testing.T) {
	p := handshakeProject(t)
	want := devLoopHandshake{
		Project:   p,
		PID:       os.Getpid(),
		AppPort:   8080,
		AdminPort: 8090,
		AdminPass: "mxcli-local-dev",
		BootConfig: map[string]any{
			"BasePath":           "/tmp/app/deployment",
			"DatabaseName":       "app",
			"MicroflowConstants": map[string]any{"A.B": "v"},
		},
		Started: time.Now(),
	}
	if err := writeDevLoopHandshake(p, want); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := readDevLoopHandshake(p)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.AdminPort != 8090 || got.AdminPass != "mxcli-local-dev" {
		t.Errorf("admin details lost: %+v", got)
	}
	if got.BootConfig["BasePath"] != "/tmp/app/deployment" {
		t.Fatalf("BootConfig lost: %v — without it a caller has to guess at every "+
			"setting it is not changing", got.BootConfig)
	}
}

// It carries a live admin credential and the runtime's database password.
func TestDevLoopHandshake_Mode0600(t *testing.T) {
	p := handshakeProject(t)
	if err := writeDevLoopHandshake(p, devLoopHandshake{PID: os.Getpid()}); err != nil {
		t.Fatalf("write: %v", err)
	}
	info, err := os.Stat(devLoopHandshakePath(p))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("mode = %o, want 600", perm)
	}
}

// No dev loop is a normal state, and the message has to say what to do about it
// rather than name a missing file.
func TestReadDevLoopHandshake_MissingSaysWhatToDo(t *testing.T) {
	_, err := readDevLoopHandshake(handshakeProject(t))
	if err == nil {
		t.Fatal("a missing handshake read as success")
	}
	if !strings.Contains(err.Error(), "run --local") {
		t.Errorf("the error does not say how to fix it: %v", err)
	}
}

// A handshake left behind by a dead process must be refused, not used: its ports
// may since have been taken by something else, and sending a configuration
// change to the wrong process is worse than reporting no dev loop.
func TestReadDevLoopHandshake_RefusesADeadProcess(t *testing.T) {
	p := handshakeProject(t)
	if err := writeDevLoopHandshake(p, devLoopHandshake{Project: p, PID: 999999, AdminPort: 8090}); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := readDevLoopHandshake(p)
	if err == nil {
		t.Fatal("a handshake from a dead process was accepted")
	}
	if !strings.Contains(err.Error(), "999999") {
		t.Errorf("the error does not name the stale pid: %v", err)
	}
}

func TestReadDevLoopHandshake_CorruptFileIsNamed(t *testing.T) {
	p := handshakeProject(t)
	if err := os.MkdirAll(filepath.Dir(devLoopHandshakePath(p)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(devLoopHandshakePath(p), []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readDevLoopHandshake(p); err == nil {
		t.Fatal("a corrupt handshake read as success")
	}
}

func TestRemoveDevLoopHandshake(t *testing.T) {
	p := handshakeProject(t)
	if err := writeDevLoopHandshake(p, devLoopHandshake{PID: os.Getpid()}); err != nil {
		t.Fatal(err)
	}
	removeDevLoopHandshake(p)
	if _, err := os.Stat(devLoopHandshakePath(p)); !os.IsNotExist(err) {
		t.Errorf("the handshake survived removal: %v", err)
	}
	removeDevLoopHandshake(p) // idempotent
}
