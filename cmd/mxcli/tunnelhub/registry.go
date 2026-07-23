// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Availability is a backend's liveness, derived from heartbeat freshness.
type Availability string

const (
	// Available: the container heartbeat is fresh — the app should be reachable.
	Available Availability = "available"
	// Stale: no recent heartbeat (e.g. a Claude Code web container reaped on idle),
	// but not yet expired — shown so the user can see it went away.
	Stale Availability = "stale"
)

// Backend is one registered preview: a locally-running app reverse-tunnelled to
// the hub and served at its subdomain.
type Backend struct {
	ID          string `json:"id"`       // opaque token (auth for heartbeat/deregister + chisel)
	Project     string `json:"project"`  // e.g. the .mpr name
	Solution    string `json:"solution"` // optional grouping for multi-app solutions
	Branch      string `json:"branch"`   // git branch
	Worktree    string `json:"worktree"` // optional, distinguishes worktrees of one branch
	Subdomain   string `json:"subdomain"`
	ReversePort int    `json:"reversePort"`
	AppPort     int    `json:"appPort"`

	RegisteredAt time.Time `json:"registeredAt"`
	LastSeenAt   time.Time `json:"lastSeenAt"` // last heartbeat
	LastUsedAt   time.Time `json:"lastUsedAt"` // last browser request to the subdomain
}

// identity is the stable key for a preview across reconnects: same project +
// branch + worktree + solution re-registers to the same slot (stable URL).
func (b *Backend) identity() string {
	return strings.Join([]string{b.Solution, b.Project, b.Branch, b.Worktree}, "\x00")
}

// BackendView is a Backend plus derived fields, for the API/admin page.
type BackendView struct {
	Backend
	URL          string       `json:"url"`
	Availability Availability `json:"availability"`
	UptimeSec    int64        `json:"uptimeSec"`
}

// RegisterRequest is the registration payload from `mxcli run --hub`.
type RegisterRequest struct {
	Project  string `json:"project"`
	Solution string `json:"solution"`
	Branch   string `json:"branch"`
	Worktree string `json:"worktree"`
	AppPort  int    `json:"appPort"`
}

// Registry is the in-memory store of registered backends. All methods are safe
// for concurrent use.
type Registry struct {
	mu          sync.Mutex
	byID        map[string]*Backend
	bySubdomain map[string]*Backend
	byIdentity  map[string]*Backend
	usedPorts   map[int]bool

	domain    string // e.g. "mxcli.org"
	portBase  int    // first reverse port to allocate
	portCount int    // number of reverse ports available
	staleFor  time.Duration
	expireFor time.Duration
	now       func() time.Time
}

// RegistryOptions configures a Registry. Zero values get sensible defaults.
type RegistryOptions struct {
	Domain    string
	PortBase  int
	PortCount int
	StaleFor  time.Duration // no heartbeat within this -> Stale (default 45s)
	ExpireFor time.Duration // no heartbeat within this -> removed (default 10m)
	Now       func() time.Time
}

