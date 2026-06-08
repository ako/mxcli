// SPDX-License-Identifier: Apache-2.0

package mcp

import "testing"

func TestConcordFailed(t *testing.T) {
	cases := map[string]bool{
		`{"success":true,"message":"saved"}`: false,
		`{"status":"ok","data":{}}`:          false,
		`{"error":"not allowed"}`:            true,
		`{"success":false,"message":"nope"}`: true,
		`{"success": false }`:                true,
		`Module 'X' created successfully.`:   false,
	}
	for text, want := range cases {
		if got := concordFailed(text); got != want {
			t.Errorf("concordFailed(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestConcordCall_NotConfigured(t *testing.T) {
	b := &Backend{} // no Concord client
	if err := b.SaveAll(); err == nil {
		t.Fatal("SaveAll without Concord should error")
	}
	if _, err := b.concordCall("save_all", nil); err == nil {
		t.Fatal("concordCall without Concord should error with an actionable message")
	}
}
