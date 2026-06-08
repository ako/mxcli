// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"
)

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
	if _, err := b.CheckModel(""); err == nil {
		t.Fatal("CheckModel without Concord should error")
	}
	if _, err := b.GetAppStatus(); err == nil {
		t.Fatal("GetAppStatus without Concord should error")
	}
	if err := b.RunApp(); err == nil {
		t.Fatal("RunApp without Concord should error")
	}
	if err := b.StopApp(); err == nil {
		t.Fatal("StopApp without Concord should error")
	}
}

func TestWriteCheckReport(t *testing.T) {
	r := &CheckResult{Healthy: true}
	r.Summary.ErrorCount = 1
	r.Summary.WarningCount = 1
	r.Errors = []CheckItem{{Module: "M", Entity: "E", Code: "bad_ref", Message: "broken"}}
	r.Warnings = []CheckItem{{Module: "M", Entity: "W", Code: "no_attributes", Message: "empty"}}
	var sb strings.Builder
	writeCheckReport(&sb, r)
	out := sb.String()
	if !strings.Contains(out, "1 error(s), 1 warning(s)") ||
		!strings.Contains(out, "ERROR  M.E [bad_ref]: broken") ||
		!strings.Contains(out, "warn   M.W [no_attributes]: empty") {
		t.Fatalf("report:\n%s", out)
	}
}
