// SPDX-License-Identifier: Apache-2.0

//go:build !linux

package tunnelhub

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// On a build that ships without the tunnel, the hub must fail with the actionable
// message rather than panicking or binding a half-working front. See ADR-0009.
func TestHubUnsupportedOffLinux(t *testing.T) {
	if HubSupported() {
		t.Fatal("HubSupported() = true on a non-Linux build")
	}

	// Construction still succeeds — only Start refuses — so the portable routing
	// tests in this package keep running on every platform.
	reg := NewRegistry(RegistryOptions{Domain: "example.com"})
	srv, err := NewServer(ServerOptions{Domain: "example.com", Registry: reg})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	if err := srv.Start(context.Background(), "127.0.0.1:0", "127.0.0.1:0"); !errors.Is(err, ErrHubUnsupported) {
		t.Fatalf("Start error = %v, want ErrHubUnsupported", err)
	}
}

// The message is the whole point of not hiding the command: it has to say where
// the hub does run and where to read more.
func TestHubUnsupportedMessageIsActionable(t *testing.T) {
	msg := ErrHubUnsupported.Error()
	for _, want := range []string{"Linux", "tunnel-hub", "linux-amd64", "https://mendixlabs.github.io/mxcli/"} {
		if !strings.Contains(msg, want) {
			t.Errorf("ErrHubUnsupported message is missing %q:\n%s", want, msg)
		}
	}
}
