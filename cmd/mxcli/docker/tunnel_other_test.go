// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package docker

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// On a build that ships without the tunnel, --hub must fail with the actionable
// message rather than panicking, hanging, or pretending it worked. See ADR-0009.
func TestTunnelUnsupportedOffLinux(t *testing.T) {
	if TunnelSupported() {
		t.Fatal("TunnelSupported() = true on a non-Linux build")
	}

	var out bytes.Buffer
	tun, err := StartTunnel(TunnelOptions{
		HubURL:    "https://hub.example.com",
		LocalPort: 8080,
		Stdout:    &out,
	})
	if !errors.Is(err, ErrTunnelUnsupported) {
		t.Fatalf("StartTunnel error = %v, want ErrTunnelUnsupported", err)
	}
	if tun != nil {
		t.Errorf("StartTunnel returned a tunnel (%v) alongside the error", tun)
	}
	// It must not claim to have exposed anything.
	if out.Len() != 0 {
		t.Errorf("StartTunnel wrote progress output on an unsupported build: %q", out.String())
	}
}

// The message is the whole point of not hiding the command: it has to say where
// the tunnel does work and where to read more.
func TestUnsupportedMessageIsActionable(t *testing.T) {
	msg := ErrTunnelUnsupported.Error()
	for _, want := range []string{"Linux", "--hub", "devcontainer", "https://mendixlabs.github.io/mxcli/"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrTunnelUnsupported message is missing %q:\n%s", want, msg)
		}
	}
}
