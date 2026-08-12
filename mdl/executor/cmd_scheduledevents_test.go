// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/mdl/visitor"
	"github.com/mendixlabs/mxcli/model"
)

func mkScheduledEvent(containerID model.ID, name string, s *model.Schedule) *model.ScheduledEvent {
	ev := &model.ScheduledEvent{
		ContainerID: containerID,
		Name:        name,
		MicroflowID: "Ops.SE_Cleanup",
		Schedule:    s,
		OnOverlap:   "DelayNext",
		TimeZone:    "UTC",
	}
	ev.ID = nextID("sched")
	return ev
}

func intPtr(v int) *int { return &v }

// TestScheduleFromStmt_EachRepeatTakesOnlyItsOwnFields is the load-bearing test:
// the eight ScheduledEvents$*Schedule variants differ in WHICH fields they
// carry, and a merged field set is the shape mxbuild accepts and Studio Pro
// refuses to open.
func TestScheduleFromStmt_EachRepeatTakesOnlyItsOwnFields(t *testing.T) {
	tests := []struct {
		repeat string
		stray  string // a property that belongs to a different repeat
		apply  func(*ast.CreateScheduledEventStmt)
	}{
		{"Daily", "Multiplier", func(s *ast.CreateScheduledEventStmt) { s.Multiplier = intPtr(3) }},
		{"Minutely", "HourOfDay", func(s *ast.CreateScheduledEventStmt) { s.HourOfDay = intPtr(4) }},
		{"Hourly", "DayOfMonth", func(s *ast.CreateScheduledEventStmt) { s.DayOfMonth = intPtr(15) }},
		{"YearlyByDate", "MonthOffset", func(s *ast.CreateScheduledEventStmt) { s.MonthOffset = intPtr(1) }},
		{"MonthlyByDate", "DaySelector", func(s *ast.CreateScheduledEventStmt) { s.DaySelector = "Last" }},
		{"Daily", "Weekdays", func(s *ast.CreateScheduledEventStmt) { s.Weekdays = "Monday" }},
	}
	for _, tt := range tests {
		t.Run(tt.repeat+"/"+tt.stray, func(t *testing.T) {
			s := &ast.CreateScheduledEventStmt{
				Name:      ast.QualifiedName{Module: "Ops", Name: "E"},
				Microflow: "Ops.MF",
				Repeat:    tt.repeat,
			}
			tt.apply(s)
			_, err := scheduleFromStmt(s)
			if err == nil {
				t.Fatalf("%s accepted %s, which belongs to a different repeat", tt.repeat, tt.stray)
			}
			if !strings.Contains(err.Error(), tt.stray) {
				t.Errorf("error should name the stray property:\n%s", err)
			}
		})
	}
}

func TestScheduleFromStmt_RangesAreEnforced(t *testing.T) {
	tests := []struct {
		name   string
		repeat string
		apply  func(*ast.CreateScheduledEventStmt)
	}{
		{"HourOfDay 24", "Daily", func(s *ast.CreateScheduledEventStmt) { s.HourOfDay = intPtr(24) }},
		{"MinuteOfHour 60", "Daily", func(s *ast.CreateScheduledEventStmt) { s.MinuteOfHour = intPtr(60) }},
		{"DayOfMonth 0", "MonthlyByDate", func(s *ast.CreateScheduledEventStmt) { s.DayOfMonth = intPtr(0) }},
		{"Month 13", "YearlyByDate", func(s *ast.CreateScheduledEventStmt) { s.Month = intPtr(13) }},
		{"Multiplier 0", "Minutely", func(s *ast.CreateScheduledEventStmt) { s.Multiplier = intPtr(0) }},
		{"MinuteOffset 60", "Hourly", func(s *ast.CreateScheduledEventStmt) { s.MinuteOffset = intPtr(60) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &ast.CreateScheduledEventStmt{
				Name:      ast.QualifiedName{Module: "Ops", Name: "E"},
				Microflow: "Ops.MF",
				Repeat:    tt.repeat,
			}
			tt.apply(s)
			if _, err := scheduleFromStmt(s); err == nil {
				t.Fatalf("%s was accepted — a schedule that can never fire is worse than a refusal", tt.name)
			}
		})
	}
}

func TestScheduleFromStmt_Defaults(t *testing.T) {
	// An unstated multiplier of 0 would never fire, so it defaults to 1.
	got, err := scheduleFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Microflow: "Ops.MF", Repeat: "Hourly",
	})
	if err != nil {
		t.Fatalf("scheduleFromStmt: %v", err)
	}
	if got.Multiplier != 1 {
		t.Errorf("Multiplier = %d, want 1", got.Multiplier)
	}
	if got.Kind != model.ScheduleHour {
		t.Errorf("Kind = %v", got.Kind)
	}
}

