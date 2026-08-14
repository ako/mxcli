// SPDX-License-Identifier: Apache-2.0

//go:build !linux

// The tunnel control server ships only in the Linux build. This stub keeps the
// rest of the package — registry, API, auth, keys, sessions, admin, and the whole
// Host-routing front — compiling *and testable* unchanged on Windows and macOS,
// while guaranteeing those binaries link no tunnel code at all. See ADR-0009 and
// the dependency guard in .github/workflows/push-test.yml.
//
// The seam is at Start, not at construction, on purpose: a Server that cannot
// bind a control server is still a Server whose routing can be exercised, so the
// portable tests run on every platform rather than only where the tunnel ships.

package tunnelhub

const hubSupported = false

// unsupportedControl stands in for the control server on builds without it.
type unsupportedControl struct{}

func (unsupportedControl) Start(string, string) error { return ErrHubUnsupported }
func (unsupportedControl) Close() error               { return nil }

func newControlServer(string) (controlServer, error) { return unsupportedControl{}, nil }
