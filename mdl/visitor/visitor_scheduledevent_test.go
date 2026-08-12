// SPDX-License-Identifier: Apache-2.0

package visitor

import (
	"testing"

	"github.com/mendixlabs/mxcli/mdl/ast"
)

func TestCreateScheduledEvent(t *testing.T) {
	input := `CREATE SCHEDULED EVENT Ops.NightlyCleanup (
		Microflow: Ops.SE_Cleanup,
		Repeat: Daily,
		HourOfDay: 4,
		MinuteOfHour: 0,
		TimeZone: Server,
		Enabled: true,
		StartDateTime: '2026-01-01T04:00:00Z'
	);`
	prog, errs := Build(input)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.CreateScheduledEventStmt)
	if !ok {
		t.Fatalf("expected CreateScheduledEventStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.String() != "Ops.NightlyCleanup" {
		t.Errorf("Name = %s", stmt.Name.String())
	}
	if stmt.Microflow != "Ops.SE_Cleanup" {
		t.Errorf("Microflow = %q — a qualified name must survive as one", stmt.Microflow)
	}
	if stmt.Repeat != "Daily" {
		t.Errorf("Repeat = %q", stmt.Repeat)
	}
	if stmt.HourOfDay == nil || *stmt.HourOfDay != 4 {
		t.Errorf("HourOfDay = %v", stmt.HourOfDay)
	}
	// 0 is a real minute, so an explicit 0 must not be indistinguishable from
	// an omitted property — that is why these fields are pointers.
	if stmt.MinuteOfHour == nil || *stmt.MinuteOfHour != 0 {
		t.Errorf("MinuteOfHour = %v, want an explicit 0", stmt.MinuteOfHour)
	}
	if stmt.TimeZone != "Server" {
		t.Errorf("TimeZone = %q", stmt.TimeZone)
	}
	if stmt.Enabled == nil || !*stmt.Enabled {
		t.Errorf("Enabled = %v", stmt.Enabled)
	}
	if stmt.StartDateTime != "2026-01-01T04:00:00Z" {
		t.Errorf("StartDateTime = %q", stmt.StartDateTime)
	}
}

func TestCreateScheduledEvent_OmittedFieldsStayNil(t *testing.T) {
	prog, errs := Build(`CREATE SCHEDULED EVENT Ops.E ( Microflow: Ops.MF, Repeat: Minutely, Multiplier: 5 );`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateScheduledEventStmt)
	if stmt.Multiplier == nil || *stmt.Multiplier != 5 {
		t.Errorf("Multiplier = %v", stmt.Multiplier)
	}
	if stmt.HourOfDay != nil {
		t.Errorf("HourOfDay = %v, want nil for an omitted property", stmt.HourOfDay)
	}
	if stmt.Enabled != nil {
		t.Errorf("Enabled = %v, want nil for an omitted property", stmt.Enabled)
	}
}

func TestCreateScheduledEvent_WeeklyAndSelectors(t *testing.T) {
	prog, errs := Build(`CREATE OR MODIFY SCHEDULED EVENT Ops.E (
		Microflow: Ops.MF,
		Repeat: MonthlyByWeekday,
		Multiplier: 3,
		DaySelector: Last,
		Weekday: Friday,
		Weekdays: 'Monday, Friday'
	);`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateScheduledEventStmt)
	if !stmt.CreateOrModify {
		t.Error("CreateOrModify not set for CREATE OR MODIFY")
	}
	if stmt.DaySelector != "Last" || stmt.Weekday != "Friday" {
		t.Errorf("DaySelector/Weekday = %q/%q", stmt.DaySelector, stmt.Weekday)
	}
	if stmt.Weekdays != "Monday, Friday" {
		t.Errorf("Weekdays = %q", stmt.Weekdays)
	}
}

func TestDropScheduledEvent(t *testing.T) {
	prog, errs := Build(`DROP SCHEDULED EVENT Ops.NightlyCleanup;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt, ok := prog.Statements[0].(*ast.DropScheduledEventStmt)
	if !ok {
		t.Fatalf("expected DropScheduledEventStmt, got %T", prog.Statements[0])
	}
	if stmt.Name.String() != "Ops.NightlyCleanup" {
		t.Errorf("Name = %s", stmt.Name.String())
	}
}

func TestShowAndDescribeScheduledEvents(t *testing.T) {
	prog, errs := Build(`SHOW SCHEDULED EVENTS; LIST SCHEDULED EVENTS IN Ops; DESCRIBE SCHEDULED EVENT Ops.NightlyCleanup;`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	if len(prog.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Statements))
	}
	if _, ok := prog.Statements[0].(*ast.ShowScheduledEventsStmt); !ok {
		t.Errorf("statement 0 = %T", prog.Statements[0])
	}
	s1, ok := prog.Statements[1].(*ast.ShowScheduledEventsStmt)
	if !ok {
		t.Fatalf("statement 1 = %T", prog.Statements[1])
	}
	if s1.Module != "Ops" {
		t.Errorf("Module = %q", s1.Module)
	}
	if _, ok := prog.Statements[2].(*ast.DescribeScheduledEventStmt); !ok {
		t.Errorf("statement 2 = %T", prog.Statements[2])
	}
}

// TestScheduledKeywordStillUsableAsIdentifier guards the cost of adding
// SCHEDULED to the lexer: a new keyword stops being usable as an ordinary name
// unless it is also listed in the `keyword` rule.
func TestScheduledKeywordStillUsableAsIdentifier(t *testing.T) {
	prog, errs := Build(`CREATE ENTITY Ops.Job ( scheduled: Boolean, event: String(50) );`)
	if len(errs) > 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	stmt := prog.Statements[0].(*ast.CreateEntityStmt)
	if len(stmt.Attributes) != 2 {
		t.Fatalf("expected 2 attributes, got %d", len(stmt.Attributes))
	}
	if stmt.Attributes[0].Name != "scheduled" || stmt.Attributes[1].Name != "event" {
		t.Errorf("attribute names = %q, %q", stmt.Attributes[0].Name, stmt.Attributes[1].Name)
	}
}
