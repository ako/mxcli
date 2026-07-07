// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"strings"
	"testing"
)

// The -32000 verify-on-timeout behavior: Studio Pro's server-side time limit
// frequently fires while the operation still applies, so mutation sites must
// not report a landed write as failed (a blind re-run duplicates elements —
// observed live on 11.12).

func noVerifyDelay(t *testing.T) {
	t.Helper()
	old := timeoutVerifyDelay
	timeoutVerifyDelay = 0
	t.Cleanup(func() { timeoutVerifyDelay = old })
}

func TestIsTimeoutErr(t *testing.T) {
	if !isTimeoutErr(&rpcError{Code: -32000, Message: "Request timed out"}) {
		t.Error("-32000 Request timed out should be a timeout")
	}
	if isTimeoutErr(&rpcError{Code: -32602, Message: "Invalid arguments"}) {
		t.Error("validation error is not a timeout")
	}
	if isTimeoutErr(nil) {
		t.Error("nil is not a timeout")
	}
}

func TestPgWritePage_TimeoutRetriesIdempotently(t *testing.T) {
	noVerifyDelay(t)
	patchCalls := 0
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		return "Page 'P' patched successfully.", false
	})
	f.rpcErr = func(name string, _ map[string]any) (int, string, bool) {
		if name == "pg_patch_page" {
			patchCalls++
			if patchCalls == 1 {
				return -32000, "Request timed out", true
			}
		}
		return 0, "", false
	}
	b := &Backend{client: f.connectClient(t)}
	if err := b.pgWritePage("M", "P", map[string]any{"title": "P"}); err != nil {
		t.Fatalf("timed-out root replace should retry (idempotent) and succeed, got: %v", err)
	}
	if patchCalls != 2 {
		t.Errorf("want 2 pg_patch_page calls (original + retry), got %d", patchCalls)
	}
}

func TestPgWritePage_DoubleTimeoutSurfacesGuidance(t *testing.T) {
	noVerifyDelay(t)
	f := newFakePED(t, nil)
	f.rpcErr = func(name string, _ map[string]any) (int, string, bool) {
		if name == "pg_patch_page" {
			return -32000, "Request timed out", true
		}
		return 0, "", false
	}
	b := &Backend{client: f.connectClient(t)}
	err := b.pgWritePage("M", "P", map[string]any{"title": "P"})
	if err == nil || !strings.Contains(err.Error(), "could not be verified") {
		t.Fatalf("double timeout should surface verify guidance, got: %v", err)
	}
}

func TestPedCreateDocument_TimeoutVerifiedApplied(t *testing.T) {
	noVerifyDelay(t)
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_find_document" {
			return `{"foundDocuments":[{"qualifiedName":"M.MyEnum","folderPath":""}]}`, false
		}
		return "SUCCESS", false
	})
	f.rpcErr = func(name string, _ map[string]any) (int, string, bool) {
		if name == "ped_create_document" {
			return -32000, "Request timed out", true
		}
		return 0, "", false
	}
	b := &Backend{client: f.connectClient(t)}
	if err := b.pedCreateDocument("M", "Enumerations$Enumeration", "MyEnum", map[string]any{}, ""); err != nil {
		t.Fatalf("timed-out create verified via ped_find_document should succeed, got: %v", err)
	}
	if !b.dirty["M"] {
		t.Error("verified-applied create must still mark the module dirty")
	}
}

func TestPedCreateDocument_TimeoutNotApplied(t *testing.T) {
	noVerifyDelay(t)
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_find_document" {
			return `{"foundDocuments":[]}`, false
		}
		return "SUCCESS", false
	})
	f.rpcErr = func(name string, _ map[string]any) (int, string, bool) {
		if name == "ped_create_document" {
			return -32000, "Request timed out", true
		}
		return 0, "", false
	}
	b := &Backend{client: f.connectClient(t)}
	err := b.pedCreateDocument("M", "Enumerations$Enumeration", "MyEnum", map[string]any{}, "")
	if err == nil || !strings.Contains(err.Error(), "could not be verified") {
		t.Fatalf("unverified timed-out create must fail with guidance, got: %v", err)
	}
	if b.dirty["M"] {
		t.Error("unverified create must not mark the module dirty")
	}
}

func TestPedUpdateVerify_TimeoutVerifiedApplied(t *testing.T) {
	noVerifyDelay(t)
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_read_document" {
			return `{"folderPath":"","results":[{"path":"/entities","result":[{"$Type":"DomainModels$Entity","$QualifiedName":"M.Item"}]}]}`, false
		}
		return "SUCCESS", false
	})
	f.rpcErr = func(name string, _ map[string]any) (int, string, bool) {
		if name == "ped_update_document" {
			return -32000, "Request timed out", true
		}
		return 0, "", false
	}
	b := &Backend{client: f.connectClient(t)}
	err := b.pedUpdateVerify("M", "entity M.Item",
		func() (bool, error) { return b.domainModelHasEntity("M", "Item") },
		pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "add", Value: map[string]any{}}})
	if err != nil {
		t.Fatalf("timed-out entity add verified via /entities read should succeed, got: %v", err)
	}
	if !b.dirty["M"] {
		t.Error("verified-applied update must mark the module dirty")
	}
}

func TestPedUpdateVerify_TimeoutNotApplied(t *testing.T) {
	noVerifyDelay(t)
	f := newFakePED(t, func(name string, _ map[string]any) (string, bool) {
		if name == "ped_read_document" {
			return `{"folderPath":"","results":[{"path":"/entities","result":[]}]}`, false
		}
		return "SUCCESS", false
	})
	f.rpcErr = func(name string, _ map[string]any) (int, string, bool) {
		if name == "ped_update_document" {
			return -32000, "Request timed out", true
		}
		return 0, "", false
	}
	b := &Backend{client: f.connectClient(t)}
	err := b.pedUpdateVerify("M", "entity M.Item",
		func() (bool, error) { return b.domainModelHasEntity("M", "Item") },
		pedOpEntry{Path: "/entities", Operation: pedOperation{Type: "add", Value: map[string]any{}}})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("unverified timed-out add must fail with the duplicate warning, got: %v", err)
	}
}
