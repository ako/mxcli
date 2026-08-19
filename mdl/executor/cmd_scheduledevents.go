// SPDX-License-Identifier: Apache-2.0

// Package executor — scheduled event commands (CREATE/DROP/SHOW/DESCRIBE
// SCHEDULED EVENT).
//
// A Mendix scheduled event runs a microflow on a repeating schedule. It is
// Mendix's cron: the repeat rule is a ScheduledEvents$Schedule child with eight
// variants, and OnOverlap (DelayNext|SkipNext) is its own concurrency control —
// scheduled events do not go through a task queue.
package executor

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
)

// repeatKinds maps the MDL Repeat value to the storage's schedule variant.
// The MDL spellings are the calendar words a reader expects; the storage names
// are Mendix's.
var repeatKinds = map[string]model.ScheduleKind{
	"minutely":         model.ScheduleMinute,
	"hourly":           model.ScheduleHour,
	"daily":            model.ScheduleDay,
	"weekly":           model.ScheduleWeek,
	"monthlybydate":    model.ScheduleMonthDate,
	"monthlybyweekday": model.ScheduleMonthWeekday,
	"yearlybydate":     model.ScheduleYearDate,
	"yearlybyweekday":  model.ScheduleYearWeekday,
}

// repeatNames is the reverse map, for DESCRIBE.
var repeatNames = map[model.ScheduleKind]string{
	model.ScheduleMinute:       "Minutely",
	model.ScheduleHour:         "Hourly",
	model.ScheduleDay:          "Daily",
	model.ScheduleWeek:         "Weekly",
	model.ScheduleMonthDate:    "MonthlyByDate",
	model.ScheduleMonthWeekday: "MonthlyByWeekday",
	model.ScheduleYearDate:     "YearlyByDate",
	model.ScheduleYearWeekday:  "YearlyByWeekday",
}

// repeatFields lists the properties each repeat actually uses. A property that
// belongs to a different variant is REFUSED rather than ignored: the variants
// differ in which fields they carry, so silently dropping one would write a
// schedule that does not do what the script says.
var repeatFields = map[model.ScheduleKind][]string{
	model.ScheduleMinute:       {"Multiplier"},
	model.ScheduleHour:         {"Multiplier", "MinuteOffset"},
	model.ScheduleDay:          {"HourOfDay", "MinuteOfHour"},
	model.ScheduleWeek:         {"Weekdays", "HourOfDay", "MinuteOfHour"},
	model.ScheduleMonthDate:    {"Multiplier", "MonthOffset", "DayOfMonth", "HourOfDay", "MinuteOfHour"},
	model.ScheduleMonthWeekday: {"Multiplier", "MonthOffset", "DaySelector", "Weekday", "HourOfDay", "MinuteOfHour"},
	model.ScheduleYearDate:     {"Month", "DayOfMonth", "HourOfDay", "MinuteOfHour"},
	model.ScheduleYearWeekday:  {"Month", "DaySelector", "Weekday", "HourOfDay", "MinuteOfHour"},
}

var daySelectors = []string{"First", "Second", "Third", "Fourth", "Last"}

var weekdayNames = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// findScheduledEvent returns the event with the given module-qualified name, or nil.
func findScheduledEvent(ctx *ExecContext, moduleName, name string) *model.ScheduledEvent {
	events, err := ctx.Backend.ListScheduledEvents()
	if err != nil {
		return nil
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil
	}
	for _, ev := range events {
		if !strings.EqualFold(ev.Name, name) {
			continue
		}
		mod := h.GetModuleName(h.FindModuleID(ev.ContainerID))
		if strings.EqualFold(mod, moduleName) {
			return ev
		}
	}
	return nil
}

// execCreateScheduledEvent handles CREATE [OR REPLACE|MODIFY] SCHEDULED EVENT.
func execCreateScheduledEvent(ctx *ExecContext, s *ast.CreateScheduledEventStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}

	module, err := findOrCreateModule(ctx, s.Name.Module)
	if err != nil {
		return err
	}

	existing := findScheduledEvent(ctx, s.Name.Module, s.Name.Name)
	if existing != nil && !s.CreateOrModify {
		return mdlerrors.NewAlreadyExists("scheduled event", s.Name.String())
	}

	ev, err := scheduledEventFromStmt(s)
	if err != nil {
		return err
	}
	ev.ContainerID = module.ID
	if existing != nil {
		ev.ID = existing.ID
		ev.ContainerID = existing.ContainerID
		// Interval/IntervalType are legacy siblings of Schedule that Studio Pro
		// writes but does not keep in sync, and MDL has no syntax for them.
		// Carry the stored values so a modify does not invent new ones.
		ev.Interval = existing.Interval
		ev.IntervalType = existing.IntervalType
		if err := ctx.Backend.UpdateScheduledEvent(ev); err != nil {
			return mdlerrors.NewBackend("update scheduled event", err)
		}
		ctx.ReportMutation("Modified", "scheduled event: %s", s.Name.String())
		return nil
	}
	if err := ctx.Backend.CreateScheduledEvent(ev); err != nil {
		return mdlerrors.NewBackend("create scheduled event", err)
	}
	fmt.Fprintf(ctx.Output, "Created scheduled event: %s\n", s.Name.String())
	return nil
}

