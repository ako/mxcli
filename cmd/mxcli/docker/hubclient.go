// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// HubMeta identifies a preview to the hub: it drives the subdomain and the
// overview grouping. Blank fields are auto-detected where possible.
type HubMeta struct {
	Prefix   string // optional hostname namespace (org/solution/team/env)
	Project  string // default: the .mpr basename
	Solution string // optional grouping for multi-app solutions
	Branch   string // default: the project's git branch
	Worktree string // optional; distinguishes worktrees of one branch
}

// HubRegistration is the result of registering with a hub: everything the client
// needs to tunnel and to advertise the app under its public URL.
type HubRegistration struct {
	URL               string        // assigned public URL -> ApplicationRootUrl
	Subdomain         string        // "" in single-app fallback
	ControlURL        string        // where the chisel client connects
	ReversePort       int           // reverse port to tunnel to
	Token             string        // heartbeat/deregister auth ("" in fallback)
	TunnelAuth        string        // chisel auth to use
	HeartbeatInterval time.Duration // 0 in fallback
	MultiTenant       bool          // false when we fell back to a slice-1 single-app hub

	hubURL string
}

type registerResponse struct {
	Subdomain            string `json:"subdomain"`
	URL                  string `json:"url"`
	ReversePort          int    `json:"reversePort"`
	ControlURL           string `json:"controlUrl"`
	Token                string `json:"token"`
	TunnelAuth           string `json:"tunnelAuth"`
	HeartbeatIntervalSec int    `json:"heartbeatIntervalSec"`
}

// RegisterWithHub registers a preview with a multi-tenant hub. If the hub has no
// registration API (a slice-1 single-app hub), it falls back to serving directly
// at the hub URL on the default reverse port. The HTTP client uses the standard
// proxy environment (honouring NO_PROXY), so an external hub goes through the
// egress proxy.
func RegisterWithHub(hubURL, secret string, meta HubMeta, appPort int) (*HubRegistration, error) {
	body, _ := json.Marshal(map[string]any{
		"prefix":   meta.Prefix,
		"project":  meta.Project,
		"solution": meta.Solution,
		"branch":   meta.Branch,
		"worktree": meta.Worktree,
		"appPort":  appPort,
	})
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(hubURL, "/")+"/api/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Hub-Secret", secret)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contacting hub: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var rr registerResponse
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&rr); err != nil {
			return nil, fmt.Errorf("decoding hub response: %w", err)
		}
		return &HubRegistration{
			URL:               rr.URL,
			Subdomain:         rr.Subdomain,
			ControlURL:        rr.ControlURL,
			ReversePort:       rr.ReversePort,
			Token:             rr.Token,
			TunnelAuth:        rr.TunnelAuth,
			HeartbeatInterval: time.Duration(rr.HeartbeatIntervalSec) * time.Second,
			MultiTenant:       true,
			hubURL:            hubURL,
		}, nil
	case http.StatusNotFound:
		// No registration API — a single-app hub. Serve directly at the hub URL.
		return &HubRegistration{
			URL:         strings.TrimRight(hubURL, "/"),
			ControlURL:  hubURL,
			ReversePort: DefaultHubBackendPort,
			TunnelAuth:  secret,
			MultiTenant: false,
			hubURL:      hubURL,
		}, nil
	default:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("hub registration failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
}

// Heartbeat periodically pings the hub so the preview shows as available, and
// deregisters on Stop. No-op for a single-app (fallback) registration.
type Heartbeat struct {
	cancel context.CancelFunc
	done   chan struct{}
	reg    *HubRegistration
}

// StartHeartbeat begins heartbeating (only for a multi-tenant registration).
func StartHeartbeat(reg *HubRegistration) *Heartbeat {
	if !reg.MultiTenant || reg.HeartbeatInterval <= 0 {
		return &Heartbeat{reg: reg}
	}
	ctx, cancel := context.WithCancel(context.Background())
	h := &Heartbeat{cancel: cancel, done: make(chan struct{}), reg: reg}
	go func() {
		defer close(h.done)
		t := time.NewTicker(reg.HeartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				postToken(ctx, reg.hubURL+"/api/status", reg.Token)
			}
		}
	}()
	return h
}

// Stop ends heartbeating and best-effort deregisters the preview from the hub.
func (h *Heartbeat) Stop() {
	if h.cancel != nil {
		h.cancel()
		<-h.done
	}
	if h.reg != nil && h.reg.MultiTenant && h.reg.Token != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		postToken(ctx, h.reg.hubURL+"/api/deregister", h.reg.Token)
	}
}

func postToken(ctx context.Context, url, token string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}
}

// DetectHubMeta fills a HubMeta from the project path + git, with explicit
// overrides taking precedence.
func DetectHubMeta(projectPath string, override HubMeta) HubMeta {
	m := override
	if m.Project == "" {
		base := filepath.Base(projectPath)
		m.Project = strings.TrimSuffix(base, filepath.Ext(base))
	}
	if m.Branch == "" {
		m.Branch = gitBranch(filepath.Dir(projectPath))
	}
	return m
}

// gitBranch returns the current branch of the repo containing dir, or "".
func gitBranch(dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	b := strings.TrimSpace(string(out))
	if b == "HEAD" { // detached
		return ""
	}
	return b
}
