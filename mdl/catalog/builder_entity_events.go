// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
)

// Catalog rows and reference edges for entity event handlers.
//
// An entity event handler runs a microflow on every create/commit/delete/
// rollback of its entity, which makes the handler microflow one of the most
// reachable documents in an app — and, before this builder, one of the few with
// no inbound edge at all. CATALOG.ENTITIES.HasEventHandlers reduced the whole
// list to "some exist", the same shape NavigationProfile.OfflineEntityCount had
// before CATALOG.OFFLINE_ENTITY_CONFIGS, so the two halves land together here:
// a row per handler, and an ENTITY -> MICROFLOW edge per handler.

// eventHandlerRef is one entity → microflow edge for an event handler, held
// until buildReferences (a later pass) emits it.
type eventHandlerRef struct{ entityQualifiedName, moduleName, microflow string }

// entityEventHandlerRow is one row of entity_event_handlers_data, plus the
// fields the edge needs. Built by a pure function so the mapping — and the
// skips — are testable without a project.
type entityEventHandlerRow struct {
	id                  string
	entityID            string
	entityQualifiedName string
	moduleName          string
	moment              string
	event               string
	microflow           string
	raiseErrorOnFalse   bool
	passEventObject     bool
}

// entityEventHandlerRows maps one entity's handlers to catalog rows.
//
// A handler naming no microflow is skipped: it is a hole in the document, not a
// reference, and a row for it would put an empty TargetName in refs — which
// matches nothing and counts as an edge in every aggregate.
func entityEventHandlerRows(entity *domainmodel.Entity, moduleName string) []entityEventHandlerRow {
	if entity == nil {
		return nil
	}
	entityQN := moduleName + "." + entity.Name
	rows := make([]entityEventHandlerRow, 0, len(entity.EventHandlers))
	for _, eh := range entity.EventHandlers {
		if eh == nil || eh.MicroflowName == "" {
			continue
		}
		rows = append(rows, entityEventHandlerRow{
			id:                  string(eh.ID),
			entityID:            string(entity.ID),
			entityQualifiedName: entityQN,
			moduleName:          moduleName,
			moment:              string(eh.Moment),
			event:               string(eh.Event),
			microflow:           eh.MicroflowName,
			raiseErrorOnFalse:   eh.RaiseErrorOnFalse,
			passEventObject:     eh.PassEventObject,
		})
	}
	return rows
}

// buildEntityEventHandlers catalogs every entity's event handlers and collects
// the edges buildReferences emits.
func (b *Builder) buildEntityEventHandlers() error {
	domainModels, err := b.cachedDomainModels()
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(`
		INSERT INTO entity_event_handlers_data
			(Id, EntityId, EntityQualifiedName, ModuleName, Moment, Event,
			 Microflow, RaiseErrorOnFalse, PassEventObject, ProjectId, SnapshotId)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, dm := range domainModels {
		moduleID := b.hierarchy.findModuleID(dm.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		for _, entity := range dm.Entities {
			for _, row := range entityEventHandlerRows(entity, moduleName) {
				if _, err := stmt.Exec(
					row.id, row.entityID, row.entityQualifiedName, row.moduleName,
					row.moment, row.event, row.microflow,
					boolToInt(row.raiseErrorOnFalse), boolToInt(row.passEventObject),
					projectID, snapshotID,
				); err != nil {
					return err
				}
				b.eventHandlerRefs = append(b.eventHandlerRefs, eventHandlerRef{
					entityQualifiedName: row.entityQualifiedName,
					moduleName:          row.moduleName,
					microflow:           row.microflow,
				})
				count++
			}
		}
	}

	b.report("Entity Event Handlers", count)
	return nil
}
