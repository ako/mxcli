// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"fmt"
	"strings"
	"time"
)

// The document types DROP addresses that no create path names.
const (
	nanoflowDocType             = "Microflows$Nanoflow"
	pageDocType                 = "Pages$Page"
	businessEventServiceDocType = "BusinessEvents$BusinessEventService"
)

// deleteDocument drops a standalone named document (constant, enumeration,
// microflow, nanoflow, page, workflow, business-event service).
//
// Studio Pro 11.16 added ped_delete_document, so a server that advertises it is
// asked directly; older servers have no delete tool and need Concord. A native
// failure falls back to Concord when one is connected: on 11.16.0 the tool fails
// for every call made outside Studio Pro's own agent session ("Session
// 'mcp-server' not found"), measured live.
func (b *Backend) deleteDocument(docType, moduleName, docName string) error {
	if b.client == nil || !b.client.HasTool("ped_delete_document") {
		return b.concordDeleteDocument(moduleName, docName)
	}
	err := b.pedDeleteDocument(docType, moduleName, docName)
	if err == nil || b.concord == nil {
		return err
	}
	if cerr := b.concordDeleteDocument(moduleName, docName); cerr != nil {
		return fmt.Errorf("%w; Concord fallback: %v", err, cerr)
	}
	return nil
}

// pedDeleteDocument calls ped_delete_document for one document. Its success
// reply has not been observed live, so a reply that is neither PED's usual
// "SUCCESS…" nor an error is settled by looking: the delete happened iff the
// document is gone. A timeout is settled the same way, as for creates.
func (b *Backend) pedDeleteDocument(docType, moduleName, docName string) error {
	target := moduleName + "." + docName
	res, err := b.client.CallTool("ped_delete_document", map[string]any{
		"documents": []map[string]any{{
			"documentType": docType,
			"moduleName":   moduleName,
			"documentName": docName,
		}},
	})
	if isTimeoutErr(err) {
		time.Sleep(timeoutVerifyDelay)
		if exists, verr := b.pedDocumentExists(moduleName, docType, docName); verr == nil && !exists {
			timeoutNotice(fmt.Sprintf("%s %s", docType, target))
			return nil
		}
		return timeoutUnverified(err, fmt.Sprintf("%s %s", docType, target))
	}
	if err != nil {
		return err
	}
	opErr := pedOpError("ped_delete_document", target, res)
	if opErr == nil || res.IsError || pedReportsFailure(res.Text) {
		return opErr
	}
	exists, verr := b.pedDocumentExists(moduleName, docType, docName)
	if verr != nil {
		return fmt.Errorf("%w (and confirming the delete failed: %v)", opErr, verr)
	}
	if exists {
		return opErr
	}
	return nil
}

// pedReportsFailure recognises PED's failure texts, which arrive with
// isError:false ("Deleting documents failed (1 of 1):\nERROR: …").
func pedReportsFailure(text string) bool {
	return strings.Contains(text, "failed (") || strings.Contains(text, "\nERROR:") || strings.HasPrefix(strings.TrimSpace(text), "ERROR")
}
