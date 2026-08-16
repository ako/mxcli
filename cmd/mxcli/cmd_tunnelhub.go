// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mendixlabs/mxcli/cmd/mxcli/tunnelhub"
	"github.com/mendixlabs/mxcli/cmd/mxcli/tunnelhub/audit"
	"github.com/spf13/cobra"
)

// tunnelHubCmd runs the multi-tenant ingress relay. It fronts many locally-running
// Mendix apps — across projects, solutions, branches, and worktrees — each at its
// own <subdomain>.<domain> over a single 443 connection, with a registration API
// and an admin overview at the hub host. Apps stay in their own (possibly
// egress-only) environments and reverse-tunnel out; nothing is pushed here.
var tunnelHubCmd = &cobra.Command{
	Use:   "tunnel-hub",
	Short: "Multi-tenant ingress relay: front many locally-running mxcli apps at per-preview subdomains",
	Long: `Run the static ingress relay that exposes locally-running Mendix apps
(started elsewhere with 'mxcli run --hub <this-url>') in a browser.

Each app self-registers and is served at its own subdomain
(<project>-<branch>.<domain>, or <prefix>-<project>-<branch> with --hub-prefix);
the hub host (hub.<domain>) serves the registration API, the admin overview, and
the tunnel control connection. Everything rides one 443 connection, so apps in
egress-only environments (e.g. Claude Code on the web) can reverse-tunnel out.

The hub is available in the Linux build only — it is a daemon you deploy on a
host, and the tunnel it embeds is left out of the Windows and macOS binaries so
they are not flagged by endpoint security for a capability they never use.

You run your own hub — there is no hosted service. Stand it up on a host you
control (a small VPS with a domain).

DNS: point a wildcard '*.<domain>' A record (and 'hub.<domain>') at this host.
TLS is issued per subdomain via Let's Encrypt on demand (needs inbound 80+443).

Security: set --secret and keep the hub to people you trust — the shared secret
gates registration. For a shared, internet-facing hub add GitHub OAuth with
--github-oauth-client-id / --github-oauth-client-secret: viewers sign in with
GitHub, each preview is owned by its registrar, the admin listing is filtered to
the viewer's own previews, and (with --require-auth, on by default) a non-owner
gets a 403. --audit-log records who signed in and who was denied.

Example (on your own VPS; *.example.com -> this host, ports 80+443 open):
  mxcli tunnel-hub --domain example.com --secret alice:s3cret

With GitHub OAuth (create an OAuth App with callback https://hub.example.com/auth/github/callback):
  mxcli tunnel-hub --domain example.com --secret alice:s3cret \
    --github-oauth-client-id <id> --github-oauth-client-secret <secret> \
    --session-secret <random> --audit-log ~/.mxcli/hub-audit.jsonl

Then, in each app's environment:
  mxcli run --hub https://hub.example.com --hub-secret alice:s3cret \
    --hub-solution CustomerPortal -p app.mpr
`,
	Run: func(cmd *cobra.Command, args []string) {
		// Fail before touching cert caches, key stores or session files: on a build
		// without the tunnel (everything but Linux — ADR-0009) the hub can never
		// serve, so it should not create state on the way to finding that out.
		if !tunnelhub.HubSupported() {
			fmt.Fprintf(os.Stderr, "Error: %v\n", tunnelhub.ErrHubUnsupported)
			os.Exit(1)
		}
		domain, _ := cmd.Flags().GetString("domain")
		hubHost, _ := cmd.Flags().GetString("hub-host")
		secret, _ := cmd.Flags().GetString("secret")
		httpsPort, _ := cmd.Flags().GetInt("port")
		httpPort, _ := cmd.Flags().GetInt("http-port")
		certCache, _ := cmd.Flags().GetString("cert-cache")
		ghClientID, _ := cmd.Flags().GetString("github-oauth-client-id")
		ghClientSecret, _ := cmd.Flags().GetString("github-oauth-client-secret")
		sessionSecret, _ := cmd.Flags().GetString("session-secret")
		cookieDomain, _ := cmd.Flags().GetString("cookie-domain")
		requireAuth, _ := cmd.Flags().GetBool("require-auth")
		auditLog, _ := cmd.Flags().GetString("audit-log")
		keysFile, _ := cmd.Flags().GetString("keys-file")
		sessionsFile, _ := cmd.Flags().GetString("sessions-file")
		sessionRetention, _ := cmd.Flags().GetDuration("session-retention")

		if domain == "" {
			fmt.Fprintln(os.Stderr, "Error: --domain is required (the wildcard base, e.g. example.com)")
			os.Exit(1)
		}
		if certCache == "" {
			home, _ := os.UserHomeDir()
			certCache = filepath.Join(home, ".mxcli", "hub-certs")
		}
		// Persist keys by default when auth is on, so they survive restarts and
		// users don't have to re-configure MXCLI_HUB_KEY after every redeploy.
		if keysFile == "" && ghClientID != "" {
			home, _ := os.UserHomeDir()
			keysFile = filepath.Join(home, ".mxcli", "hub-keys.json")
		}
		// Env fallbacks so secrets need not appear in the process table.
		if ghClientSecret == "" {
			ghClientSecret = os.Getenv("MXCLI_HUB_GH_SECRET")
		}
		if sessionSecret == "" {
			sessionSecret = os.Getenv("MXCLI_HUB_SESSION_SECRET")
		}

		auditSink, err := audit.New(auditLog)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: opening --audit-log %q: %v\n", auditLog, err)
			os.Exit(1)
		}

		auth := buildHubAuth(hubHost, domain, cookieDomain, ghClientID, ghClientSecret, sessionSecret, requireAuth, auditSink)

		// Persist the per-session endpoint history by default so the overview shows
		// past sessions across restarts (30-day window unless overridden).
		if sessionsFile == "" {
			home, _ := os.UserHomeDir()
			sessionsFile = filepath.Join(home, ".mxcli", "hub-sessions.json")
		}
		sessions, err := tunnelhub.NewSessionLogFile(sessionsFile, sessionRetention)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: opening --sessions-file %q: %v\n", sessionsFile, err)
			os.Exit(1)
		}

		reg := tunnelhub.NewRegistry(tunnelhub.RegistryOptions{Domain: domain, Sessions: sessions})
		srv, err := tunnelhub.NewServer(tunnelhub.ServerOptions{
			Domain:         domain,
			HubHost:        hubHost,
			Registry:       reg,
			TunnelAuth:     secret,
			RegisterSecret: secret,
			CertCacheDir:   certCache,
			Auth:           auth,
			Audit:          auditSink,
			KeysFile:       keysFile,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: configuring tunnel-hub: %v\n", err)
			os.Exit(1)
		}
		if auth != nil {
			mode := "soft (owner-filtered listing, previews open)"
			if requireAuth {
				mode = "required (owner check enforced on previews)"
			}
			fmt.Printf("  auth: GitHub OAuth enabled, %s\n", mode)
			if keysFile != "" {
				fmt.Printf("  keys: durable at %s (survive restarts)\n", keysFile)
			}
		}

		host := hubHost
		if host == "" {
			host = "hub." + domain
		}
		fmt.Printf("mxcli tunnel-hub: serving *.%s (control/admin at https://%s) on :%d\n", domain, host, httpsPort)
		fmt.Printf("  admin overview: https://%s/\n", host)

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if err := srv.Start(ctx, fmt.Sprintf(":%d", httpsPort), fmt.Sprintf(":%d", httpPort)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: tunnel-hub: %v\n", err)
			os.Exit(1)
		}
	},
}