// scheduledEventFromStmt validates the statement and builds the semantic event.
func scheduledEventFromStmt(s *ast.CreateScheduledEventStmt) (*model.ScheduledEvent, error) {
	if strings.TrimSpace(s.Microflow) == "" {
		return nil, mdlerrors.NewValidation(
			"scheduled event " + s.Name.String() + " has no Microflow — a scheduled event runs a microflow, " +
				"so add `Microflow: Module.MyMicroflow` to the property list")
	}
	ev := &model.ScheduledEvent{
		Name:          s.Name.Name,
		Documentation: s.Documentation,
		MicroflowID:   model.ID(s.Microflow),
		TimeZone:      s.TimeZone,
		OnOverlap:     s.OnOverlap,
		ExportLevel:   s.ExportLevel,
	}
	if s.Enabled != nil {
		ev.Enabled = *s.Enabled
	}
	if s.Excluded != nil {
		ev.Excluded = *s.Excluded
	}
	if err := validateEnumProperty("TimeZone", ev.TimeZone, []string{"UTC", "Server"}); err != nil {
		return nil, err
	}
	if err := validateEnumProperty("OnOverlap", ev.OnOverlap, []string{"DelayNext", "SkipNext"}); err != nil {
		return nil, err
	}
	if s.StartDateTime != "" {
		t, err := time.Parse(time.RFC3339, s.StartDateTime)
		if err != nil {
			return nil, mdlerrors.NewValidation(fmt.Sprintf(
				"StartDateTime %q is not an RFC 3339 timestamp (e.g. '2026-01-01T04:00:00Z')", s.StartDateTime))
		}
		utc := t.UTC()
		ev.StartDateTime = &utc
	}

	sched, err := scheduleFromStmt(s)
	if err != nil {
		return nil, err
	}
	ev.Schedule = sched
	ev.Interval, ev.IntervalType = legacyIntervalFor(sched)
	return ev, nil
}

// legacyIntervalFor derives the Interval/IntervalType pair a new event gets.
//
// These are legacy siblings of Schedule with no MDL syntax, but they are not
// optional: IntervalType is a Mendix enumeration, and writing an empty string
// puts a value in the model that the enumeration does not have. Every Studio
// Pro-authored event carries a real one.
//
// The mapping is what the two self-consistent references show (SAML: Day/1
// beside a DaySchedule; OIDC: Hour/1 beside an HourSchedule with Multiplier 1).
// It is only used on CREATE — a modify carries the stored pair through
// untouched, because Studio Pro does NOT keep these in sync with Schedule (the
// Workflow Commons event stores Minute/0 next to a DaySchedule) and re-deriving
// them would overwrite whatever the developer's Studio Pro left behind.
func legacyIntervalFor(s *model.Schedule) (int, string) {
	if s == nil {
		return 1, "Day"
	}
	switch s.Kind {
	case model.ScheduleMinute:
		return s.Multiplier, "Minute"
	case model.ScheduleHour:
		return s.Multiplier, "Hour"
	case model.ScheduleDay:
		return 1, "Day"
	case model.ScheduleWeek:
		return 1, "Week"
	case model.ScheduleMonthDate, model.ScheduleMonthWeekday:
		return s.Multiplier, "Month"
	case model.ScheduleYearDate, model.ScheduleYearWeekday:
		return 1, "Year"
	}
	return 1, "Day"
}

