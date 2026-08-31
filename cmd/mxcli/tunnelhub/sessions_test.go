// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSessionURL(t *testing.T) {
	cases := map[string]string{
		"cse_01JX": "https://claude.ai/code/session_01JX",
		"cse_":     "", // no id after prefix
		"abc123":   "", // not a remote id
		"":         "",
	}
	for in, want := range cases {
		if got := sessionURL(in); got != want {
			t.Errorf("sessionURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSessionLog_PersistAndPrune(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "hub-sessions.json")
	// Relative to the real clock, unlike the fixed base the other tests here
	// use, and that difference is load-bearing: NewSessionLogFile prunes inside
	// load(), before a test can inject its clock, so the prune on reload runs
	// against time.Now() no matter what is assigned to log2.now afterwards. With
	// a hardcoded base this test passes until the wall clock drifts past the
	// retention window and then fails for good -- which is what happened on
	// 2026-08-31, exactly 30 days after a base of 2026-08-01 and a 30-day
	// retention, on every branch at once.
	base := time.Now().UTC()

	log, err := NewSessionLogFile(path, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("NewSessionLogFile: %v", err)
	}
	log.now = func() time.Time { return base }
	log.Record(EndpointRecord{
		Session: "cse_A", Owner: "alice", Project: "App", Branch: "main",
		Subdomain: "app", URL: "https://app.example.com",
		RegisteredAt: base.Add(-2 * time.Hour), LastSeenAt: base.Add(-time.Hour),
	})
	// A stale record that should be pruned on reload (older than retention).
	log.Record(EndpointRecord{
		Session: "cse_OLD", Owner: "alice", Project: "Ancient", Branch: "main",
		Subdomain: "ancient", LastSeenAt: base.Add(-40 * 24 * time.Hour),
	})

	// Reload from disk. The prune happens during load, on the real clock: the
	// 40-day-old record is outside the 30-day window and the 1-hour-old one is
	// comfortably inside it, so the outcome does not depend on when this runs.
	log2, err := NewSessionLogFile(path, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	log2.now = func() time.Time { return base }
	snap := log2.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("after reload+prune: %d records, want 1 (%+v)", len(snap), snap)
	}
	if snap[0].Session != "cse_A" || snap[0].Project != "App" {
		t.Errorf("unexpected surviving record: %+v", snap[0])
	}
}

func TestSessionLog_RecordUpsertKeepsEarliestRegistered(t *testing.T) {
	log := NewSessionLog(30 * 24 * time.Hour)
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	log.now = func() time.Time { return base }

	e := EndpointRecord{Session: "s", Owner: "o", Project: "P", Branch: "main"}
	e.RegisteredAt, e.LastSeenAt = base.Add(-3*time.Hour), base.Add(-3*time.Hour)
	log.Record(e)
	e.RegisteredAt, e.LastSeenAt = base.Add(-time.Hour), base // a reconnect
	log.Record(e)

	snap := log.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("want 1 merged record, got %d", len(snap))
	}
	if !snap[0].RegisteredAt.Equal(base.Add(-3 * time.Hour)) {
		t.Errorf("RegisteredAt = %v, want earliest -3h", snap[0].RegisteredAt)
	}
	if !snap[0].LastSeenAt.Equal(base) {
		t.Errorf("LastSeenAt = %v, want latest (base)", snap[0].LastSeenAt)
	}
}

func TestRegistry_SessionsGroupsAndRetainsOffline(t *testing.T) {
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	now := base
	reg := NewRegistry(RegistryOptions{
		Domain:    "example.com",
		StaleFor:  45 * time.Second,
		ExpireFor: 10 * time.Minute,
		Sessions:  NewSessionLog(30 * 24 * time.Hour),
		Now:       func() time.Time { return now },
	})
	reg.sessions.now = func() time.Time { return now }

	// Session A exposes two endpoints; session B one.
	reg.Register(RegisterRequest{Session: "cse_A", Owner: "alice", Project: "Web", Branch: "main"})
	reg.Register(RegisterRequest{Session: "cse_A", Owner: "alice", Project: "Api", Branch: "main"})
	reg.Register(RegisterRequest{Session: "cse_B", Owner: "alice", Project: "Web", Branch: "feature"})

	sessions := reg.Sessions("")
	if len(sessions) != 2 {
		t.Fatalf("want 2 sessions, got %d", len(sessions))
	}
	byID := map[string]SessionView{}
	for _, s := range sessions {
		byID[s.Session] = s
	}
	if a := byID["cse_A"]; len(a.Endpoints) != 2 || !a.Online || a.SessionURL != "https://claude.ai/code/session_A" {
		t.Errorf("session A wrong: eps=%d online=%v url=%q", len(a.Endpoints), a.Online, a.SessionURL)
	}
	for _, e := range byID["cse_A"].Endpoints {
		if e.State != "available" {
			t.Errorf("A endpoint %s state=%q, want available", e.Project, e.State)
		}
	}

	// Advance past expiry so session B's endpoint is reaped → offline history.
	now = base.Add(11 * time.Minute)
	sessions = reg.Sessions("")
	// A's endpoints are also stale now (no heartbeat) but still live; B is offline.
	var b SessionView
	var found bool
	for _, s := range sessions {
		if s.Session == "cse_B" {
			b, found = s, true
		}
	}
	if !found {
		t.Fatal("session B dropped from history after reap; want it retained offline")
	}
	if b.Online {
		t.Errorf("session B should be offline after reap")
	}
	if len(b.Endpoints) != 1 || b.Endpoints[0].State != "offline" {
		t.Fatalf("session B endpoint should be offline: %+v", b.Endpoints)
	}
}

// A reaped backend that reconnects gets a fresh RegisteredAt, so first-seen must
// come from the durable log instead — otherwise the overview reports a
// long-running preview as brand new after every container reap.
func TestRegistry_SessionsFirstSeenSurvivesReconnect(t *testing.T) {
	base := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	now := base
	reg := NewRegistry(RegistryOptions{
		Domain: "example.com", ExpireFor: 10 * time.Minute,
		Sessions: NewSessionLog(30 * 24 * time.Hour), Now: func() time.Time { return now },
	})
	reg.sessions.now = func() time.Time { return now }

	req := RegisterRequest{Session: "cse_A", Owner: "alice", Project: "Web", Branch: "main"}
	if _, err := reg.Register(req); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Idle past expiry so the live backend is reaped, then reconnect.
	now = base.Add(3 * time.Hour)
	if _, err := reg.Register(req); err != nil {
		t.Fatalf("re-Register: %v", err)
	}

	sessions := reg.Sessions("")
	if len(sessions) != 1 || len(sessions[0].Endpoints) != 1 {
		t.Fatalf("want 1 session with 1 endpoint, got %+v", sessions)
	}
	ep := sessions[0].Endpoints[0]
	if ep.State != "available" {
		t.Fatalf("endpoint should be live after reconnect, got %q", ep.State)
	}
	if !ep.RegisteredAt.Equal(now) {
		t.Errorf("RegisteredAt = %v, want the reconnect time %v", ep.RegisteredAt, now)
	}
	if !ep.FirstSeenAt.Equal(base) {
		t.Errorf("FirstSeenAt = %v, want the original registration %v", ep.FirstSeenAt, base)
	}
	if !sessions[0].FirstSeen.Equal(base) {
		t.Errorf("session FirstSeen = %v, want %v", sessions[0].FirstSeen, base)
	}
}

func TestRegistry_SessionsViewerScoped(t *testing.T) {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	reg := NewRegistry(RegistryOptions{
		Domain: "example.com", Sessions: NewSessionLog(0), Now: func() time.Time { return now },
	})
	reg.Register(RegisterRequest{Session: "cse_A", Owner: "alice", Project: "Web", Branch: "main"})
	reg.Register(RegisterRequest{Session: "cse_B", Owner: "bob", Project: "Web", Branch: "main"})

	if got := reg.Sessions("alice"); len(got) != 1 || got[0].Owner != "alice" {
		t.Errorf("viewer alice should see only her session, got %+v", got)
	}
	if got := reg.Sessions(""); len(got) != 2 {
		t.Errorf("open mode should see all sessions, got %d", len(got))
	}
}
