// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	sched "github.com/mendixlabs/mxcli/mdl/scheduledevents"
	"github.com/mendixlabs/mxcli/model"
)

// Scheduled events (ScheduledEvents$ScheduledEvent) for the legacy engine.
//
// The document shape lives in mdl/scheduledevents so both engines write exactly
// the same bytes — the schedule child has eight variants that differ in which
// fields they carry, and two copies of that dispatch would eventually disagree.
//
// The legacy READER for scheduled events is parseScheduledEvent (in
// parser_enumeration.go), which predates this and covers only the flat legacy
// fields; the write path here round-trips through the shared codec.

// CreateScheduledEvent inserts a new scheduled event document.
func (w *Writer) CreateScheduledEvent(ev *model.ScheduledEvent) error {
	if ev == nil {
		return fmt.Errorf("CreateScheduledEvent: nil event")
	}
	if ev.ID == "" {
		ev.ID = model.ID(generateUUID())
	}
	contents, err := sched.Serialize(ev)
	if err != nil {
		return err
	}
	return w.insertUnit(string(ev.ID), string(ev.ContainerID), "Documents", sched.TypeName, contents)
}

// UpdateScheduledEvent rewrites an existing scheduled event in place.
func (w *Writer) UpdateScheduledEvent(ev *model.ScheduledEvent) error {
	if ev == nil {
		return fmt.Errorf("UpdateScheduledEvent: nil event")
	}
	contents, err := sched.Serialize(ev)
	if err != nil {
		return err
	}
	return w.UpdateRawUnit(string(ev.ID), contents)
}

// DeleteScheduledEvent removes a scheduled event by ID.
func (w *Writer) DeleteScheduledEvent(id string) error {
	return w.deleteUnit(id)
}
