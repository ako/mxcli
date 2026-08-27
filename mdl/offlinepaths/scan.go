// SPDX-License-Identifier: Apache-2.0

// Package offlinepaths finds the attribute bindings an offline navigation
// profile would reject.
//
// CE6206: "Attribute paths with multiple steps cannot be used on pages that are
// accessible through an offline-based navigation." One association hop is
// allowed, two are not — measured in Studio Pro 11.14 on a page carrying both,
// where only the second is flagged:
//
//	MaintenanceRequest_Asset → AssetName               1 step   accepted
//	MaintenanceRequest_Asset → Asset_Site → SiteName   2 steps  CE6206
//
// The point of scanning the STORED documents, rather than only the script being
// executed, is that this error bites at a distance: the pages were valid, and
// adding an offline profile is what invalidates them. Someone who runs
// `create or replace navigation TabletOffline` against a working app gets a
// clean statement and a build that has just acquired errors in pages the
// statement never mentioned. That is the whole finding from ako/mxcli-maintenance.
package offlinepaths

import (
	"fmt"
	"sort"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// Finding is one attribute binding that crosses more than one association.
type Finding struct {
	Document string // qualified name, e.g. "Maintenance.Request_Overview"
	Kind     string // "page" or "snippet"
	Path     string // the attribute as stored, e.g. "Maintenance.Site.SiteName"
	Steps    int    // association hops; always >= 2
}

// Source is the slice of a backend this scan needs: which documents exist, and
// what they store.
//
// It is declared as its own narrow interface rather than taking the full backend
// so that the dependency stays visible and a test can drive the scan without a
// project. Both methods are already on the backend interface — this adds no new
// data access, it composes what is there (ADR-0002).
type Source interface {
	ListPages() ([]*pages.Page, error)
	ListSnippets() ([]*pages.Snippet, error)
	GetRawUnitBytes(id model.ID) ([]byte, error)
}

// QualifyFunc turns a document's container and bare name into the name a user
// would recognise ("Maintenance.Request_Overview").
//
// A page does not carry its module — module membership is a containment
// question the caller already has a hierarchy for — so rather than rebuild that
// here, the caller supplies the answer. A nil QualifyFunc falls back to the bare
// name, which keeps the scan usable in a test that has no hierarchy.
type QualifyFunc func(containerID model.ID, name string) string

// Scan returns every multi-step attribute binding in the project's pages and
// snippets, sorted by document and then by path.
//
// A document that cannot be read is skipped rather than failing the scan: this
// feeds a warning, and a warning that aborts the statement it is advising on
// would be worse than one that is silent about a document it could not open.
func Scan(src Source, qualify QualifyFunc) ([]Finding, error) {
	if src == nil {
		return nil, nil
	}
	if qualify == nil {
		qualify = func(_ model.ID, name string) string { return name }
	}
	pageList, err := src.ListPages()
	if err != nil {
		return nil, fmt.Errorf("list pages: %w", err)
	}
	snippetList, err := src.ListSnippets()
	if err != nil {
		return nil, fmt.Errorf("list snippets: %w", err)
	}

	var out []Finding
	scanOne := func(id model.ID, kind, qualified string) {
		raw, err := src.GetRawUnitBytes(id)
		if err != nil || len(raw) == 0 {
			return
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			return
		}
		for _, f := range ScanDocument(doc) {
			f.Document, f.Kind = qualified, kind
			out = append(out, f)
		}
	}
	for _, p := range pageList {
		if p != nil {
			scanOne(p.ID, "page", qualify(p.ContainerID, p.Name))
		}
	}
	for _, s := range snippetList {
		if s != nil {
			scanOne(s.ID, "snippet", qualify(s.ContainerID, s.Name))
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Document != out[j].Document {
			return out[i].Document < out[j].Document
		}
		return out[i].Path < out[j].Path
	})
	return out, nil
}

// ScanDocument walks a decoded page or snippet and returns its multi-step
// attribute bindings, with Document and Kind left for the caller to fill in.
//
// The walk keys on $Type rather than on a list of property names, which is what
// makes it cover pluggable widgets for free: a DataGrid2 column stores its
// binding as a DomainModels$AttributeRef nested several levels inside a
// CustomWidgets$WidgetValue, at a key path no property table would have listed.
// The same reason means it will keep working for widgets that do not exist yet.
//
// Only AttributeRef is considered. A data source that navigates an association
// (Forms$AssociationSource, a DataView over a reference) is a different element
// and is NOT what CE6206 rejects, so matching every IndirectEntityRef in the
// document would report constructs offline navigation permits.
func ScanDocument(doc bson.D) []Finding {
	var found []Finding
	var walkDoc func(bson.D)
	var walkVal func(any)

	walkVal = func(v any) {
		switch t := v.(type) {
		case bson.D:
			walkDoc(t)
		case bson.A:
			for _, e := range t {
				walkVal(e)
			}
		}
	}
	walkDoc = func(d bson.D) {
		if docType(d) == "DomainModels$AttributeRef" {
			if n := entityRefSteps(fieldOf(d, "EntityRef")); n >= 2 {
				path, _ := fieldOf(d, "Attribute").(string)
				found = append(found, Finding{Path: path, Steps: n})
			}
			// No early return: an AttributeRef is a leaf in practice, but nothing
			// guarantees it, and a missed nested binding is a missed error.
		}
		for _, e := range d {
			walkVal(e.Value)
		}
	}
	walkDoc(doc)
	return found
}

// entityRefSteps counts the association hops an EntityRef navigates. A nil
// EntityRef (an attribute on the widget's own entity) is zero.
//
// The Steps array carries a leading typed-array marker — an int32, not a
// document — so the count is of the element documents, not of the array length.
func entityRefSteps(v any) int {
	ref, ok := v.(bson.D)
	if !ok {
		return 0
	}
	steps, ok := fieldOf(ref, "Steps").(bson.A)
	if !ok {
		return 0
	}
	n := 0
	for _, s := range steps {
		if _, isDoc := s.(bson.D); isDoc {
			n++
		}
	}
	return n
}

func docType(d bson.D) string {
	t, _ := fieldOf(d, "$Type").(string)
	return t
}

func fieldOf(d bson.D, key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}