// buildHubAuth assembles the AuthConfig from the CLI flags, or returns nil (open
// mode) when no GitHub OAuth client id is configured. When auth is on but no
// --session-secret is given, it generates an ephemeral key and warns that
// sessions won't survive a hub restart.
func buildHubAuth(hubHost, domain, cookieDomain, clientID, clientSecret, sessionSecret string, requireAuth bool, sink audit.Sink) *tunnelhub.AuthConfig {
	if clientID == "" {
		return nil // open mode
	}
	if hubHost == "" {
		hubHost = "hub." + domain
	}
	if cookieDomain == "" {
		cookieDomain = "." + domain // SSO across every *.<domain> preview
	}
	secretBytes := []byte(sessionSecret)
	if len(secretBytes) == 0 {
		secretBytes = make([]byte, 32)
		_, _ = rand.Read(secretBytes)
		fmt.Fprintln(os.Stderr, "Warning: no --session-secret (or MXCLI_HUB_SESSION_SECRET) set; "+
			"using an ephemeral key — existing sessions are invalidated on every hub restart.")
	}
	return &tunnelhub.AuthConfig{
		GitHubClientID:     clientID,
		GitHubClientSecret: clientSecret,
		SessionSecret:      secretBytes,
		CookieDomain:       cookieDomain,
		HubHost:            hubHost,
		RequireAuth:        requireAuth,
		Audit:              sink,
	}
}

func init() {
	tunnelHubCmd.Flags().String("domain", "", "Wildcard base domain you control, e.g. example.com (previews served at <sub>.<domain>)")
	tunnelHubCmd.Flags().String("hub-host", "", "Control/admin host (default hub.<domain>)")
	tunnelHubCmd.Flags().String("secret", "", "Shared secret (\"user:pass\") apps present via --hub-secret; empty = open")
	tunnelHubCmd.Flags().Int("port", 443, "HTTPS port to listen on")
	tunnelHubCmd.Flags().Int("http-port", 80, "HTTP port for ACME challenges + http->https redirect")
	tunnelHubCmd.Flags().String("cert-cache", "", "Directory for Let's Encrypt certificates (default ~/.mxcli/hub-certs)")
	tunnelHubCmd.Flags().String("github-oauth-client-id", "", "GitHub OAuth App client id; enables the viewer auth plane (empty = open mode)")
	tunnelHubCmd.Flags().String("github-oauth-client-secret", "", "GitHub OAuth App client secret (or env MXCLI_HUB_GH_SECRET)")
	tunnelHubCmd.Flags().String("session-secret", "", "HMAC key for the SSO session cookie (or env MXCLI_HUB_SESSION_SECRET); random if unset")
	tunnelHubCmd.Flags().String("cookie-domain", "", "Session cookie domain for SSO across previews (default .<domain>)")
	tunnelHubCmd.Flags().Bool("require-auth", true, "Enforce the owner check on previews (deny non-owners); --require-auth=false leaves owned previews open but still filters the listing")
	tunnelHubCmd.Flags().String("audit-log", "", "Append-only JSONL audit trail path (\"stdout\" for stdout; empty = off)")
	tunnelHubCmd.Flags().String("keys-file", "", "Durable hub API-key store path (default ~/.mxcli/hub-keys.json when auth is on); keys survive restarts so clients keep their MXCLI_HUB_KEY")
	tunnelHubCmd.Flags().String("sessions-file", "", "Durable per-session endpoint history path (default ~/.mxcli/hub-sessions.json); lets the overview show past sessions across restarts")
	tunnelHubCmd.Flags().Duration("session-retention", tunnelhub.DefaultSessionRetention, "How long an offline session/endpoint stays in the overview before it is pruned")
	rootCmd.AddCommand(tunnelHubCmd)
}
