// SPDX-License-Identifier: Apache-2.0

//go:build linux

// This is the single place in the package that imports the chisel tunnel server,
// so the Windows and macOS binaries never link it. See ADR-0009 for why that
// matters; the CI guard in .github/workflows/push-test.yml enforces it.

package tunnelhub

import (
	"fmt"

	chserver "github.com/jpillora/chisel/server"
)

const hubSupported = true

// chiselControl is the Linux implementation of controlServer.
type chiselControl struct{ srv *chserver.Server }

func (c *chiselControl) Start(host, port string) error { return c.srv.Start(host, port) }
func (c *chiselControl) Close() error                  { return c.srv.Close() }

// newControlServer builds the embedded reverse-tunnel control server. auth is the
// shared "user:pass" every client presents; empty leaves the hub open.
func newControlServer(auth string) (controlServer, error) {
	srv, err := chserver.NewServer(&chserver.Config{Reverse: true, Auth: auth})
	if err != nil {
		return nil, fmt.Errorf("tunnel control server: %w", err)
	}
	return &chiselControl{srv: srv}, nil
}
