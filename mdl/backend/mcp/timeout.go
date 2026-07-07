// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Studio Pro enforces an internal ~30s limit per tool call and answers with a
// JSON-RPC -32000 "Request timed out" when an operation takes longer — but the
// operation frequently APPLIES anyway (observed live: 11.11 navigation writes,
// and repeatedly on 11.12 page and entity creates — the response times out
// while the model change lands). Reporting these as failures is worse than
// wrong: a user (or agent) who re-runs the "failed" statement — especially in
// a fresh session, whose hybrid reads can't see Studio Pro's unsaved in-memory
// state — can duplicate model elements. So mutation call sites verify on
// timeout: an idempotent op (pg_patch_page root replace) is retried once; a
// non-idempotent create is confirmed by reading back whether its target now
// exists, and only reported failed when it verifiably did not apply.

// rpcCodeServerTimeout is the JSON-RPC error code Studio Pro uses for its
// server-side per-call time limit.
const rpcCodeServerTimeout = -32000

// timeoutVerifyDelay gives a timed-out operation a moment to finish applying
// before the verification read (or idempotent retry). Variable so tests can
// shorten it.
var timeoutVerifyDelay = 2 * time.Second

// isTimeoutErr reports whether an MCP call failed with the server-side
// "Request timed out" (-32000).
func isTimeoutErr(err error) bool {
	if rpc, ok := errors.AsType[*rpcError](err); ok {
		return rpc.Code == rpcCodeServerTimeout && strings.Contains(strings.ToLower(rpc.Message), "timed out")
	}
	return false
}

// timeoutNotice tells the user a timed-out operation was verified applied —
// the outcome is success, but the slow round-trip is worth surfacing.
func timeoutNotice(what string) {
	fmt.Fprintf(os.Stderr, "notice: Studio Pro timed out answering %s, but the change was verified applied — continuing\n", what)
}

// timeoutUnverified wraps a timeout whose operation could not be confirmed
// applied, with the guidance that prevents the duplicate-element trap.
func timeoutUnverified(err error, what string) error {
	return fmt.Errorf("%w — Studio Pro timed out applying %s and it could not be verified as applied; "+
		"check in Studio Pro (and save) before re-running — a blind re-run can duplicate model elements", err, what)
}

// pedDocumentExists reports whether moduleName.docName of docType exists in
// Studio Pro's in-memory model, via ped_find_document.
func (b *Backend) pedDocumentExists(moduleName, docType, docName string) (bool, error) {
	res, err := b.client.CallTool("ped_find_document", map[string]any{
		"moduleName":   moduleName,
		"documentType": docType,
	})
	if err != nil {
		return false, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError {
		return false, fmt.Errorf("ped_find_document %s: %s", moduleName, text)
	}
	var doc struct {
		FoundDocuments []struct {
			QualifiedName string `json:"qualifiedName"`
		} `json:"foundDocuments"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return false, fmt.Errorf("ped_find_document %s: unexpected result: %s", moduleName, text)
	}
	want := moduleName + "." + docName
	for _, d := range doc.FoundDocuments {
		if d.QualifiedName == want {
			return true, nil
		}
	}
	return false, nil
}

// domainModelHasEntity reports whether the module's live domain model contains
// an entity with the given name, via a shallow /entities read.
func (b *Backend) domainModelHasEntity(moduleName, entityName string) (bool, error) {
	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        []string{"/entities"},
	})
	if err != nil {
		return false, err
	}
	text := pedStripReminder(res.Text)
	if res.IsError || strings.HasPrefix(strings.TrimSpace(text), "ERROR") {
		return false, fmt.Errorf("ped_read_document %s: %s", moduleName, text)
	}
	var doc struct {
		Results []struct {
			Result []struct {
				QualifiedName string `json:"$QualifiedName"`
			} `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(text), &doc); err != nil {
		return false, fmt.Errorf("ped_read_document %s: unexpected result: %s", moduleName, text)
	}
	want := moduleName + "." + entityName
	for _, r := range doc.Results {
		for _, e := range r.Result {
			if e.QualifiedName == want {
				return true, nil
			}
		}
	}
	return false, nil
}

// pedUpdateVerify is pedUpdate for non-idempotent domain-model operations that
// can be independently verified: on a server timeout it waits, runs the probe,
// and treats a confirmed-applied operation as success instead of failing a
// write that landed.
func (b *Backend) pedUpdateVerify(moduleName, what string, verify func() (bool, error), ops ...pedOpEntry) error {
	err := b.pedUpdateDoc(domainModelDocType, moduleName, ops...)
	if isTimeoutErr(err) && verify != nil {
		time.Sleep(timeoutVerifyDelay)
		if ok, verr := verify(); verr == nil && ok {
			timeoutNotice(what)
			b.markDirty(moduleName)
			return nil
		}
		return timeoutUnverified(err, what)
	}
	if err != nil {
		return err
	}
	b.markDirty(moduleName)
	return nil
}
