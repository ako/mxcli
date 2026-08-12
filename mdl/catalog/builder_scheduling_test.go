// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// TestScheduleIntervalSeconds pins the interval a catalog query sees.
//
// It is derived from the Schedule child on purpose. The stored
// Interval/IntervalType pair is a legacy sibling that Studio Pro writes and does
// NOT keep in sync — Workflow Commons 4.11.0 ships an event storing 0/"Minute"
// beside a DaySchedule of 01:00 — so a threshold query keyed on the pair would
// read a nightly job as firing every 0 seconds.
func TestScheduleIntervalSeconds(t *testing.T) {
	const day = int64(86400)
	tests := []struct {
		name  string
		sched *model.Schedule
		want  int64
	}{
		{"nil", nil, 0},
		{"every 2 minutes", &model.Schedule{Kind: model.ScheduleMinute, Multiplier: 2}, 120},
		{"every 3 hours", &model.Schedule{Kind: model.ScheduleHour, Multiplier: 3}, 10800},
		{"daily", &model.Schedule{Kind: model.ScheduleDay}, day},
		{"weekly, one day", &model.Schedule{Kind: model.ScheduleWeek,
			Weekdays: [7]bool{false, true, false, false, false, false, false}}, 7 * day},
		// Two selected days means it fires twice a week, not once.
		{"weekly, two days", &model.Schedule{Kind: model.ScheduleWeek,
			Weekdays: [7]bool{false, true, false, false, false, true, false}}, (7 * day) / 2},
		{"weekly, no days", &model.Schedule{Kind: model.ScheduleWeek}, 7 * day},
		{"every 3 months", &model.Schedule{Kind: model.ScheduleMonthDate, Multiplier: 3}, 3 * 30 * day},
		{"yearly", &model.Schedule{Kind: model.ScheduleYearWeekday}, 365 * day},
		// An unstated multiplier is 1, not 0 — 0 would read as "never fires".
		{"multiplier defaults to 1", &model.Schedule{Kind: model.ScheduleHour}, 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scheduleIntervalSeconds(tt.sched); got != tt.want {
				t.Errorf("scheduleIntervalSeconds = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestDescribeSchedule checks the human phrase each variant produces, and that
// only that variant's fields reach it — a MonthWeekday must not print a day of
// the month, or the catalog would describe a schedule the model does not have.
func TestDescribeSchedule(t *testing.T) {
	full := model.Schedule{
		Multiplier: 3, MinuteOffset: 23, MonthOffset: 2,
		HourOfDay: 18, MinuteOfHour: 30, DayOfMonth: 15, Month: 3,
		DaySelector: "Last", Weekday: "Friday",
		Weekdays: [7]bool{false, true, false, false, false, true, false},
	}
	tests := []struct {
		kind    model.ScheduleKind
		want    string
		mustNot []string
	}{
		{model.ScheduleMinute, "every 3 min", []string{"18:30", "Friday"}},
		{model.ScheduleHour, "every 3h at :23", []string{"18:30", "Friday"}},
		{model.ScheduleDay, "daily at 18:30", []string{"Friday", "3 month"}},
		{model.ScheduleWeek, "weekly Mon/Fri at 18:30", []string{"Last"}},
		{model.ScheduleMonthDate, "every 3 month(s) on day 15 at 18:30", []string{"Friday"}},
		{model.ScheduleMonthWeekday, "every 3 month(s) on the Last Friday at 18:30", []string{"day 15"}},
		{model.ScheduleYearDate, "yearly on 3/15 at 18:30", []string{"Friday"}},
		{model.ScheduleYearWeekday, "yearly on the Last Friday of month 3 at 18:30", []string{"day 15"}},
	}
	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			s := full
			s.Kind = tt.kind
			got := describeSchedule(&s)
			if got != tt.want {
				t.Errorf("describeSchedule = %q, want %q", got, tt.want)
			}
			for _, bad := range tt.mustNot {
				if strings.Contains(got, bad) {
					t.Errorf("describeSchedule = %q — %q belongs to a different variant", got, bad)
				}
			}
		})
	}
	if got := describeSchedule(nil); got != "" {
		t.Errorf("describeSchedule(nil) = %q, want empty", got)
	}
}
