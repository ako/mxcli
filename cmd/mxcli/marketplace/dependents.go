// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"fmt"
	"sort"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/model"
	"go.mongodb.org/mongo-driver/bson"
)

// DependentAssociation is a cross-module association another module owns that
// points into the module being updated, captured as stored.
//
// The update has to carry these across itself. DROP MODULE deletes the module's
// entities, and DeleteEntity cascades into every other domain model to remove
// the cross-module associations that reach a deleted entity by name — so the
// local module loses an association it owns, and its access rules keep a
// by-name MemberAccess to it that Studio Pro reports as missing
// (mendixlabs/mxcli#1502). Re-creating it from MDL afterwards is not
// equivalent: that mints a new GUID, and the runtime keys an association's
// stored links on its GUID — measured, a re-created association comes back
// with every link NULL (docs/11-proposals/data/marketplace-upgrade/
// GUID_IDENTITY.md, "Associations"). Putting back the stored element keeps the
// $ID, the GUID and every property as they were, and the links with them.
type DependentAssociation struct {
	// UnitID is the owning domain model unit; its module is not the one being
	// updated, so its $IDs survive the update untouched — including the FROM
	// entity the element's ParentPointer holds.
	UnitID string
	// Qualified is Module.Name in the owning module, for reporting.
	Qualified string
	// Target is the TO entity, Module.Entity — the stored `Child`.
	Target string
	// Element is the stored DomainModels$CrossAssociation, verbatim.
	Element bson.D
}

// CaptureDependentAssociations records every cross-module association another
// module owns whose TO end is an entity of moduleName. Read before the module
// is dropped, since the drop is what removes them.
func CaptureDependentAssociations(mprPath, moduleName string) ([]DependentAssociation, error) {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", mprPath, err)
	}
	defer reader.Disconnect()

	modules, err := reader.ListModules()
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}
	moduleNames := make(map[model.ID]string, len(modules))
	for _, m := range modules {
		moduleNames[m.ID] = m.Name
	}
	units, err := reader.ListRawUnitsByType("DomainModels$DomainModel")
	if err != nil {
		return nil, fmt.Errorf("read domain models: %w", err)
	}

	prefix := moduleName + "."
	var out []DependentAssociation
	for _, u := range units {
		owner := moduleNames[u.ContainerID]
		if strings.EqualFold(owner, moduleName) {
			continue // the module's own: replaced with it, by design
		}
		var doc bson.D
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			return nil, fmt.Errorf("decode domain model %s: %w", u.ID, err)
		}
		for _, x := range crossAssociationsOf(doc) {
			ca, ok := x.(bson.D)
			if !ok {
				continue
			}
			m := ca.Map()
			child, _ := m["Child"].(string)
			if !strings.HasPrefix(child, prefix) {
				continue
			}
			name, _ := m["Name"].(string)
			out = append(out, DependentAssociation{
				UnitID:    string(u.ID),
				Qualified: owner + "." + name,
				Target:    child,
				Element:   ca,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Qualified < out[j].Qualified })
	return out, nil
}