// scheduleFromStmt builds the repeat rule, refusing any field that does not
// belong to the chosen Repeat.
func scheduleFromStmt(s *ast.CreateScheduledEventStmt) (*model.Schedule, error) {
	if strings.TrimSpace(s.Repeat) == "" {
		return nil, mdlerrors.NewValidation(
			"scheduled event " + s.Name.String() + " has no Repeat — add one of " +
				strings.Join(sortedRepeatNames(), ", "))
	}
	kind, ok := repeatKinds[strings.ToLower(s.Repeat)]
	if !ok {
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"unknown Repeat %q — expected one of %s", s.Repeat, strings.Join(sortedRepeatNames(), ", ")))
	}

	// Which properties the statement actually set, so a field belonging to a
	// different variant is reported instead of silently dropped.
	set := map[string]bool{
		"Multiplier":   s.Multiplier != nil,
		"MinuteOffset": s.MinuteOffset != nil,
		"MonthOffset":  s.MonthOffset != nil,
		"HourOfDay":    s.HourOfDay != nil,
		"MinuteOfHour": s.MinuteOfHour != nil,
		"DayOfMonth":   s.DayOfMonth != nil,
		"Month":        s.Month != nil,
		"Weekdays":     s.Weekdays != "",
		"DaySelector":  s.DaySelector != "",
		"Weekday":      s.Weekday != "",
	}
	allowed := map[string]bool{}
	for _, f := range repeatFields[kind] {
		allowed[f] = true
	}
	var stray []string
	for f, isSet := range set {
		if isSet && !allowed[f] {
			stray = append(stray, f)
		}
	}
	if len(stray) > 0 {
		sort.Strings(stray)
		return nil, mdlerrors.NewValidation(fmt.Sprintf(
			"Repeat %s does not have %s — it takes %s",
			s.Repeat, strings.Join(stray, ", "), strings.Join(repeatFields[kind], ", ")))
	}

	sched := &model.Schedule{Kind: kind}
	if s.Multiplier != nil {
		sched.Multiplier = *s.Multiplier
	} else if allowed["Multiplier"] {
		// "every 1 <unit>" — an unstated multiplier of 0 would never fire.
		sched.Multiplier = 1
	}
	if s.MinuteOffset != nil {
		sched.MinuteOffset = *s.MinuteOffset
	}
	if s.MonthOffset != nil {
		sched.MonthOffset = *s.MonthOffset
	}
	if s.HourOfDay != nil {
		sched.HourOfDay = *s.HourOfDay
	}
	if s.MinuteOfHour != nil {
		sched.MinuteOfHour = *s.MinuteOfHour
	}
	if s.DayOfMonth != nil {
		sched.DayOfMonth = *s.DayOfMonth
	} else if allowed["DayOfMonth"] {
		sched.DayOfMonth = 1
	}
	if s.Month != nil {
		sched.Month = *s.Month
	} else if allowed["Month"] {
		sched.Month = 1
	}
	sched.DaySelector = s.DaySelector
	sched.Weekday = s.Weekday

	if err := validateScheduleRanges(sched, allowed); err != nil {
		return nil, err
	}
	if allowed["Weekdays"] {
		days, err := parseWeekdays(s.Weekdays)
		if err != nil {
			return nil, err
		}
		sched.Weekdays = days
	}
	if allowed["DaySelector"] {
		if err := validateEnumProperty("DaySelector", sched.DaySelector, daySelectors); err != nil {
			return nil, err
		}
		if sched.DaySelector == "" {
			sched.DaySelector = "First"
		}
		if err := validateEnumProperty("Weekday", sched.Weekday, weekdayNames); err != nil {
			return nil, err
		}
		if sched.Weekday == "" {
			sched.Weekday = "Monday"
		}
	}
	return sched, nil
}

// validateScheduleRanges rejects values Mendix's editor would not let you enter.
// A schedule that is stored but can never fire is worse than a refusal, because
// nothing downstream reports it.
func validateScheduleRanges(s *model.Schedule, allowed map[string]bool) error {
	checks := []struct {
		name     string
		value    int
		min, max int
	}{
		{"Multiplier", s.Multiplier, 1, 1_000_000},
		{"MinuteOffset", s.MinuteOffset, 0, 59},
		{"MonthOffset", s.MonthOffset, 0, 11},
		{"HourOfDay", s.HourOfDay, 0, 23},
		{"MinuteOfHour", s.MinuteOfHour, 0, 59},
		{"DayOfMonth", s.DayOfMonth, 1, 31},
		{"Month", s.Month, 1, 12},
	}
	for _, c := range checks {
		if !allowed[c.name] {
			continue
		}
		if c.value < c.min || c.value > c.max {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"%s is %d — it must be between %d and %d", c.name, c.value, c.min, c.max))
		}
	}
	return nil
}

