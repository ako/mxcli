// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"testing"
	"time"
)

// fakeClock is a controllable time source for deterministic tests.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) add(d time.Duration) { c.t = c.t.Add(d) }

func newTestRegistry(clk *fakeClock) *Registry {
	return NewRegistry(RegistryOptions{
		Domain:    "example.com",
		PortBase:  9001,
		PortCount: 5,
		StaleFor:  45 * time.Second,
		ExpireFor: 10 * time.Minute,
		Now:       clk.now,
	})
}

func TestRegister_AllocatesSubdomainAndPort(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := newTestRegistry(clk)

	b, err := r.Register(RegisterRequest{Project: "MyApp", Branch: "feature/x", AppPort: 8080})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if b.Subdomain != "myapp-feature-x" {
		t.Errorf("subdomain = %q, want myapp-feature-x", b.Subdomain)
	}
	if b.ReversePort != 9001 {
		t.Errorf("reversePort = %d, want 9001", b.ReversePort)
	}
	if b.ID == "" {
		t.Error("token must be set")
	}
	// main branch collapses to just the project.
	m, _ := r.Register(RegisterRequest{Project: "MyApp", Branch: "main"})
	if m.Subdomain != "myapp" {
		t.Errorf("main-branch subdomain = %q, want myapp", m.Subdomain)
	}
}

func TestRegister_Prefix(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := newTestRegistry(clk)

	// prefix namespaces the hostname: <prefix>-<project>[-<branch>].
	a, _ := r.Register(RegisterRequest{Prefix: "AcmeCorp", Project: "Portal", Branch: "feature/x"})
	if a.Subdomain != "acmecorp-portal-feature-x" {
		t.Errorf("prefixed subdomain = %q, want acmecorp-portal-feature-x", a.Subdomain)
	}
	// main branch with a prefix -> <prefix>-<project>.
	b, _ := r.Register(RegisterRequest{Prefix: "AcmeCorp", Project: "Portal", Branch: "main"})
	if b.Subdomain != "acmecorp-portal" {
		t.Errorf("prefixed main subdomain = %q, want acmecorp-portal", b.Subdomain)
	}
	// same project, different prefix -> distinct slot (prefix is part of identity).
	c, _ := r.Register(RegisterRequest{Prefix: "Other", Project: "Portal", Branch: "main"})
	if c.Subdomain != "other-portal" || c.ID == b.ID {
		t.Errorf("different prefix should be a distinct backend: %q id=%s vs %s", c.Subdomain, c.ID, b.ID)
	}
}

func TestRegister_SameIdentityIsStable(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := newTestRegistry(clk)

	a, _ := r.Register(RegisterRequest{Project: "App", Branch: "main", AppPort: 8080})
	clk.add(30 * time.Second)
	b, _ := r.Register(RegisterRequest{Project: "App", Branch: "main", AppPort: 8080})

	if a.ID != b.ID || a.Subdomain != b.Subdomain || a.ReversePort != b.ReversePort {
		t.Errorf("re-register changed the slot: %+v vs %+v", a, b)
	}
	if !b.LastSeenAt.Equal(clk.t) {
		t.Error("re-register should refresh the heartbeat")
	}
	if got := r.List("used"); len(got) != 1 {
		t.Errorf("want 1 backend after re-register, got %d", len(got))
	}
}

func TestRegister_CollisionDisambiguation(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := newTestRegistry(clk)

	// Same project+branch, different worktree -> worktree disambiguates.
	a, _ := r.Register(RegisterRequest{Project: "App", Branch: "dev", Worktree: "wt-a"})
	b, _ := r.Register(RegisterRequest{Project: "App", Branch: "dev", Worktree: "wt-b"})
	if a.Subdomain != "app-dev" {
		t.Errorf("first subdomain = %q, want app-dev", a.Subdomain)
	}
	if b.Subdomain != "app-dev-wt-b" {
		t.Errorf("second subdomain = %q, want app-dev-wt-b", b.Subdomain)
	}
	// A third with no distinguishing worktree -> numeric suffix.
	c, _ := r.Register(RegisterRequest{Project: "App", Branch: "dev"})
	if c.Subdomain != "app-dev-2" {
		t.Errorf("third subdomain = %q, want app-dev-2", c.Subdomain)
	}
}

func TestAvailability_StaleAfterNoHeartbeat(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := newTestRegistry(clk)
	b, _ := r.Register(RegisterRequest{Project: "App", Branch: "main"})

	if av := r.List("used")[0].Availability; av != Available {
		t.Errorf("fresh backend = %q, want available", av)
	}
	clk.add(60 * time.Second) // past StaleFor (45s), before ExpireFor
	if av := r.List("used")[0].Availability; av != Stale {
		t.Errorf("after 60s = %q, want stale", av)
	}
	// A heartbeat brings it back.
	r.Heartbeat(b.ID)
	if av := r.List("used")[0].Availability; av != Available {
		t.Errorf("after heartbeat = %q, want available", av)
	}
}

func TestReap_RemovesExpiredAndFreesPort(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := newTestRegistry(clk)
	first, _ := r.Register(RegisterRequest{Project: "App", Branch: "main"})

	clk.add(11 * time.Minute) // past ExpireFor
	// List triggers a reap.
	if got := r.List("used"); len(got) != 0 {
		t.Fatalf("expired backend should be reaped, got %d", len(got))
	}
	// Its port is freed and reusable.
	next, _ := r.Register(RegisterRequest{Project: "Other", Branch: "main"})
	if next.ReversePort != first.ReversePort {
		t.Errorf("freed port %d should be reused, got %d", first.ReversePort, next.ReversePort)
	}
}

func TestList_Sorting(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := newTestRegistry(clk)

	a, _ := r.Register(RegisterRequest{Project: "Beta", Solution: "S", Branch: "main"})
	clk.add(time.Second)
	b, _ := r.Register(RegisterRequest{Project: "Alpha", Solution: "S", Branch: "main"})

	// used: Beta (a) is touched last -> most recently used -> first.
	r.TouchUsed(b.Subdomain)
	clk.add(time.Second)
	r.TouchUsed(a.Subdomain)
	if got := r.List("used"); got[0].Project != "Beta" {
		t.Errorf("used sort: first = %q, want Beta", got[0].Project)
	}
	// registered: newest first -> Alpha.
	if got := r.List("registered"); got[0].Project != "Alpha" {
		t.Errorf("registered sort: first = %q, want Alpha", got[0].Project)
	}
	// project: alphabetical -> Alpha.
	if got := r.List("project"); got[0].Project != "Alpha" {
		t.Errorf("project sort: first = %q, want Alpha", got[0].Project)
	}
}

func TestPortExhaustion(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	r := NewRegistry(RegistryOptions{Domain: "example.com", PortBase: 9001, PortCount: 2, Now: clk.now})
	if _, err := r.Register(RegisterRequest{Project: "A", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register(RegisterRequest{Project: "B", Branch: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register(RegisterRequest{Project: "C", Branch: "main"}); err == nil {
		t.Error("expected port-exhaustion error on the 3rd registration")
	}
}
