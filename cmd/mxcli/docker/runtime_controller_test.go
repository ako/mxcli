// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// mockAdmin is a fake M2EE admin API. It records the sequence of actions it
// received and returns a scripted response per action. A response func may
// return a different value on each call (e.g. start-then-DDL-then-start).
type mockAdmin struct {
	calls    []string
	handlers map[string]func(call int) M2EEResponse
	seen     map[string]int
}

func newMockAdmin(t *testing.T, handlers map[string]func(call int) M2EEResponse) (*mockAdmin, M2EEOptions) {
	t.Helper()
	m := &mockAdmin{handlers: handlers, seen: map[string]int{}}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode admin request: %v", err)
		}
		m.calls = append(m.calls, req.Action)
		h, ok := m.handlers[req.Action]
		if !ok {
			// Default: success with empty feedback.
			_ = json.NewEncoder(w).Encode(M2EEResponse{})
			return
		}
		resp := h(m.seen[req.Action])
		m.seen[req.Action]++
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(ts.Close)

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}
	host, portStr, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host:port: %v", err)
	}
	port, _ := strconv.Atoi(portStr)
	// Direct + Token bypasses docker-exec and .env resolution.
	return m, M2EEOptions{Host: host, Port: port, Token: "test", Direct: true}
}

func ok(int) M2EEResponse { return M2EEResponse{} }

func TestDecideApply(t *testing.T) {
	if got := DecideApply(false); got != ActionReload {
		t.Errorf("DecideApply(false) = %v, want ActionReload", got)
	}
	if got := DecideApply(true); got != ActionRestart {
		t.Errorf("DecideApply(true) = %v, want ActionRestart", got)
	}
	if ActionReload.String() != "reload" || ActionRestart.String() != "restart" {
		t.Errorf("String() = %q/%q", ActionReload.String(), ActionRestart.String())
	}
}

func TestReloadModel(t *testing.T) {
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{"reload_model": ok})
	if err := NewRuntimeController(opts).ReloadModel(); err != nil {
		t.Fatalf("ReloadModel: %v", err)
	}
	if len(m.calls) != 1 || m.calls[0] != "reload_model" {
		t.Errorf("calls = %v, want [reload_model]", m.calls)
	}
}

func TestReloadModel_Error(t *testing.T) {
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{
		"reload_model": func(int) M2EEResponse { return M2EEResponse{Result: 1, Message: "boom"} },
	})
	if err := NewRuntimeController(opts).ReloadModel(); err == nil {
		t.Error("expected error from a non-zero result")
	}
	if len(m.calls) != 1 {
		t.Errorf("calls = %v", m.calls)
	}
}

func TestStart_CleanDatabase(t *testing.T) {
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{"start": ok})
	if _, err := NewRuntimeController(opts).Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(m.calls) != 1 || m.calls[0] != "start" {
		t.Errorf("calls = %v, want a single [start]", m.calls)
	}
}