// RestoreDependentAssociations puts captured associations back into their
// owning domain models after the module has been replaced.
//
// An association is restored only when moduleName still has its TO entity:
// pointing it at an entity the new version removed would be a by-name
// reference to nothing (CE1613). Those are returned as lost, with the reason.
// One still present — nothing removed it — is left alone and counted as kept.
//
// The element goes back byte-for-byte as it was read, so nothing about it is
// new to the stored model: its $ID and GUID are the ones the project already
// had, and the owning unit is written only when something was missing from it.
func RestoreDependentAssociations(mprPath, moduleName string, deps []DependentAssociation) (kept, lost []string, err error) {
	if len(deps) == 0 {
		return nil, nil, nil
	}
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open %s: %w", mprPath, err)
	}

	entities, err := entitiesOfModule(reader, moduleName)
	if err != nil {
		reader.Disconnect()
		return nil, nil, err
	}

	byUnit := map[string][]DependentAssociation{}
	var unitOrder []string
	for _, d := range deps {
		if _, seen := byUnit[d.UnitID]; !seen {
			unitOrder = append(unitOrder, d.UnitID)
		}
		byUnit[d.UnitID] = append(byUnit[d.UnitID], d)
	}

	type pending struct {
		id       string
		contents []byte
	}
	var writes []pending
	for _, unitID := range unitOrder {
		raw, rerr := reader.GetRawUnitBytes(model.ID(unitID))
		if rerr != nil {
			reader.Disconnect()
			return nil, nil, fmt.Errorf("read domain model %s: %w", unitID, rerr)
		}
		var doc bson.D
		if err := bson.Unmarshal(raw, &doc); err != nil {
			reader.Disconnect()
			return nil, nil, fmt.Errorf("decode domain model %s: %w", unitID, err)
		}
		present := map[string]bool{}
		for _, x := range crossAssociationsOf(doc) {
			if ca, ok := x.(bson.D); ok {
				if n, _ := ca.Map()["Name"].(string); n != "" {
					present[n] = true
				}
			}
		}

		var add bson.A
		for _, d := range byUnit[unitID] {
			name, _ := d.Element.Map()["Name"].(string)
			switch {
			case present[name]:
				kept = append(kept, d.Qualified)
			case !entities[strings.TrimPrefix(d.Target, moduleName+".")]:
				lost = append(lost, fmt.Sprintf("%s (%s is not in the new version)", d.Qualified, d.Target))
			default:
				add = append(add, d.Element)
				kept = append(kept, d.Qualified)
			}
		}
		if len(add) == 0 {
			continue
		}
		if !appendCrossAssociations(doc, add) {
			reader.Disconnect()
			return nil, nil, fmt.Errorf("domain model %s has no CrossAssociations list", unitID)
		}
		enc, merr := bson.Marshal(doc)
		if merr != nil {
			reader.Disconnect()
			return nil, nil, fmt.Errorf("re-encode domain model %s: %w", unitID, merr)
		}
		writes = append(writes, pending{unitID, enc})
	}
	reader.Disconnect()

	if len(writes) > 0 {
		writer, werr := modelsdk.OpenForWriting(mprPath)
		if werr != nil {
			return nil, nil, fmt.Errorf("open %s for writing: %w", mprPath, werr)
		}
		defer writer.Disconnect()
		for _, w := range writes {
			if err := writer.UpdateRawUnit(w.id, w.contents); err != nil {
				return nil, nil, fmt.Errorf("restore associations into domain model %s: %w", w.id, err)
			}
		}
	}
	sort.Strings(kept)
	sort.Strings(lost)
	return kept, lost, nil
}

// entitiesOfModule returns the names of the entities in a module's domain model.
func entitiesOfModule(reader backend.FullBackend, moduleName string) (map[string]bool, error) {
	modules, err := reader.ListModules()
	if err != nil {
		return nil, fmt.Errorf("list modules: %w", err)
	}
	for _, m := range modules {
		if !strings.EqualFold(m.Name, moduleName) {
			continue
		}
		dm, err := reader.GetDomainModel(m.ID)
		if err != nil {
			return nil, fmt.Errorf("read the domain model of %s: %w", moduleName, err)
		}
		out := map[string]bool{}
		for _, e := range dm.Entities {
			out[e.Name] = true
		}
		return out, nil
	}
	return nil, fmt.Errorf("module %q not found after the update", moduleName)
}

// crossAssociationsOf returns a domain model's stored CrossAssociations list,
// including its leading array-kind marker.
func crossAssociationsOf(doc bson.D) bson.A {
	for _, e := range doc {
		if e.Key == "CrossAssociations" {
			if a, ok := e.Value.(bson.A); ok {
				return a
			}
		}
	}
	return nil
}

// appendCrossAssociations appends elements to the stored list in place.
func appendCrossAssociations(doc bson.D, add bson.A) bool {
	for i, e := range doc {
		if e.Key == "CrossAssociations" {
			a, ok := e.Value.(bson.A)
			if !ok {
				return false
			}
			doc[i].Value = append(append(bson.A{}, a...), add...)
			return true
		}
	}
	return false
}