func TestScheduleFromStmt_Weekdays(t *testing.T) {
	got, err := scheduleFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Microflow: "Ops.MF",
		Repeat: "Weekly", Weekdays: "monday, FRIDAY", HourOfDay: intPtr(9),
	})
	if err != nil {
		t.Fatalf("scheduleFromStmt: %v", err)
	}
	want := [7]bool{false, true, false, false, false, true, false}
	if got.Weekdays != want {
		t.Errorf("Weekdays = %v, want %v (case-insensitive names)", got.Weekdays, want)
	}
}

func TestScheduleFromStmt_UnknownWeekday(t *testing.T) {
	_, err := scheduleFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Microflow: "Ops.MF",
		Repeat: "Weekly", Weekdays: "Moonday",
	})
	if err == nil || !strings.Contains(err.Error(), "Moonday") {
		t.Fatalf("expected an unknown-weekday error, got %v", err)
	}
}

// TestScheduledEventFromStmt_EnumCasing checks that a near-miss enum value is
// reported rather than normalised: Mendix stores the exact spelling, and a value
// the enumeration does not have produces a document that loads and misbehaves.
func TestScheduledEventFromStmt_EnumCasing(t *testing.T) {
	_, err := scheduledEventFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Microflow: "Ops.MF",
		Repeat: "Daily", TimeZone: "server",
	})
	if err == nil || !strings.Contains(err.Error(), "Server") {
		t.Fatalf("expected a casing error naming the stored spelling, got %v", err)
	}
}

func TestScheduledEventFromStmt_RequiresMicroflowAndRepeat(t *testing.T) {
	if _, err := scheduledEventFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Repeat: "Daily",
	}); err == nil {
		t.Error("a scheduled event with no Microflow was accepted")
	}
	if _, err := scheduledEventFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Microflow: "Ops.MF",
	}); err == nil {
		t.Error("a scheduled event with no Repeat was accepted")
	}
}

func TestScheduledEventFromStmt_StartDateTime(t *testing.T) {
	ev, err := scheduledEventFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Microflow: "Ops.MF",
		Repeat: "Daily", StartDateTime: "2026-01-01T04:00:00Z",
	})
	if err != nil {
		t.Fatalf("scheduledEventFromStmt: %v", err)
	}
	if ev.StartDateTime == nil || ev.StartDateTime.Year() != 2026 {
		t.Errorf("StartDateTime = %v", ev.StartDateTime)
	}
	if _, err := scheduledEventFromStmt(&ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "E"}, Microflow: "Ops.MF",
		Repeat: "Daily", StartDateTime: "1 January 2026",
	}); err == nil {
		t.Error("a non-RFC-3339 StartDateTime was accepted")
	}
}

// TestLegacyIntervalFor pins the Interval/IntervalType pair a new event gets.
// IntervalType is a Mendix enumeration, so an empty string is not a valid
// "unset" — every Studio Pro-authored event carries a real value.
func TestLegacyIntervalFor(t *testing.T) {
	tests := []struct {
		kind     model.ScheduleKind
		mult     int
		wantN    int
		wantType string
	}{
		{model.ScheduleMinute, 5, 5, "Minute"},
		{model.ScheduleHour, 2, 2, "Hour"},
		{model.ScheduleDay, 0, 1, "Day"},
		{model.ScheduleWeek, 0, 1, "Week"},
		{model.ScheduleMonthDate, 3, 3, "Month"},
		{model.ScheduleYearWeekday, 0, 1, "Year"},
	}
	for _, tt := range tests {
		n, typ := legacyIntervalFor(&model.Schedule{Kind: tt.kind, Multiplier: tt.mult})
		if n != tt.wantN || typ != tt.wantType {
			t.Errorf("%s -> %d/%q, want %d/%q", tt.kind, n, typ, tt.wantN, tt.wantType)
		}
		if typ == "" {
			t.Errorf("%s produced an empty IntervalType — not a value the enumeration has", tt.kind)
		}
	}
}

func TestShowScheduledEvents_Mock(t *testing.T) {
	mod := mkModule("Ops")
	e1 := mkScheduledEvent(mod.ID, "Nightly", &model.Schedule{Kind: model.ScheduleDay, HourOfDay: 4})
	e1.Enabled = true
	e2 := mkScheduledEvent(mod.ID, "Hourly", &model.Schedule{Kind: model.ScheduleHour, Multiplier: 2, MinuteOffset: 23})

	h := mkHierarchy(mod)
	withContainer(h, e1.ContainerID, mod.ID)
	withContainer(h, e2.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListScheduledEventsFunc: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{e1, e2}, nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execShowScheduledEvents(ctx, &ast.ShowScheduledEventsStmt{}))

	out := buf.String()
	assertContainsStr(t, out, "Ops.Nightly")
	assertContainsStr(t, out, "daily at 04:00")
	assertContainsStr(t, out, "every 2h at :23")
	assertContainsStr(t, out, "(2 scheduled event(s))")
}

