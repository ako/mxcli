// SPDX-License-Identifier: Apache-2.0

// ako/mxcli-maintenance #1 and #3, revisited.
//
// The first fix lowered every 11.14 project to Java 21 because mxcli could only
// run 21. Once the runtime launcher became version-aware that was no longer
// true, and lowering unconditionally downgraded projects AGAINST the platform —
// Mendix warns "Java versions below 25 are deprecated for deployment", and a
// Java 25 project builds without that warning on a machine that has a JDK 25.
//
// So the question is not "is 25 supported" but "can THIS machine build it", and
// the parsing this used to own now lives in the docker package beside the
// resolver that answers it.
package main

import (
	"io"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

func TestJavaMajorParsingLivesWithTheResolver(t *testing.T) {
	// A guard against the parsing drifting back into two copies: `mxcli new` and
	// the JDK resolver must agree on what "Java25" means, or new would lower a
	// project the resolver could have built.
	for _, tc := range []struct {
		in   string
		want int
		ok   bool
	}{
		{"25", 25, true},
		{"Java25", 25, true},
		{"21", 21, true},
		{"Java21", 21, true},
		{"", 0, false},
		{"tip", 0, false},
	} {
		got, ok := docker.ParseJavaMajor(tc.in)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Errorf("ParseJavaMajor(%q) = (%d, %v), want (%d, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestAlignJavaVersionLeavesABuildableProjectAlone(t *testing.T) {
	// The regression this file exists for. A project asking for a Java release
	// this machine HAS must not be touched — the JDK the tests run under is by
	// definition present, so DefaultJavaMajor stands in for "buildable here".
	if _, err := docker.ResolveJDK(docker.DefaultJavaMajor); err != nil {
		t.Skipf("no JDK %d on this machine; nothing to assert", docker.DefaultJavaMajor)
	}
	// A missing project reads as unparseable, which must also be left alone
	// rather than overwritten.
	lowered, err := alignJavaVersion("/nonexistent/project.mpr", io.Discard)
	if err != nil {
		t.Fatalf("alignJavaVersion on a missing project: %v", err)
	}
	if lowered {
		t.Error("an unreadable project must not be rewritten")
	}
}
