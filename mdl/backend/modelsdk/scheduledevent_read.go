// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	sched "github.com/mendixlabs/mxcli/mdl/scheduledevents"
	"github.com/mendixlabs/mxcli/model"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

// Scheduled events. Used by SHOW STRUCTURE (per-module counts), the project
// tree, and SHOW/DESCRIBE/CREATE/DROP SCHEDULED EVENT.
//
// The document is read and written as raw BSON through mdl/scheduledevents
// rather than through modelsdk/gen, because gen's generated types disagree with
// what Studio Pro actually writes on two properties: every integer is stored as
// int64 while gen declares int32 (the mismatch behind issue #585), and
// StartDateTime is a BSON UTC datetime while gen declares a string.

func (b *Backend) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	units, err := b.reader.ListRawUnitsByType(sched.TypeName)
	if err != nil {
		return nil, err
	}
	out := make([]*model.ScheduledEvent, 0, len(units))
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			return nil, fmt.Errorf("unmarshal scheduled event %s: %w", u.ID, err)
		}
		out = append(out, sched.Parse(doc, model.ID(u.ID), model.ID(u.ContainerID)))
	}
	return out, nil
}

func (b *Backend) GetScheduledEvent(id model.ID) (*model.ScheduledEvent, error) {
	events, err := b.ListScheduledEvents()
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if ev.ID == id {
			return ev, nil
		}
	}
	return nil, fmt.Errorf("scheduled event not found: %s", id)
}

// CreateScheduledEvent inserts a new scheduled event document.
func (b *Backend) CreateScheduledEvent(ev *model.ScheduledEvent) error {
	if ev == nil {
		return fmt.Errorf("CreateScheduledEvent: nil event")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateScheduledEvent: not connected for writing")
	}
	if ev.ID == "" {
		ev.ID = model.ID(mmpr.GenerateID())
	}
	contents, err := sched.Serialize(ev)
	if err != nil {
		return err
	}
	return b.writer.InsertUnit(string(ev.ID), string(ev.ContainerID), "Documents", sched.TypeName, contents)
}

// UpdateScheduledEvent rewrites an existing scheduled event in place.
func (b *Backend) UpdateScheduledEvent(ev *model.ScheduledEvent) error {
	if ev == nil {
		return fmt.Errorf("UpdateScheduledEvent: nil event")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateScheduledEvent: not connected for writing")
	}
	contents, err := sched.Serialize(ev)
	if err != nil {
		return err
	}
	return b.writer.UpdateRawUnit(string(ev.ID), contents)
}

// DeleteScheduledEvent removes a scheduled event unit by ID.
func (b *Backend) DeleteScheduledEvent(id string) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteScheduledEvent: not connected for writing")
	}
	return b.writer.DeleteUnit(id)
}
