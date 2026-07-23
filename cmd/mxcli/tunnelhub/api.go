// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"encoding/json"
	"net/http"
	"strings"
)

// APIOptions configures the registration API.
type APIOptions struct {
	Registry *Registry
	// ControlURL is where the client points its chisel control connection
	// (e.g. https://hub.mxcli.org). Returned in the registration response.
	ControlURL string
	// TunnelAuth is the shared chisel auth ("user:pass") the client must use, if
	// the hub's tunnel server requires one. Returned to the client.
	TunnelAuth string
	// RegisterSecret, if set, gates /api/register: the client must send a matching
	// X-Hub-Secret header (from --hub-secret). Empty means open registration.
	RegisterSecret string
	// HeartbeatIntervalSec is how often the client should heartbeat (default 20).
	HeartbeatIntervalSec int
}

// API serves the hub's registration + query endpoints over the registry.
type API struct {
	opts APIOptions
}

// RegisterResponse is returned to `mxcli run --hub` after registration.
type RegisterResponse struct {
	Subdomain            string `json:"subdomain"`
	URL                  string `json:"url"`
	ReversePort          int    `json:"reversePort"`
	ControlURL           string `json:"controlUrl"`
	Token                string `json:"token"`
	TunnelAuth           string `json:"tunnelAuth,omitempty"`
	HeartbeatIntervalSec int    `json:"heartbeatIntervalSec"`
}

// NewAPI builds the API handler set.
func NewAPI(o APIOptions) *API {
	if o.HeartbeatIntervalSec == 0 {
		o.HeartbeatIntervalSec = 20
	}
	return &API{opts: o}
}

// Mount registers the API routes on mux under /api/.
func (a *API) Mount(mux *http.ServeMux) {
	mux.HandleFunc("/api/register", a.handleRegister)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/deregister", a.handleDeregister)
	mux.HandleFunc("/api/backends", a.handleBackends)
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if a.opts.RegisterSecret != "" && r.Header.Get("X-Hub-Secret") != a.opts.RegisterSecret {
		http.Error(w, "invalid or missing hub secret", http.StatusUnauthorized)
		return
	}
	var req RegisterRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.Project) == "" {
		http.Error(w, "project is required", http.StatusBadRequest)
		return
	}
	b, err := a.opts.Registry.Register(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	host := b.Subdomain
	if d := a.opts.Registry.domain; d != "" {
		host = b.Subdomain + "." + d
	}
	writeJSON(w, http.StatusOK, RegisterResponse{
		Subdomain:            b.Subdomain,
		URL:                  "https://" + host,
		ReversePort:          b.ReversePort,
		ControlURL:           a.opts.ControlURL,
		Token:                b.ID,
		TunnelAuth:           a.opts.TunnelAuth,
		HeartbeatIntervalSec: a.opts.HeartbeatIntervalSec,
	})
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	a.byToken(w, r, func(token string) {
		if !a.opts.Registry.Heartbeat(token) {
			http.Error(w, "unknown token", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func (a *API) handleDeregister(w http.ResponseWriter, r *http.Request) {
	a.byToken(w, r, func(token string) {
		a.opts.Registry.Deregister(token)
		w.WriteHeader(http.StatusNoContent)
	})
}

// byToken extracts the bearer token (Authorization: Bearer <t> or ?token=) and
// invokes fn. POST only.
func (a *API) byToken(w http.ResponseWriter, r *http.Request, fn func(token string)) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	token := bearerToken(r)
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}
	fn(token)
}

func (a *API) handleBackends(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.opts.Registry.List(r.URL.Query().Get("sort")))
}

func bearerToken(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