// NewRegistry creates an empty registry.
func NewRegistry(o RegistryOptions) *Registry {
	if o.PortBase == 0 {
		o.PortBase = 9001
	}
	if o.PortCount == 0 {
		o.PortCount = 200
	}
	if o.StaleFor == 0 {
		o.StaleFor = 45 * time.Second
	}
	if o.ExpireFor == 0 {
		o.ExpireFor = 10 * time.Minute
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return &Registry{
		byID:        map[string]*Backend{},
		bySubdomain: map[string]*Backend{},
		byIdentity:  map[string]*Backend{},
		usedPorts:   map[int]bool{},
		domain:      o.Domain,
		portBase:    o.PortBase,
		portCount:   o.PortCount,
		staleFor:    o.StaleFor,
		expireFor:   o.ExpireFor,
		now:         o.Now,
	}
}

// Register allocates (or refreshes) a backend for the request and returns it. A
// re-registration with the same identity (project/branch/worktree/solution)
// returns the existing slot with a fresh heartbeat, so URLs are stable across
// reconnects.
func (r *Registry) Register(req RegisterRequest) (*Backend, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reapLocked()

	now := r.now()
	b := &Backend{
		Project:  strings.TrimSpace(req.Project),
		Solution: strings.TrimSpace(req.Solution),
		Branch:   strings.TrimSpace(req.Branch),
		Worktree: strings.TrimSpace(req.Worktree),
		AppPort:  req.AppPort,
	}
	if existing, ok := r.byIdentity[b.identity()]; ok {
		existing.LastSeenAt = now
		existing.AppPort = req.AppPort
		return existing, nil
	}

	port, err := r.allocPortLocked()
	if err != nil {
		return nil, err
	}
	b.ID = newToken()
	b.Subdomain = r.allocSubdomainLocked(b.Project, b.Branch, b.Worktree)
	b.ReversePort = port
	b.RegisteredAt = now
	b.LastSeenAt = now
	b.LastUsedAt = time.Time{}

	r.byID[b.ID] = b
	r.bySubdomain[b.Subdomain] = b
	r.byIdentity[b.identity()] = b
	r.usedPorts[port] = true
	return b, nil
}

// Heartbeat refreshes a backend's liveness by token.
func (r *Registry) Heartbeat(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.byID[id]
	if !ok {
		return false
	}
	b.LastSeenAt = r.now()
	return true
}

// TouchUsed records a browser request to a subdomain (updates LastUsedAt).
func (r *Registry) TouchUsed(subdomain string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.bySubdomain[subdomain]; ok {
		b.LastUsedAt = r.now()
	}
}

// Deregister removes a backend by token.
func (r *Registry) Deregister(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.byID[id]
	if !ok {
		return false
	}
	r.removeLocked(b)
	return true
}

// LookupSubdomain returns the backend serving a subdomain.
func (r *Registry) LookupSubdomain(subdomain string) (*Backend, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.bySubdomain[subdomain]
	if !ok {
		return nil, false
	}
	cp := *b
	return &cp, true
}

// List returns a snapshot of all backends as views, sorted by the given key
// ("used", "registered", "project"; default "used"), most-recent/first.
func (r *Registry) List(sortKey string) []BackendView {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reapLocked()

	out := make([]BackendView, 0, len(r.byID))
	for _, b := range r.byID {
		out = append(out, r.viewLocked(b))
	}
	sortViews(out, sortKey)
	return out
}

// viewLocked builds the derived view for a backend.
func (r *Registry) viewLocked(b *Backend) BackendView {
	host := b.Subdomain
	if r.domain != "" {
		host = b.Subdomain + "." + r.domain
	}
	av := Available
	if r.now().Sub(b.LastSeenAt) > r.staleFor {
		av = Stale
	}
	return BackendView{
		Backend:      *b,
		URL:          "https://" + host,
		Availability: av,
		UptimeSec:    int64(r.now().Sub(b.RegisteredAt).Seconds()),
	}
}

// reapLocked removes backends whose heartbeat is older than expireFor.
func (r *Registry) reapLocked() {
	cutoff := r.now().Add(-r.expireFor)
	for _, b := range r.byID {
		if b.LastSeenAt.Before(cutoff) {
			r.removeLocked(b)
		}
	}
}

func (r *Registry) removeLocked(b *Backend) {
	delete(r.byID, b.ID)
	delete(r.bySubdomain, b.Subdomain)
	delete(r.byIdentity, b.identity())
	delete(r.usedPorts, b.ReversePort)
}

// allocPortLocked returns a free reverse port from the configured range.
func (r *Registry) allocPortLocked() (int, error) {
	for p := r.portBase; p < r.portBase+r.portCount; p++ {
		if !r.usedPorts[p] {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free reverse port (all %d in use)", r.portCount)
}

// allocSubdomainLocked returns a unique subdomain slug, disambiguating a
// collision with the worktree name then a numeric suffix.
func (r *Registry) allocSubdomainLocked(project, branch, worktree string) string {
	base := baseSlug(project, branch)
	if _, taken := r.bySubdomain[base]; !taken {
		return base
	}
	if wt := slugify(worktree); wt != "" {
		cand := truncateLabel(base + "-" + wt)
		if _, taken := r.bySubdomain[cand]; !taken {
			return cand
		}
	}
	for i := 2; ; i++ {
		cand := truncateLabel(fmt.Sprintf("%s-%d", base, i))
		if _, taken := r.bySubdomain[cand]; !taken {
			return cand
		}
	}
}

func newToken() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// sortViews orders views in place by key, newest first for time keys.
func sortViews(v []BackendView, key string) {
	less := map[string]func(a, b BackendView) bool{
		"registered": func(a, b BackendView) bool { return a.RegisteredAt.After(b.RegisteredAt) },
		"project": func(a, b BackendView) bool {
			if a.Solution != b.Solution {
				return a.Solution < b.Solution
			}
			if a.Project != b.Project {
				return a.Project < b.Project
			}
			return a.Branch < b.Branch
		},
		"used": func(a, b BackendView) bool { return a.LastUsedAt.After(b.LastUsedAt) },
	}[key]
	if less == nil {
		less = func(a, b BackendView) bool { return a.LastUsedAt.After(b.LastUsedAt) }
	}
	sort.SliceStable(v, func(i, j int) bool { return less(v[i], v[j]) })
}
