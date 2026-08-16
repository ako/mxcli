// SPDX-License-Identifier: Apache-2.0

//go:build linux

// This is the single place in the package that imports the chisel tunnel client,
// so the Windows and macOS binaries never link it. See ADR-0009 for why that
// matters; the CI guard in .github/workflows/push-test.yml enforces it.

package docker

import (
	"context"
	"fmt"
	"time"

	chclient "github.com/jpillora/chisel/client"
)

const tunnelSupported = true

// chiselTunnel is the Linux implementation of tunnelConn.
type chiselTunnel struct {
	client *chclient.Client
	cancel context.CancelFunc
}

func (c *chiselTunnel) Close() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	if c.client != nil {
		_ = c.client.Close()
	}
}

// startTunnel opens the reverse tunnel. Options are already validated and
// defaulted by StartTunnel.
func startTunnel(o TunnelOptions) (tunnelConn, error) {
	// R:<remote>:127.0.0.1:<local> — the hub's chisel server listens on <remote>
	// and forwards to this app's <local> port (the app binds 127.0.0.1).
	remote := fmt.Sprintf("R:%d:127.0.0.1:%d", o.RemotePort, o.LocalPort)
	cfg := &chclient.Config{
		Server:           o.HubURL,
		Proxy:            o.Proxy,
		Auth:             o.Secret,
		Remotes:          []string{remote},
		KeepAlive:        25 * time.Second,
		MaxRetryCount:    -1, // retry forever: survive a hub restart or network blip
		MaxRetryInterval: 30 * time.Second,
	}
	c, err := chclient.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("configuring tunnel client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Start(ctx); err != nil {
		cancel()
		return nil, fmt.Errorf("starting tunnel: %w", err)
	}
	return &chiselTunnel{client: c, cancel: cancel}, nil
}