// parseWeekdays turns "Monday, Friday" into the seven flags of a WeekSchedule.
func parseWeekdays(spec string) ([7]bool, error) {
	var days [7]bool
	if strings.TrimSpace(spec) == "" {
		return days, mdlerrors.NewValidation(
			"Repeat Weekly needs Weekdays — e.g. Weekdays: 'Monday, Friday'")
	}
	for _, part := range strings.Split(spec, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		idx := -1
		for i, w := range weekdayNames {
			if strings.EqualFold(w, name) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return days, mdlerrors.NewValidation(fmt.Sprintf(
				"unknown weekday %q in Weekdays — expected one of %s", name, strings.Join(weekdayNames, ", ")))
		}
		days[idx] = true
	}
	return days, nil
}

// validateEnumProperty accepts an empty value (the writer supplies the default)
// and otherwise requires an exact match, normalising nothing — an enum value
// Mendix does not know produces a document that loads and misbehaves.
func validateEnumProperty(name, value string, allowed []string) error {
	if value == "" {
		return nil
	}
	for _, a := range allowed {
		if a == value {
			return nil
		}
	}
	for _, a := range allowed {
		if strings.EqualFold(a, value) {
			return mdlerrors.NewValidation(fmt.Sprintf(
				"%s %q has the wrong casing — Mendix stores it as %q", name, value, a))
		}
	}
	return mdlerrors.NewValidation(fmt.Sprintf(
		"unknown %s %q — expected one of %s", name, value, strings.Join(allowed, ", ")))
}

func sortedRepeatNames() []string {
	out := make([]string, 0, len(repeatNames))
	for _, n := range repeatNames {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// execDropScheduledEvent handles DROP SCHEDULED EVENT Module.Name.
func execDropScheduledEvent(ctx *ExecContext, s *ast.DropScheduledEventStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	if !ctx.ConnectedForWrite() {
		return mdlerrors.NewNotConnectedWrite()
	}
	existing := findScheduledEvent(ctx, s.Name.Module, s.Name.Name)
	if existing == nil {
		return mdlerrors.NewNotFound("scheduled event", s.Name.String())
	}
	if err := ctx.Backend.DeleteScheduledEvent(string(existing.ID)); err != nil {
		return mdlerrors.NewBackend("drop scheduled event", err)
	}
	fmt.Fprintf(ctx.Output, "Dropped scheduled event: %s\n", s.Name.String())
	return nil
}

// execShowScheduledEvents handles SHOW|LIST SCHEDULED EVENTS [IN Module].
func execShowScheduledEvents(ctx *ExecContext, s *ast.ShowScheduledEventsStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	events, err := ctx.Backend.ListScheduledEvents()
	if err != nil {
		return mdlerrors.NewBackend("list scheduled events", err)
	}
	h, err := getHierarchy(ctx)
	if err != nil {
		return mdlerrors.NewBackend("build hierarchy", err)
	}

	type row struct{ qualified, repeat, microflow, enabled string }
	var rows []row
	for _, ev := range events {
		mod := h.GetModuleName(h.FindModuleID(ev.ContainerID))
		if s.Module != "" && !strings.EqualFold(mod, s.Module) {
			continue
		}
		rows = append(rows, row{
			qualified: mod + "." + ev.Name,
			repeat:    describeRepeat(ev.Schedule),
			microflow: string(ev.MicroflowID),
			enabled:   fmt.Sprintf("%t", ev.Enabled),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].qualified < rows[j].qualified })

	result := &TableResult{
		Columns: []string{"Scheduled Event", "Repeat", "Microflow", "Enabled"},
		Summary: fmt.Sprintf("(%d scheduled event(s))", len(rows)),
	}
	for _, r := range rows {
		result.Rows = append(result.Rows, []any{r.qualified, r.repeat, r.microflow, r.enabled})
	}
	return writeResult(ctx, result)
}

// describeRepeat renders the schedule as a short human phrase for the listing.
func describeRepeat(s *model.Schedule) string {
	if s == nil {
		return "(none)"
	}
	at := fmt.Sprintf("%02d:%02d", s.HourOfDay, s.MinuteOfHour)
	switch s.Kind {
	case model.ScheduleMinute:
		return fmt.Sprintf("every %d min", s.Multiplier)
	case model.ScheduleHour:
		return fmt.Sprintf("every %dh at :%02d", s.Multiplier, s.MinuteOffset)
	case model.ScheduleDay:
		return "daily at " + at
	case model.ScheduleWeek:
		return "weekly " + strings.Join(setWeekdayNames(s.Weekdays), "/") + " at " + at
	case model.ScheduleMonthDate:
		return fmt.Sprintf("every %d month(s) on day %d at %s", s.Multiplier, s.DayOfMonth, at)
	case model.ScheduleMonthWeekday:
		return fmt.Sprintf("every %d month(s) on the %s %s at %s", s.Multiplier, s.DaySelector, s.Weekday, at)
	case model.ScheduleYearDate:
		return fmt.Sprintf("yearly on %d/%d at %s", s.Month, s.DayOfMonth, at)
	case model.ScheduleYearWeekday:
		return fmt.Sprintf("yearly on the %s %s of month %d at %s", s.DaySelector, s.Weekday, s.Month, at)
	}
	return string(s.Kind)
}

func setWeekdayNames(days [7]bool) []string {
	var out []string
	for i, on := range days {
		if on {
			out = append(out, weekdayNames[i][:3])
		}
	}
	if out == nil {
		return []string{"(no days)"}
	}
	return out
}

// execDescribeScheduledEvent handles DESCRIBE SCHEDULED EVENT Module.Name,
// emitting re-executable MDL so describe → exec round-trips.
func execDescribeScheduledEvent(ctx *ExecContext, s *ast.DescribeScheduledEventStmt) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}
	ev := findScheduledEvent(ctx, s.Name.Module, s.Name.Name)
	if ev == nil {
		return mdlerrors.NewNotFound("scheduled event", s.Name.String())
	}

	if ev.Documentation != "" {
		fmt.Fprintf(ctx.Output, "/**\n * %s\n */\n", ev.Documentation)
	}
	fmt.Fprintf(ctx.Output, "create or modify scheduled event %s (\n", s.Name.String())
	fmt.Fprintf(ctx.Output, "  Microflow: %s,\n", ev.MicroflowID)
	for _, line := range describeScheduleProperties(ev.Schedule) {
		fmt.Fprintf(ctx.Output, "  %s,\n", line)
	}
	fmt.Fprintf(ctx.Output, "  Enabled: %t,\n", ev.Enabled)
	if ev.OnOverlap != "" {
		fmt.Fprintf(ctx.Output, "  OnOverlap: %s,\n", ev.OnOverlap)
	}
	if ev.TimeZone != "" {
		fmt.Fprintf(ctx.Output, "  TimeZone: %s,\n", ev.TimeZone)
	}
	// A zero StartDateTime is how "no start restriction" is stored (Mendix's own
	// DateTime.MinValue), not a date anyone chose — emitting it would put
	// '0001-01-01T00:00:00Z' into every describe.
	if ev.StartDateTime != nil && !ev.StartDateTime.IsZero() {
		fmt.Fprintf(ctx.Output, "  StartDateTime: '%s',\n", ev.StartDateTime.Format(time.RFC3339))
	}
	fmt.Fprint(ctx.Output, ");\n")
	// Interval/IntervalType have no MDL syntax: they are legacy siblings of
	// Schedule that Studio Pro writes without keeping in sync. Reporting them as
	// a comment keeps the output honest without making it non-re-executable.
	if ev.IntervalType != "" {
		fmt.Fprintf(ctx.Output, "-- legacy (preserved, not authorable): Interval %d %s\n", ev.Interval, ev.IntervalType)
	}
	return nil
}

