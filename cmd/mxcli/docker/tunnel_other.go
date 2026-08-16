// SPDX-License-Identifier: Apache-2.0

//go:build !linux

// The tunnel ships only in the Linux build. This stub keeps the rest of the
// package compiling unchanged on Windows and macOS, and — because it is the only
// other half of the seam — guarantees those binaries link no tunnel code at all.
// See ADR-0009 and .github/workflows/push-test.yml (the dependency guard).

package docker

const tunnelSupported = false

func startTunnel(TunnelOptions) (tunnelConn, error) { return nil, ErrTunnelUnsupported }
