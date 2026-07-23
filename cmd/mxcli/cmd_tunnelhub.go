// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	chserver "github.com/jpillora/chisel/server"
	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

// tunnelHubCmd is the static ingress relay. It runs on a host with a public IP
// and domain (e.g. a small VPS) and fronts a locally-running mxcli app: the app
// stays in its own (possibly egress-only) environment and reverse-tunnels out to
// this hub over 443; browsers reach it at the hub's URL. Nothing is pushed here —
// only live HTTP flows through the tunnel.
//
// Slice 1 fronts a single app. Multi-tenant registration (a /register API that
// allocates per-container subdomains + tokens) and the admin overview are a
// follow-on (see PROPOSAL_mxcli_dev_warm_loop.md § Scaling).
var tunnelHubCmd = &cobra.Command{
	Use:   "tunnel-hub",
	Short: "Static ingress relay that fronts a locally-running mxcli app at a public URL",
	Long: `Run a static ingress relay that exposes a locally-running Mendix app
(started elsewhere with 'mxcli run --hub <this-url>') in a browser at this host's
public URL.

It embeds a chisel reverse-tunnel server: the app's environment dials in over 443
and reverse-tunnels its local port here; every non-tunnel HTTP request to this
host is proxied down that tunnel to the app. Everything rides a single 443
connection, so it works from egress-only environments (e.g. Claude Code on the
web).

TLS: pass --domain for automatic Let's Encrypt (the host must be reachable on 80
and 443 for the ACME challenge), or --tls-cert/--tls-key for an existing
certificate.

Example (on a VPS with hub.mxcli.org -> this host, ports 80+443 open):
  mxcli tunnel-hub --domain hub.mxcli.org --secret myuser:mypass

Then, in the app's environment:
  mxcli run --hub https://hub.mxcli.org --hub-secret myuser:mypass -p app.mpr
`,
	Run: func(cmd *cobra.Command, args []string) {
		domain, _ := cmd.Flags().GetString("domain")
		secret, _ := cmd.Flags().GetString("secret")
		port, _ := cmd.Flags().GetInt("port")
		backendPort, _ := cmd.Flags().GetInt("backend-port")
		tlsKey, _ := cmd.Flags().GetString("tls-key")
		tlsCert, _ := cmd.Flags().GetString("tls-cert")
		host, _ := cmd.Flags().GetString("host")

		hasCert := tlsKey != "" && tlsCert != ""
		if domain == "" && !hasCert {
			fmt.Fprintln(os.Stderr, "Error: --domain (automatic Let's Encrypt) or both --tls-cert and --tls-key are required")
			os.Exit(1)
		}

		cfg := &chserver.Config{
			Reverse: true,
			Auth:    secret,
			// Proxy is chisel's HTTP backend: non-tunnel requests are reverse-proxied
			// here, which is the reverse-tunnel listener the app dials into.
			Proxy: fmt.Sprintf("http://127.0.0.1:%d", backendPort),
			TLS: chserver.TLSConfig{
				Key:  tlsKey,
				Cert: tlsCert,
			},
		}
		if domain != "" {
			cfg.TLS.Domains = []string{domain}
		}

		srv, err := chserver.NewServer(cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: configuring tunnel-hub: %v\n", err)
			os.Exit(1)
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if domain != "" {
			fmt.Printf("mxcli tunnel-hub: fronting https://%s on %s:%d (reverse port %d)\n", domain, host, port, backendPort)
			fmt.Printf("  app connects with: mxcli run --hub https://%s%s -p app.mpr\n", domain, secretHint(secret))
		} else {
			fmt.Printf("mxcli tunnel-hub: listening on %s:%d (reverse port %d)\n", host, port, backendPort)
		}

		if err := srv.StartContext(ctx, host, strconv.Itoa(port)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: starting tunnel-hub: %v\n", err)
			os.Exit(1)
		}
		if err := srv.Wait(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: tunnel-hub stopped: %v\n", err)
			os.Exit(1)
		}
	},
}

// secretHint renders " --hub-secret <secret>" for the copy-paste hint, or "" when
// no secret is set.
func secretHint(secret string) string {
	if secret == "" {
		return ""
	}
	return " --hub-secret " + secret
}

func init() {
	tunnelHubCmd.Flags().String("domain", "", "Domain for automatic Let's Encrypt TLS (e.g. hub.mxcli.org); host must be reachable on 80+443")
	tunnelHubCmd.Flags().String("secret", "", "Shared auth secret (\"user:pass\") the app must present via --hub-secret")
	tunnelHubCmd.Flags().Int("port", 443, "Public port to listen on")
	tunnelHubCmd.Flags().Int("backend-port", docker.DefaultHubBackendPort, "Reverse-tunnel port the app dials into (must match the app side; default 9000)")
	tunnelHubCmd.Flags().String("tls-cert", "", "TLS certificate file (instead of --domain autocert)")
	tunnelHubCmd.Flags().String("tls-key", "", "TLS key file (instead of --domain autocert)")
	tunnelHubCmd.Flags().String("host", "0.0.0.0", "Address to bind")
	rootCmd.AddCommand(tunnelHubCmd)
}
