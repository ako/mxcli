// SPDX-License-Identifier: Apache-2.0

package ast

// CreateScheduledEventStmt represents:
//
//	CREATE [OR REPLACE|MODIFY] SCHEDULED EVENT Module.Name (
//	  Microflow: Module.MF, Repeat: Daily, HourOfDay: 4, ...
//	);
//
// Properties are kept as written and validated by the executor, which knows
// which fields the chosen Repeat actually has. Numeric fields are pointers so
// "not mentioned" stays distinguishable from "mentioned as 0" — 0 is a real
// hour, minute and month offset.
type CreateScheduledEventStmt struct {
	Name          QualifiedName
	Documentation string
	// Microflow is the qualified name of the microflow to run.
	Microflow string
	// Repeat names the schedule variant: Minutely, Hourly, Daily, Weekly,
	// MonthlyByDate, MonthlyByWeekday, YearlyByDate, YearlyByWeekday.
	Repeat string

	Multiplier   *int
	MinuteOffset *int
	MonthOffset  *int
	HourOfDay    *int
	MinuteOfHour *int
	DayOfMonth   *int
	Month        *int

	// Weekdays is the day list of a Weekly repeat, as written
	// ("Monday, Friday"); DaySelector/Weekday are the two fields of the
	// ByWeekday repeats.
	Weekdays    string
	DaySelector string
	Weekday     string

	// StartDateTime is an RFC 3339 timestamp; TimeZone is UTC or Server.
	StartDateTime string
	TimeZone      string
	// OnOverlap is DelayNext or SkipNext — what happens when a run is still
	// going when the next one is due.
	OnOverlap string

	Enabled     *bool
	Excluded    *bool
	ExportLevel string

	// CreateOrModify is set by the shared CREATE OR REPLACE/MODIFY prefix.
	CreateOrModify bool
}

func (s *CreateScheduledEventStmt) isStatement() {}

// DropScheduledEventStmt represents: DROP SCHEDULED EVENT Module.Name;
type DropScheduledEventStmt struct {
	Name QualifiedName
}

func (s *DropScheduledEventStmt) isStatement() {}

// ShowScheduledEventsStmt represents: SHOW|LIST SCHEDULED EVENTS [IN Module];
type ShowScheduledEventsStmt struct {
	Module string
}

func (s *ShowScheduledEventsStmt) isStatement() {}

// DescribeScheduledEventStmt represents: DESCRIBE SCHEDULED EVENT Module.Name;
type DescribeScheduledEventStmt struct {
	Name QualifiedName
}

func (s *DescribeScheduledEventStmt) isStatement() {}
