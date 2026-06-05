// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// markDirty records that a module's live model has diverged from disk. Called
// after every successful PED write so subsequent reads of that module route to
// the live reconstruction instead of the stale local reader.
func (b *Backend) markDirty(moduleName string) {
	if b.dirty == nil {
		b.dirty = map[string]bool{}
	}
	b.dirty[moduleName] = true
}

func synthEntityID(module, name string) model.ID {
	return model.ID("mcp~ent~" + module + "~" + name)
}

func synthAssocID(module, name string) model.ID {
	return model.ID("mcp~assoc~" + module + "~" + name)
}

// syntheticName resolves a synthetic ID handed out by a reconstructed read back
// to its PED-addressable element name.
func (b *Backend) syntheticName(id model.ID) (string, bool) {
	n, ok := b.synthetic[id]
	return n, ok
}

func (b *Backend) registerSynthetic(id model.ID, name string) {
	if b.synthetic == nil {
		b.synthetic = map[model.ID]string{}
	}
	b.synthetic[id] = name
}

// pedIDRef matches a PED path reference like "$id(/entities/3)".
var pedIDRef = regexp.MustCompile(`^\$id\(/entities/(\d+)\)$`)

// reconstructDomainModel builds a *domainmodel.DomainModel for a dirty module
// from Studio Pro's live in-memory model (read over MCP), assigning synthetic
// IDs to entities and associations.
//
// Fidelity note: PED reads expose attribute NAMES (parsed from $QualifiedName)
// but not their primitive types — recovering a type costs a read per attribute.
// The reconstruction therefore uses placeholder attribute types. That is
// sufficient for the operations the slice routes through it (ALTER attribute
// add/drop and DROP, which key on names), but DESCRIBE of an in-session-edited
// entity will not show precise attribute types until the project is saved.
func (b *Backend) reconstructDomainModel(moduleName string, moduleID model.ID) (*domainmodel.DomainModel, error) {
	// The domain-model document itself always exists on disk (nameless), so the
	// local reader gives us its real ID/container even when entities diverged.
	localDM, err := b.reader.GetDomainModel(moduleID)
	if err != nil {
		return nil, fmt.Errorf("reconstruct %s: local domain model: %w", moduleName, err)
	}

	res, err := b.client.CallTool("ped_read_document", map[string]any{
		"documentType": domainModelDocType,
		"documentName": moduleName,
		"paths":        []string{"/entities", "/associations"},
	})
	if err != nil {
		return nil, err
	}
	if res.IsError {
		return nil, fmt.Errorf("reconstruct %s: %s", moduleName, res.Text)
	}

	var doc struct {
		Results []struct {
			Path   string          `json:"path"`
			Result json.RawMessage `json:"result"`
		} `json:"results"`
	}
	if err := json.Unmarshal([]byte(res.Text), &doc); err != nil {
		return nil, fmt.Errorf("reconstruct %s: parse: %w", moduleName, err)
	}

	dm := &domainmodel.DomainModel{
		ContainerID: localDM.ContainerID,
	}
	dm.ID = localDM.ID

	var entityOrder []string // index -> entity synthetic ID (for $id(/entities/N) refs)

	for _, r := range doc.Results {
		switch r.Path {
		case "/entities":
			ents, order, err := b.reconstructEntities(moduleName, dm.ID, r.Result)
			if err != nil {
				return nil, err
			}
			dm.Entities = ents
			entityOrder = order
		case "/associations":
			assocs, err := b.reconstructAssociations(moduleName, r.Result, entityOrder)
			if err != nil {
				return nil, err
			}
			dm.Associations = assocs
		}
	}
	return dm, nil
}

func (b *Backend) reconstructEntities(moduleName string, dmID model.ID, raw json.RawMessage) ([]*domainmodel.Entity, []string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	var pedEntities []struct {
		Name       string `json:"name"`
		Attributes []struct {
			QualifiedName string `json:"$QualifiedName"`
		} `json:"attributes"`
	}
	if err := json.Unmarshal(raw, &pedEntities); err != nil {
		return nil, nil, fmt.Errorf("reconstruct %s entities: %w", moduleName, err)
	}

	entities := make([]*domainmodel.Entity, 0, len(pedEntities))
	order := make([]string, 0, len(pedEntities))
	for _, pe := range pedEntities {
		id := synthEntityID(moduleName, pe.Name)
		b.registerSynthetic(id, pe.Name)

		e := &domainmodel.Entity{
			ContainerID: dmID,
			Name:        pe.Name,
			Persistable: true, // unknown via PED; default (and the slice only creates persistent)
		}
		e.ID = id
		for _, pa := range pe.Attributes {
			e.Attributes = append(e.Attributes, &domainmodel.Attribute{
				ContainerID: id,
				Name:        lastSegment(pa.QualifiedName),
				// Placeholder type — see fidelity note on reconstructDomainModel.
				Type: &domainmodel.StringAttributeType{},
			})
		}
		entities = append(entities, e)
		order = append(order, string(id))
	}
	return entities, order, nil
}

func (b *Backend) reconstructAssociations(moduleName string, raw json.RawMessage, entityOrder []string) ([]*domainmodel.Association, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var pedAssocs []struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Owner  string `json:"owner"`
		Parent string `json:"parent"`
		Child  string `json:"child"`
	}
	if err := json.Unmarshal(raw, &pedAssocs); err != nil {
		return nil, fmt.Errorf("reconstruct %s associations: %w", moduleName, err)
	}

	assocs := make([]*domainmodel.Association, 0, len(pedAssocs))
	for _, pa := range pedAssocs {
		id := synthAssocID(moduleName, pa.Name)
		b.registerSynthetic(id, pa.Name)

		a := &domainmodel.Association{
			Name:     pa.Name,
			Type:     domainmodel.AssociationType(pa.Type),
			Owner:    domainmodel.AssociationOwner(pa.Owner),
			ParentID: resolveEntityRef(pa.Parent, entityOrder),
			ChildID:  resolveEntityRef(pa.Child, entityOrder),
		}
		a.ID = id
		assocs = append(assocs, a)
	}
	return assocs, nil
}

// resolveEntityRef turns a PED "$id(/entities/N)" reference into the synthetic
// ID of the N-th reconstructed entity.
func resolveEntityRef(ref string, entityOrder []string) model.ID {
	m := pedIDRef.FindStringSubmatch(ref)
	if m == nil {
		return ""
	}
	idx, err := strconv.Atoi(m[1])
	if err != nil || idx < 0 || idx >= len(entityOrder) {
		return ""
	}
	return model.ID(entityOrder[idx])
}

func lastSegment(qualifiedName string) string {
	if i := strings.LastIndex(qualifiedName, "."); i >= 0 {
		return qualifiedName[i+1:]
	}
	return qualifiedName
}
