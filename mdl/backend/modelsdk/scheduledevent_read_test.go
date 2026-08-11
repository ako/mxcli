// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"
	"time"

	"github.com/mendixlabs/mxcli/model"
)

// TestListScheduledEvents_Empty confirms the read returns an empty (not error)
// result on a project with no scheduled events — the minimal fixture — so SHOW
// STRUCTURE no longer swallows a not-implemented error.
func TestListScheduledEvents_Empty(t *testing.T) {
	b := New()
	if err := b.Connect(fixture); err != nil {
		t.Fatalf("connect(%s): %v", fixture, err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	events, err := b.ListScheduledEvents()
	if err != nil {
		t.Fatalf("ListScheduledEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("got %d scheduled events, want 0 (minimal fixture has none)", len(events))
	}
}

// TestScheduledEventRoundTrip writes an event and reads it back through the real
// backend. A round trip (rather than a reader-only test on hand-written BSON) is
// what catches a reader keyed on different property names than the writer
// produces — the failure mode recorded for the workflow-activity reads.
func TestScheduledEventRoundTrip(t *testing.T) {
	proj := copyFixture(t)
	b := New()
	if err := b.Connect(proj); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = b.Disconnect() })

	mod, err := b.GetModuleByName("MyFirstModule")
	if err != nil || mod == nil {
		t.Fatalf("GetModuleByName: %v", err)
	}
	start := time.Date(2026, 1, 1, 4, 0, 0, 0, time.UTC)
	want := &model.ScheduledEvent{
		ContainerID:   mod.ID,
		Name:          "ZzNightly",
		Documentation: "nightly cleanup",
		MicroflowID:   "MyFirstModule.DoCleanup",
		StartDateTime: &start,
		TimeZone:      "Server",
		OnOverlap:     "SkipNext",
		Interval:      1,
		IntervalType:  "Day",
		Enabled:       true,
		Schedule: &model.Schedule{
			Kind: model.ScheduleMonthWeekday, Multiplier: 3, MonthOffset: 2,
			DaySelector: "Last", Weekday: "Friday", HourOfDay: 18, MinuteOfHour: 30,
		},
	}
	if err := b.CreateScheduledEvent(want); err != nil {
		t.Fatalf("CreateScheduledEvent: %v", err)
	}

	b2 := New()
	if err := b2.Connect(proj); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	t.Cleanup(func() { _ = b2.Disconnect() })

	events, err := b2.ListScheduledEvents()
	if err != nil {
		t.Fatalf("ListScheduledEvents: %v", err)
	}
	for _, got := range events {
		if got.Name != "ZzNightly" {
			continue
		}
		if got.MicroflowID != want.MicroflowID {
			t.Errorf("MicroflowID = %q", got.MicroflowID)
		}
		if got.Documentation != want.Documentation {
			t.Errorf("Documentation = %q", got.Documentation)
		}
		if got.TimeZone != "Server" || got.OnOverlap != "SkipNext" {
			t.Errorf("TimeZone/OnOverlap = %q/%q", got.TimeZone, got.OnOverlap)
		}
		if got.Interval != 1 || got.IntervalType != "Day" {
			t.Errorf("legacy pair = %d/%q", got.Interval, got.IntervalType)
		}
		if !got.Enabled {
			t.Error("Enabled did not round-trip")
		}
		if got.StartDateTime == nil || !got.StartDateTime.Equal(start) {
			t.Errorf("StartDateTime = %v, want %v", got.StartDateTime, start)
		}
		if got.Schedule == nil {
			t.Fatal("Schedule did not round-trip")
		}
		if *got.Schedule != *want.Schedule {
			t.Errorf("Schedule = %+v, want %+v", *got.Schedule, *want.Schedule)
		}
		return
	}
	t.Fatalf("ZzNightly not found after create (got %d events)", len(events))
}
