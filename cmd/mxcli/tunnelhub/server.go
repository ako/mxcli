// SPDX-License-Identifier: Apache-2.0

package tunnelhub

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	chserver "github.com/jpillora/chisel/server"
	"golang.org/x/crypto/acme/autocert"
)

// ctxKey is the type for request-context values the front handler passes to the
// app reverse proxy.
type ctxKey int

const (
	targetPortKey ctxKey = iota
	publicHostKey
)

// ServerOptions configures the multi-tenant hub front.
type ServerOptions struct {
	// Domain is the wildcard base, e.g. "example.com". App previews are served at
	// <subdomain>.<Domain>.
	Domain string
	// HubHost is the control/admin/API host (default "hub."+Domain). Clients dial
	// their chisel control connection here and the admin page lives here.
	HubHost string
	// Registry is the shared backend store.
	Registry *Registry
	// TunnelAuth is the shared chisel auth ("user:pass"); empty disables auth.
	TunnelAuth string
	// RegisterSecret optionally gates /api/register (matched against X-Hub-Secret).
	RegisterSecret string
	// CertCacheDir is the autocert certificate cache directory.
	CertCacheDir string
	// chiselAddr is the internal address the embedded chisel control server binds
	// (default 127.0.0.1:8100). Not public — the front proxies the WS here.
	ChiselAddr string
}

// Server is the running multi-tenant hub: one embedded chisel reverse server
// (fanning in all client tunnels) behind a single-443 TLS front that routes by
// Host — the hub host to the admin/API/chisel-control, each preview subdomain to
// its tunnel.
type Server struct {
	opts    ServerOptions
	reg     *Registry
	chisel  *chserver.Server
	manager *autocert.Manager
	http    *http.Server
	apiMux  *http.ServeMux
	admin   http.Handler

	chiselProxy *httputil.ReverseProxy // -> internal chisel control (WS)
	appProxy    *httputil.ReverseProxy // -> 127.0.0.1:<reversePort> (per request)
}

// NewServer wires the registry, API, admin page, embedded chisel server, and the
// TLS front. Call Start to listen.
func NewServer(o ServerOptions) (*Server, error) {
	if o.Domain == "" {
		return nil, fmt.Errorf("Domain is required")
	}
	if o.HubHost == "" {
		o.HubHost = "hub." + o.Domain
	}
	if o.ChiselAddr == "" {
		o.ChiselAddr = "127.0.0.1:8100"
	}
	if o.Registry == nil {
		return nil, fmt.Errorf("Registry is required")
	}

	chisel, err := chserver.NewServer(&chserver.Config{Reverse: true, Auth: o.TunnelAuth})
	if err != nil {
		return nil, fmt.Errorf("chisel server: %w", err)
	}

	api := NewAPI(APIOptions{
		Registry:       o.Registry,
		ControlURL:     "https://" + o.HubHost,
		TunnelAuth:     o.TunnelAuth,
		RegisterSecret: o.RegisterSecret,
	})
	apiMux := http.NewServeMux()
	api.Mount(apiMux)

	s := &Server{
		opts:   o,
		reg:    o.Registry,
		chisel: chisel,
		apiMux: apiMux,
		admin:  NewAdmin(o.Registry),
	}

	// Front proxies: chisel control (WS) to the internal chisel server, and app
	// traffic to the per-request reverse port (Host preserved as the public host).
	chiselURL := &url.URL{Scheme: "http", Host: o.ChiselAddr}
	s.chiselProxy = httputil.NewSingleHostReverseProxy(chiselURL)

	s.appProxy = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			port, _ := req.Context().Value(targetPortKey).(int)
			host, _ := req.Context().Value(publicHostKey).(string)
			req.URL.Scheme = "http"
			req.URL.Host = fmt.Sprintf("127.0.0.1:%d", port)
			if host != "" {
				req.Host = host // the app sees its real public host, not the loopback
			}
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			// The subdomain is registered but nothing answers on its reverse port —
			// the client's tunnel is down (e.g. the container was reaped).
			writeOfflinePage(w)
		},
	}

	// autocert: issue a cert per host, but only for the hub host or a currently
	// registered subdomain — so a request for a random subdomain can't drive cert
	// issuance.
	s.manager = &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(o.CertCacheDir),
		HostPolicy: s.hostPolicy,
	}
	return s, nil
}

// hostPolicy allows the hub host and any registered preview subdomain.
func (s *Server) hostPolicy(_ context.Context, host string) error {
	if host == s.opts.HubHost {
		return nil
	}
	if sub, ok := s.subOf(host); ok {
		if _, found := s.reg.LookupSubdomain(sub); found {
			return nil
		}
	}
	return fmt.Errorf("host %q is not the hub or a registered preview", host)
}

