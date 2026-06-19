// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"slices"
	"testing"
)

func TestInjectLDPreload(t *testing.T) {
	const lib = "/usr/lib/aarch64-linux-gnu/libfreetype.so.6"

	tests := []struct {
		name        string
		env         []string
		wantChanged bool
		wantEntry   string // expected LD_PRELOAD value; "" means assert no LD_PRELOAD added beyond input
	}{
		{
			name:        "no existing LD_PRELOAD appends",
			env:         []string{"PATH=/bin", "HOME=/root"},
			wantChanged: true,
			wantEntry:   "LD_PRELOAD=" + lib,
		},
		{
			name:        "freetype already preloaded is left alone",
			env:         []string{"LD_PRELOAD=/some/other/libfreetype.so.6"},
			wantChanged: false,
			wantEntry:   "LD_PRELOAD=/some/other/libfreetype.so.6",
		},
		{
			name:        "non-freetype LD_PRELOAD gets lib prepended",
			env:         []string{"LD_PRELOAD=/opt/libfoo.so"},
			wantChanged: true,
			wantEntry:   "LD_PRELOAD=" + lib + ":/opt/libfoo.so",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := slices.Clone(tt.env)
			got, changed := injectLDPreload(input, lib)
			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			// Input must never be mutated.
			if !slices.Equal(input, tt.env) {
				t.Errorf("input env mutated: %v", input)
			}
			var found string
			for _, e := range got {
				if after, ok := stringsCutPreload(e); ok {
					found = "LD_PRELOAD=" + after
				}
			}
			if found != tt.wantEntry {
				t.Errorf("LD_PRELOAD = %q, want %q", found, tt.wantEntry)
			}
			// Exactly one LD_PRELOAD entry in the result.
			count := 0
			for _, e := range got {
				if _, ok := stringsCutPreload(e); ok {
					count++
				}
			}
			if count > 1 {
				t.Errorf("expected at most one LD_PRELOAD entry, got %d in %v", count, got)
			}
		})
	}
}

func stringsCutPreload(e string) (string, bool) {
	const p = "LD_PRELOAD="
	if len(e) >= len(p) && e[:len(p)] == p {
		return e[len(p):], true
	}
	return "", false
}
