// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"github.com/mendixlabs/mxcli/model"
)

const viewEntitySourceDocType = "DomainModels$ViewEntitySourceDocument"

// CreateViewEntitySourceDocument creates the OQL source document that backs a
// view entity, then sets its query. The companion view entity (created
// afterwards by the executor via CreateEntity) references this document by
// qualified name through its OqlViewEntitySource. By convention the source
// document's name equals the view entity's name.
//
// Choreography (verified live): ped_create_document {name} -> ped_update_document
// set /oql. No ped_check_errors here: the source document is "unlinked" until
// the entity referencing it exists, so validation runs after the entity write.
func (b *Backend) CreateViewEntitySourceDocument(moduleID model.ID, moduleName, docName, oqlQuery, documentation string) (model.ID, error) {
	if err := b.ensureSchema(viewEntitySourceDocType); err != nil {
		return "", err
	}
	if err := b.pedCreateDocument(moduleName, viewEntitySourceDocType, docName, map[string]any{"name": docName}, ""); err != nil {
		return "", err
	}
	qn := moduleName + "." + docName
	ops := []pedOpEntry{{Path: "/oql", Operation: pedOperation{Type: "set", Value: oqlQuery}}}
	if documentation != "" {
		ops = append(ops, pedOpEntry{Path: "/documentation", Operation: pedOperation{Type: "set", Value: documentation}})
	}
	if err := b.pedUpdateDoc(viewEntitySourceDocType, qn, ops...); err != nil {
		return "", err
	}
	return model.ID("mcp~vesrc~" + qn), nil
}

// DeleteViewEntitySourceDocumentByName is a no-op: the PED server exposes no
// delete-document tool. The executor calls this before every
// CreateViewEntitySourceDocument; for a new view entity there is nothing to
// delete, so a no-op lets CREATE proceed. Re-creating an existing view entity
// fails at ped_create_document (duplicate document) with a clear error.
func (b *Backend) DeleteViewEntitySourceDocumentByName(_ string, _ string) error {
	return nil
}