func TestStart_AttachesLogSubscriber(t *testing.T) {
	// Capture the create_log_subscriber params so we can assert the file
	// subscriber is wired correctly (findings #25).
	var subParams map[string]any
	var actions []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string         `json:"action"`
			Params map[string]any `json:"params"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		actions = append(actions, req.Action)
		if req.Action == "create_log_subscriber" {
			subParams = req.Params
		}
		_ = json.NewEncoder(w).Encode(M2EEResponse{})
	}))
	t.Cleanup(ts.Close)
	host, port := parseTestServerAddr(t, ts.URL)

	c := NewRuntimeController(M2EEOptions{Host: host, Port: port, Token: "test", Direct: true})
	c.LogSubscriberFile = "/tmp/x/runtime.log"
	if _, err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The subscriber is attached AND logging is started after a successful start —
	// order matters: a standalone runtime boots with logging inactive (findings #25).
	if len(actions) != 3 || actions[0] != "start" || actions[1] != "create_log_subscriber" || actions[2] != "start_logging" {
		t.Fatalf("actions = %v, want [start create_log_subscriber start_logging]", actions)
	}
	if subParams == nil {
		t.Fatal("create_log_subscriber received no params")
	}
	if subParams["type"] != "file" {
		t.Errorf("type = %v, want file", subParams["type"])
	}
	if subParams["filename"] != "/tmp/x/runtime.log" {
		t.Errorf("filename = %v, want /tmp/x/runtime.log", subParams["filename"])
	}
	if subParams["autosubscribe"] != "INFO" {
		t.Errorf("autosubscribe = %v, want INFO", subParams["autosubscribe"])
	}
	if subParams["name"] != logSubscriberName {
		t.Errorf("name = %v, want %v", subParams["name"], logSubscriberName)
	}
	// max_rotate 0 = never rename the file (the JVM tee holds an fd on it). JSON
	// numbers decode to float64.
	if mr, ok := subParams["max_rotate"].(float64); !ok || mr != 0 {
		t.Errorf("max_rotate = %v, want 0", subParams["max_rotate"])
	}
}

func TestStart_NoLogSubscriberWhenUnset(t *testing.T) {
	// Without LogSubscriberFile, Start must not attach a subscriber.
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{"start": ok})
	if _, err := NewRuntimeController(opts).Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	for _, a := range m.calls {
		if a == "create_log_subscriber" {
			t.Fatalf("create_log_subscriber was called with no LogSubscriberFile set (calls=%v)", m.calls)
		}
	}
}

func TestStart_LogSubscriberFailureNonFatal(t *testing.T) {
	// A failing create_log_subscriber must not fail an otherwise-good start; the
	// app is already up. The failure is reported to Stdout instead.
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{
		"start":                 ok,
		"create_log_subscriber": func(int) M2EEResponse { return M2EEResponse{Result: 1, Message: "no disk"} },
	})
	var stdout bytes.Buffer
	c := NewRuntimeController(opts)
	c.LogSubscriberFile = "/tmp/x/runtime.log"
	c.Stdout = &stdout
	if _, err := c.Start(); err != nil {
		t.Fatalf("Start should not fail when the log subscriber fails: %v", err)
	}
	if len(m.calls) != 2 || m.calls[1] != "create_log_subscriber" {
		t.Errorf("calls = %v, want [start create_log_subscriber]", m.calls)
	}
	if !strings.Contains(stdout.String(), "application log not attached") {
		t.Errorf("expected a non-fatal warning on stdout, got %q", stdout.String())
	}
}

func TestStart_StartLoggingAlreadyStartedIsSuccess(t *testing.T) {
	// start_logging can be re-issued on a still-running JVM; an "already started"
	// response must be treated as success, not warned about (findings #25).
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{
		"start":                 ok,
		"create_log_subscriber": ok,
		"start_logging":         func(int) M2EEResponse { return M2EEResponse{Result: 1, Message: "Logging has already been started"} },
	})
	var stdout bytes.Buffer
	c := NewRuntimeController(opts)
	c.LogSubscriberFile = "/tmp/x/runtime.log"
	c.Stdout = &stdout
	if _, err := c.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(m.calls) != 3 || m.calls[2] != "start_logging" {
		t.Errorf("calls = %v, want [start create_log_subscriber start_logging]", m.calls)
	}
	if strings.Contains(stdout.String(), "not attached") {
		t.Errorf("an already-started logging response should be silent, got %q", stdout.String())
	}
}

func TestStart_OutOfDateDatabase(t *testing.T) {
	// First start reports the schema must change (result 3); after DDL, start succeeds.
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{
		"start": func(call int) M2EEResponse {
			if call == 0 {
				return M2EEResponse{Result: 3, Message: "database has to be updated"}
			}
			return M2EEResponse{}
		},
		"execute_ddl_commands": ok,
	})
	if _, err := NewRuntimeController(opts).Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	want := []string{"start", "execute_ddl_commands", "start"}
	if len(m.calls) != 3 || m.calls[0] != want[0] || m.calls[1] != want[1] || m.calls[2] != want[2] {
		t.Errorf("calls = %v, want %v", m.calls, want)
	}
}

func TestStart_DDLFails(t *testing.T) {
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{
		"start":                func(int) M2EEResponse { return M2EEResponse{Result: 3} },
		"execute_ddl_commands": func(int) M2EEResponse { return M2EEResponse{Result: 1, Message: "ddl boom"} },
	})
	if _, err := NewRuntimeController(opts).Start(); err == nil {
		t.Error("expected error when execute_ddl_commands fails")
	}
	// start once, ddl once, then abort (no second start).
	if len(m.calls) != 2 {
		t.Errorf("calls = %v, want [start execute_ddl_commands]", m.calls)
	}
}

func TestRuntimeStatus(t *testing.T) {
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{
		"runtime_status": func(int) M2EEResponse {
			return M2EEResponse{RawFeedback: json.RawMessage(`{"status":"running"}`)}
		},
	})
	status, err := NewRuntimeController(opts).RuntimeStatus()
	if err != nil {
		t.Fatalf("RuntimeStatus: %v", err)
	}
	if status != "running" {
		t.Errorf("status = %q, want running", status)
	}
	if len(m.calls) != 1 || m.calls[0] != "runtime_status" {
		t.Errorf("calls = %v", m.calls)
	}
}

func TestApplyBuild_HotReload(t *testing.T) {
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{"reload_model": ok})
	restartCalled := false
	action, err := NewRuntimeController(opts).ApplyBuild(
		&BuildResult{Status: "Success", RestartRequired: false},
		func() error { restartCalled = true; return nil },
	)
	if err != nil {
		t.Fatalf("ApplyBuild: %v", err)
	}
	if action != ActionReload {
		t.Errorf("action = %v, want ActionReload", action)
	}
	if restartCalled {
		t.Error("restart callback should not run for a hot-reloadable build")
	}
	if len(m.calls) != 1 || m.calls[0] != "reload_model" {
		t.Errorf("calls = %v, want [reload_model]", m.calls)
	}
}

func TestApplyBuild_Restart(t *testing.T) {
	m, opts := newMockAdmin(t, map[string]func(int) M2EEResponse{"start": ok})
	restartCalled := false
	action, err := NewRuntimeController(opts).ApplyBuild(
		&BuildResult{Status: "Success", RestartRequired: true},
		func() error { restartCalled = true; return nil },
	)
	if err != nil {
		t.Fatalf("ApplyBuild: %v", err)
	}
	if action != ActionRestart {
		t.Errorf("action = %v, want ActionRestart", action)
	}
	if !restartCalled {
		t.Error("restart callback must run for a restart-required build")
	}
	// After relaunch, ApplyBuild runs the Start cycle (clean DB -> single start).
	if len(m.calls) != 1 || m.calls[0] != "start" {
		t.Errorf("calls = %v, want [start]", m.calls)
	}
}

func TestApplyBuild_RestartNilCallback(t *testing.T) {
	// restart=nil means the caller drives relaunch; ApplyBuild makes no admin calls.
	m, opts := newMockAdmin(t, nil)
	action, err := NewRuntimeController(opts).ApplyBuild(
		&BuildResult{Status: "Success", RestartRequired: true}, nil)
	if err != nil {
		t.Fatalf("ApplyBuild: %v", err)
	}
	if action != ActionRestart {
		t.Errorf("action = %v, want ActionRestart", action)
	}
	if len(m.calls) != 0 {
		t.Errorf("calls = %v, want none", m.calls)
	}
}

func TestApplyBuild_NilBuild(t *testing.T) {
	_, opts := newMockAdmin(t, nil)
	if _, err := NewRuntimeController(opts).ApplyBuild(nil, nil); err == nil {
		t.Error("expected error for a nil build result")
	}
}

func TestNeedsDBUpdate(t *testing.T) {
	cases := []struct {
		name string
		resp *M2EEResponse
		want bool
	}{
		{"nil", nil, false},
		{"clean", &M2EEResponse{}, false},
		{"result3", &M2EEResponse{Result: 3}, true},
		{"message", &M2EEResponse{Message: "The database has to be updated first"}, true},
		{"result2", &M2EEResponse{Result: 2}, true},
		{"no-existing-db-message", &M2EEResponse{Message: "The database to be used does not exist."}, true},
		{"feedback", &M2EEResponse{RawFeedback: json.RawMessage(`{"synchronizationreason":"x"}`)}, true},
		{"other-error", &M2EEResponse{Result: 1, Message: "unrelated"}, false},
	}
	for _, tc := range cases {
		if got := needsDBUpdate(tc.resp); got != tc.want {
			t.Errorf("%s: needsDBUpdate = %v, want %v", tc.name, got, tc.want)
		}
	}
}