// describeScheduleProperties emits the Repeat plus exactly the fields that
// repeat uses, in the order repeatFields lists them.
func describeScheduleProperties(s *model.Schedule) []string {
	if s == nil {
		return nil
	}
	out := []string{"Repeat: " + repeatNames[s.Kind]}
	for _, f := range repeatFields[s.Kind] {
		switch f {
		case "Multiplier":
			out = append(out, fmt.Sprintf("Multiplier: %d", s.Multiplier))
		case "MinuteOffset":
			out = append(out, fmt.Sprintf("MinuteOffset: %d", s.MinuteOffset))
		case "MonthOffset":
			out = append(out, fmt.Sprintf("MonthOffset: %d", s.MonthOffset))
		case "HourOfDay":
			out = append(out, fmt.Sprintf("HourOfDay: %d", s.HourOfDay))
		case "MinuteOfHour":
			out = append(out, fmt.Sprintf("MinuteOfHour: %d", s.MinuteOfHour))
		case "DayOfMonth":
			out = append(out, fmt.Sprintf("DayOfMonth: %d", s.DayOfMonth))
		case "Month":
			out = append(out, fmt.Sprintf("Month: %d", s.Month))
		case "DaySelector":
			out = append(out, "DaySelector: "+s.DaySelector)
		case "Weekday":
			out = append(out, "Weekday: "+s.Weekday)
		case "Weekdays":
			var names []string
			for i, on := range s.Weekdays {
				if on {
					names = append(names, weekdayNames[i])
				}
			}
			out = append(out, "Weekdays: '"+strings.Join(names, ", ")+"'")
		}
	}
	return out
}