func TestCreateScheduledEvent_Mock(t *testing.T) {
	mod := mkModule("Ops")
	h := mkHierarchy(mod)

	var created *model.ScheduledEvent
	mb := &mock.MockBackend{
		IsConnectedFunc:         func() bool { return true },
		ListModulesFunc:         func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListScheduledEventsFunc: func() ([]*model.ScheduledEvent, error) { return nil, nil },
		CreateScheduledEventFunc: func(ev *model.ScheduledEvent) error {
			created = ev
			return nil
		},
	}

	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateScheduledEvent(ctx, &ast.CreateScheduledEventStmt{
		Name:         ast.QualifiedName{Module: "Ops", Name: "Nightly"},
		Microflow:    "Ops.SE_Cleanup",
		Repeat:       "Daily",
		HourOfDay:    intPtr(4),
		MinuteOfHour: intPtr(0),
		Enabled:      boolPtr(true),
	}))

	if created == nil {
		t.Fatal("CreateScheduledEvent was not called")
	}
	if created.Schedule == nil || created.Schedule.Kind != model.ScheduleDay {
		t.Fatalf("Schedule = %+v", created.Schedule)
	}
	if created.Schedule.HourOfDay != 4 {
		t.Errorf("HourOfDay = %d", created.Schedule.HourOfDay)
	}
	if created.IntervalType == "" {
		t.Error("IntervalType is empty — it is an enumeration, not an optional string")
	}
	assertContainsStr(t, buf.String(), "Created scheduled event: Ops.Nightly")
}

// TestModifyScheduledEvent_Mock_PreservesLegacyInterval: Studio Pro does not
// keep Interval/IntervalType in sync with Schedule and MDL cannot author them,
// so a modify must carry the stored pair through rather than re-derive it.
func TestModifyScheduledEvent_Mock_PreservesLegacyInterval(t *testing.T) {
	mod := mkModule("Ops")
	existing := mkScheduledEvent(mod.ID, "Nightly", &model.Schedule{Kind: model.ScheduleDay, HourOfDay: 1})
	existing.Interval = 0
	existing.IntervalType = "Minute" // the Workflow Commons shape: stale, but real
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	var updated *model.ScheduledEvent
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListScheduledEventsFunc: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{existing}, nil
		},
		UpdateScheduledEventFunc: func(ev *model.ScheduledEvent) error {
			updated = ev
			return nil
		},
	}

	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execCreateScheduledEvent(ctx, &ast.CreateScheduledEventStmt{
		Name:           ast.QualifiedName{Module: "Ops", Name: "Nightly"},
		Microflow:      "Ops.SE_Cleanup",
		Repeat:         "Daily",
		HourOfDay:      intPtr(6),
		CreateOrModify: true,
	}))

	if updated == nil {
		t.Fatal("UpdateScheduledEvent was not called")
	}
	if updated.ID != existing.ID {
		t.Errorf("ID = %q, want the existing %q", updated.ID, existing.ID)
	}
	if updated.Interval != 0 || updated.IntervalType != "Minute" {
		t.Errorf("legacy pair = %d/%q, want the stored 0/\"Minute\"", updated.Interval, updated.IntervalType)
	}
	if updated.Schedule.HourOfDay != 6 {
		t.Errorf("HourOfDay = %d, want the new 6", updated.Schedule.HourOfDay)
	}
}

func TestCreateScheduledEvent_Mock_DuplicateWithoutOrModify(t *testing.T) {
	mod := mkModule("Ops")
	existing := mkScheduledEvent(mod.ID, "Nightly", &model.Schedule{Kind: model.ScheduleDay})
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListModulesFunc: func() ([]*model.Module, error) { return []*model.Module{mod}, nil },
		ListScheduledEventsFunc: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{existing}, nil
		},
		CreateScheduledEventFunc: func(ev *model.ScheduledEvent) error {
			t.Error("CreateScheduledEvent must not be called for a duplicate")
			return nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb), withHierarchy(h))
	if err := execCreateScheduledEvent(ctx, &ast.CreateScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "Nightly"}, Microflow: "Ops.MF", Repeat: "Daily",
	}); err == nil {
		t.Fatal("expected an already-exists error")
	}
}

