// SPDX-License-Identifier: Apache-2.0

//go:build integration

package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestReconnectInsideScript_KeepsConnection is a regression test for the
// "not connected to a project" failure reported after running a doctype script
// that contains REFRESH CATALOG FULL (e.g. 11-navigation-examples.mdl).
//
// Trigger chain: a `.mxcli/catalog.db` already exists on disk, the project has
// since been written to, so REFRESH CATALOG FULL finds the cache stale
// ("project file modified") and calls reconnect(), which swaps the executor's
// backend for a fresh connection. When that reconnect happens INSIDE
// `execute script`, the outer script statement's ExecContext still holds the
// pre-reconnect (now-closed) backend; its syncBack after the script used to
// clobber e.backend with that stale value, silently disconnecting the session.
// The very next statement then failed with "not connected to a project".
func TestReconnectInsideScript_KeepsConnection(t *testing.T) {
	env := setupTestEnv(t)
	defer env.teardown()

	// 1. Build the catalog so a cache file lands on disk.
	if err := env.executeMDL("refresh catalog full;"); err != nil {
		t.Fatalf("initial refresh catalog: %v", err)
	}

	// 2. Modify the project so the on-disk cache becomes stale
	//    ("project file modified") — this is what makes the nested REFRESH take
	//    the reconnect branch rather than a cheap cache load.
	if err := env.executeMDL("create module ReconOuter;"); err != nil {
		t.Fatalf("create module ReconOuter: %v", err)
	}

	// 3. A script that modifies the project and then refreshes the (now-stale)
	//    catalog — the refresh reconnects the backend from inside the script.
	scriptPath := filepath.Join(t.TempDir(), "inner.mdl")
	script := "create module ReconInner;\nrefresh catalog full;\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatalf("write inner script: %v", err)
	}
	if err := env.executeMDL(fmt.Sprintf("execute script '%s';", scriptPath)); err != nil {
		t.Fatalf("execute script with nested reconnect: %v", err)
	}

	// 4. The session must still be connected after the script returns.
	if !env.executor.IsConnected() {
		t.Fatal("executor disconnected after a script that reconnected internally — the outer script context clobbered the live backend")
	}

	// 5. And a subsequent write must succeed (the user's symptom was the next
	//    statement failing with "not connected to a project").
	if err := env.executeMDL("create module AfterRecon;"); err != nil {
		t.Fatalf("write after script-internal reconnect failed (regression): %v", err)
	}
}