// subOf returns the subdomain label of host under Domain (e.g. "app" from
// "app.example.com"), and whether host is under Domain.
func (s *Server) subOf(host string) (string, bool) {
	suffix := "." + s.opts.Domain
	if !strings.HasSuffix(host, suffix) {
		return "", false
	}
	sub := strings.TrimSuffix(host, suffix)
	if sub == "" || strings.Contains(sub, ".") {
		return "", false // only single-label subdomains
	}
	return sub, true
}

// ServeHTTP routes by Host: the hub host serves chisel control (WS upgrade),
// the API, and the admin page; a preview subdomain proxies to its tunnel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := stripPort(r.Host)
	switch {
	case host == s.opts.HubHost:
		if isWebSocketUpgrade(r) {
			s.chiselProxy.ServeHTTP(w, r) // chisel client control connection
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			s.apiMux.ServeHTTP(w, r)
			return
		}
		s.admin.ServeHTTP(w, r)
	default:
		sub, ok := s.subOf(host)
		if !ok {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		b, found := s.reg.LookupSubdomain(sub)
		if !found {
			writeNoSuchPreview(w, host)
			return
		}
		s.reg.TouchUsed(sub)
		ctx := context.WithValue(r.Context(), targetPortKey, b.ReversePort)
		ctx = context.WithValue(ctx, publicHostKey, host)
		s.appProxy.ServeHTTP(w, r.WithContext(ctx))
	}
}

// Start binds the internal chisel server and the public TLS front (443), plus an
// HTTP :80 listener for ACME challenges and http->https redirects. It blocks
// until ctx is cancelled.
func (s *Server) Start(ctx context.Context, httpsAddr, httpAddr string) error {
	if err := s.chisel.Start("127.0.0.1", portOf(s.opts.ChiselAddr)); err != nil {
		return fmt.Errorf("starting chisel: %w", err)
	}

	s.http = &http.Server{
		Addr:      httpsAddr,
		Handler:   s,
		TLSConfig: s.manager.TLSConfig(),
		// WebSocket/long-poll: no write timeout; bound only the header read.
		ReadHeaderTimeout: 20 * time.Second,
	}
	// :80 serves ACME HTTP-01 + redirects everything else to https.
	httpSrv := &http.Server{Addr: httpAddr, Handler: s.manager.HTTPHandler(redirectToHTTPS()), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = httpSrv.ListenAndServe() }()

	// Periodically trigger a reap so expired backends free their ports even when
	// nobody loads the admin page (List reaps as a side effect).
	reaper := time.NewTicker(30 * time.Second)
	defer reaper.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-reaper.C:
				s.reg.List("")
			}
		}
	}()

	errc := make(chan error, 1)
	go func() { errc <- s.http.ListenAndServeTLS("", "") }()

	select {
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.http.Shutdown(shutCtx)
		_ = httpSrv.Shutdown(shutCtx)
		_ = s.chisel.Close()
		return nil
	case err := <-errc:
		_ = httpSrv.Close()
		_ = s.chisel.Close()
		return err
	}
}

func redirectToHTTPS() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+stripPort(r.Host)+r.URL.RequestURI(), http.StatusMovedPermanently)
	})
}

func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

func stripPort(hostport string) string {
	if i := strings.LastIndexByte(hostport, ':'); i >= 0 && !strings.Contains(hostport[i:], "]") {
		return hostport[:i]
	}
	return hostport
}

func portOf(addr string) string {
	if i := strings.LastIndexByte(addr, ':'); i >= 0 {
		return addr[i+1:]
	}
	return addr
}

func writeNoSuchPreview(w http.ResponseWriter, host string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><title>No such preview</title>`+
		`<body style="font:16px system-ui;margin:4rem auto;max-width:32rem">`+
		`<h1>No preview here</h1><p>Nothing is registered for <code>%s</code>. `+
		`It may have ended, or the name is wrong.</p>`, html.EscapeString(host))
}

func writeOfflinePage(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadGateway)
	fmt.Fprint(w, `<!doctype html><meta charset=utf-8><title>Preview offline</title>`+
		`<body style="font:16px system-ui;margin:4rem auto;max-width:32rem">`+
		`<h1>Preview offline</h1><p>This preview is registered but its tunnel isn't `+
		`connected right now — the dev container may be asleep or reaped. It will `+
		`return when the app is running again.</p>`)
}