func TestDropScheduledEvent_Mock(t *testing.T) {
	mod := mkModule("Ops")
	existing := mkScheduledEvent(mod.ID, "Nightly", &model.Schedule{Kind: model.ScheduleDay})
	h := mkHierarchy(mod)
	withContainer(h, existing.ContainerID, mod.ID)

	var deleted string
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListScheduledEventsFunc: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{existing}, nil
		},
		DeleteScheduledEventFunc: func(id string) error {
			deleted = id
			return nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDropScheduledEvent(ctx, &ast.DropScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "Nightly"},
	}))
	if deleted != string(existing.ID) {
		t.Errorf("deleted %q, want %q", deleted, existing.ID)
	}
	assertContainsStr(t, buf.String(), "Dropped scheduled event: Ops.Nightly")
}

// TestDescribeScheduledEvent_Mock_RoundTrips checks that DESCRIBE emits exactly
// the fields the repeat has, and nothing from another variant.
func TestDescribeScheduledEvent_Mock_RoundTrips(t *testing.T) {
	mod := mkModule("Ops")
	ev := mkScheduledEvent(mod.ID, "QuarterEnd", &model.Schedule{
		Kind: model.ScheduleMonthWeekday, Multiplier: 3, MonthOffset: 2,
		DaySelector: "Last", Weekday: "Friday", HourOfDay: 18,
	})
	h := mkHierarchy(mod)
	withContainer(h, ev.ContainerID, mod.ID)

	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		ListScheduledEventsFunc: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{ev}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
	assertNoError(t, execDescribeScheduledEvent(ctx, &ast.DescribeScheduledEventStmt{
		Name: ast.QualifiedName{Module: "Ops", Name: "QuarterEnd"},
	}))

	out := buf.String()
	assertContainsStr(t, out, "create or modify scheduled event Ops.QuarterEnd (")
	assertContainsStr(t, out, "Repeat: MonthlyByWeekday,")
	assertContainsStr(t, out, "DaySelector: Last,")
	assertContainsStr(t, out, "Weekday: Friday,")
	// Fields of other variants must not appear, or the output would not
	// re-execute: the executor refuses a field the repeat does not have.
	assertNotContainsStr(t, out, "DayOfMonth")
	assertNotContainsStr(t, out, "Weekdays:")
	assertNotContainsStr(t, out, "MinuteOffset")
}

// TestDescribeScheduledEvent_Mock_OutputParses is the real round-trip proof:
// asserting on strings passes against a formatter emitting something nothing
// can read.
func TestDescribeScheduledEvent_Mock_OutputParses(t *testing.T) {
	mod := mkModule("Ops")
	for _, sched := range []*model.Schedule{
		{Kind: model.ScheduleMinute, Multiplier: 5},
		{Kind: model.ScheduleHour, Multiplier: 2, MinuteOffset: 23},
		{Kind: model.ScheduleDay, HourOfDay: 4},
		{Kind: model.ScheduleWeek, Weekdays: [7]bool{false, true, false, false, false, true, false}, HourOfDay: 9},
		{Kind: model.ScheduleMonthDate, Multiplier: 1, DayOfMonth: 15},
		{Kind: model.ScheduleMonthWeekday, Multiplier: 3, DaySelector: "Last", Weekday: "Friday"},
		{Kind: model.ScheduleYearDate, Month: 1, DayOfMonth: 2},
		{Kind: model.ScheduleYearWeekday, Month: 3, DaySelector: "First", Weekday: "Monday"},
	} {
		t.Run(string(sched.Kind), func(t *testing.T) {
			ev := mkScheduledEvent(mod.ID, "E", sched)
			h := mkHierarchy(mod)
			withContainer(h, ev.ContainerID, mod.ID)
			mb := &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				ListScheduledEventsFunc: func() ([]*model.ScheduledEvent, error) {
					return []*model.ScheduledEvent{ev}, nil
				},
			}
			ctx, buf := newMockCtx(t, withBackend(mb), withHierarchy(h))
			assertNoError(t, execDescribeScheduledEvent(ctx, &ast.DescribeScheduledEventStmt{
				Name: ast.QualifiedName{Module: "Ops", Name: "E"},
			}))

			prog, errs := visitor.Build(buf.String())
			if len(errs) > 0 {
				t.Fatalf("describe output does not parse: %v\n%s", errs, buf.String())
			}
			stmt, ok := prog.Statements[0].(*ast.CreateScheduledEventStmt)
			if !ok {
				t.Fatalf("statement 0 = %T", prog.Statements[0])
			}
			// And the parsed statement must be accepted by the same validation
			// that guards a hand-written one.
			back, err := scheduleFromStmt(stmt)
			if err != nil {
				t.Fatalf("describe output is refused on re-execution: %v\n%s", err, buf.String())
			}
			if back.Kind != sched.Kind {
				t.Errorf("round-tripped kind = %v, want %v", back.Kind, sched.Kind)
			}
		})
	}
}
