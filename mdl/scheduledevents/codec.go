// SPDX-License-Identifier: Apache-2.0

// Package scheduledevents holds the BSON codec for scheduled events
// (ScheduledEvents$ScheduledEvent) and their polymorphic Schedule child.
//
// It lives outside both engines because both write this document, and the
// schedule has eight variants that differ in which fields they carry — a shape
// worth getting right once. The package depends only on the model types and the
// BSON driver, so either backend can call it.
package scheduledevents

import (
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// TypeName is the BSON storage name for a scheduled event document.
const TypeName = "ScheduledEvents$ScheduledEvent"

// ScheduleTypeNames maps a schedule kind to its BSON storage name. The variants
// differ in arity, not just in values, so the kind is dispatched before any
// field is read or written — see Serialize and ParseSchedule.
var ScheduleTypeNames = map[model.ScheduleKind]string{
	model.ScheduleMinute:       "ScheduledEvents$MinuteSchedule",
	model.ScheduleHour:         "ScheduledEvents$HourSchedule",
	model.ScheduleDay:          "ScheduledEvents$DaySchedule",
	model.ScheduleWeek:         "ScheduledEvents$WeekSchedule",
	model.ScheduleMonthDate:    "ScheduledEvents$MonthDateSchedule",
	model.ScheduleMonthWeekday: "ScheduledEvents$MonthWeekdaySchedule",
	model.ScheduleYearDate:     "ScheduledEvents$YearDateSchedule",
	model.ScheduleYearWeekday:  "ScheduledEvents$YearWeekdaySchedule",
}

// WeekdayNames indexes model.Schedule.Weekdays: Sunday first, matching the
// Mendix Weekday enumeration and the BSON property names of WeekSchedule.
var WeekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// Serialize writes the document in the shape Studio Pro produces.
//
// Pinned against four Studio Pro-authored events (Workflow Commons 4.11.0, OIDC
// SSO 4.6.0, SAML 4.2.1 ×2). All four agree on the property set and on three
// types the generated metamodel would lead you to get wrong:
//
//   - every integer is BSON int64, while modelsdk/gen declares int32 — the same
//     mismatch that made the READER fail on Studio Pro documents in issue #585;
//   - StartDateTime is a BSON UTC datetime, while gen declares a string;
//   - Microflow is a by-name reference (the qualified name), not an element ID.
//
// Interval/IntervalType are legacy siblings of Schedule that Studio Pro still
// writes and does NOT keep in sync — the Workflow Commons event stores
// Interval 0 / "Minute" next to a DaySchedule of 01:00. They are therefore
// carried through from the caller rather than derived from the schedule, so a
// round trip cannot invent a value Mendix did not have.
func Serialize(ev *model.ScheduledEvent) ([]byte, error) {
	exportLevel := ev.ExportLevel
	if exportLevel == "" {
		exportLevel = "Hidden"
	}
	onOverlap := ev.OnOverlap
	if onOverlap == "" {
		// Every reference document uses DelayNext, and it is the safer default:
		// SkipNext silently drops a run.
		onOverlap = "DelayNext"
	}
	timeZone := ev.TimeZone
	if timeZone == "" {
		timeZone = "UTC"
	}
	var start time.Time
	if ev.StartDateTime != nil {
		start = ev.StartDateTime.UTC()
	}

	doc := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(string(ev.ID))},
		{Key: "$Type", Value: TypeName},
		{Key: "Documentation", Value: ev.Documentation},
		{Key: "Enabled", Value: ev.Enabled},
		{Key: "Excluded", Value: ev.Excluded},
		{Key: "ExportLevel", Value: exportLevel},
		{Key: "Interval", Value: int64(ev.Interval)},
		{Key: "IntervalType", Value: ev.IntervalType},
		{Key: "Microflow", Value: string(ev.MicroflowID)},
		{Key: "Name", Value: ev.Name},
		{Key: "OnOverlap", Value: onOverlap},
	}
	if ev.Schedule != nil {
		sched, err := SerializeSchedule(ev.Schedule)
		if err != nil {
			return nil, err
		}
		doc = append(doc, bson.E{Key: "Schedule", Value: sched})
	}
	doc = append(doc,
		bson.E{Key: "StartDateTime", Value: primitive.DateTime(start.UnixMilli())},
		bson.E{Key: "TimeZone", Value: timeZone},
	)

	out, err := bson.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("serialize scheduled event %q: %w", ev.Name, err)
	}
	return out, nil
}

// SerializeSchedule writes the ScheduledEvents$*Schedule child.
//
// Each variant writes only the fields its type declares. Writing a field the
// type does not carry is the failure mode that produces a document mxbuild
// accepts and Studio Pro cannot open (System.InvalidOperationException at
// MprProperty), so this dispatches on the kind and never merges field sets.
// Keys are alphabetical within each variant, matching the observed documents.
func SerializeSchedule(s *model.Schedule) (bson.D, error) {
	typeName, ok := ScheduleTypeNames[s.Kind]
	if !ok {
		return nil, fmt.Errorf("unknown schedule kind %q", s.Kind)
	}
	head := bson.D{
		{Key: "$ID", Value: bsonutil.IDToBsonBinary(types.GenerateID())},
		{Key: "$Type", Value: typeName},
	}

	var fields bson.D
	switch s.Kind {
	case model.ScheduleMinute:
		fields = bson.D{{Key: "Multiplier", Value: int64(s.Multiplier)}}
	case model.ScheduleHour:
		fields = bson.D{
			{Key: "MinuteOffset", Value: int64(s.MinuteOffset)},
			{Key: "Multiplier", Value: int64(s.Multiplier)},
		}
	case model.ScheduleDay:
		fields = bson.D{
			{Key: "HourOfDay", Value: int64(s.HourOfDay)},
			{Key: "MinuteOfHour", Value: int64(s.MinuteOfHour)},
		}
	case model.ScheduleWeek:
		// Alphabetical, so the seven day flags interleave with the times rather
		// than grouping — Friday, HourOfDay, MinuteOfHour, Monday, ...
		fields = bson.D{
			{Key: "Friday", Value: s.Weekdays[5]},
			{Key: "HourOfDay", Value: int64(s.HourOfDay)},
			{Key: "MinuteOfHour", Value: int64(s.MinuteOfHour)},
			{Key: "Monday", Value: s.Weekdays[1]},
			{Key: "Saturday", Value: s.Weekdays[6]},
			{Key: "Sunday", Value: s.Weekdays[0]},
			{Key: "Thursday", Value: s.Weekdays[4]},
			{Key: "Tuesday", Value: s.Weekdays[2]},
			{Key: "Wednesday", Value: s.Weekdays[3]},
		}
	case model.ScheduleMonthDate:
		fields = bson.D{
			{Key: "DayOfMonth", Value: int64(s.DayOfMonth)},
			{Key: "HourOfDay", Value: int64(s.HourOfDay)},
			{Key: "MinuteOfHour", Value: int64(s.MinuteOfHour)},
			{Key: "MonthOffset", Value: int64(s.MonthOffset)},
			{Key: "Multiplier", Value: int64(s.Multiplier)},
		}
	case model.ScheduleMonthWeekday:
		fields = bson.D{
			{Key: "DaySelector", Value: s.DaySelector},
			{Key: "HourOfDay", Value: int64(s.HourOfDay)},
			{Key: "MinuteOfHour", Value: int64(s.MinuteOfHour)},
			{Key: "MonthOffset", Value: int64(s.MonthOffset)},
			{Key: "Multiplier", Value: int64(s.Multiplier)},
			{Key: "Weekday", Value: s.Weekday},
		}
	case model.ScheduleYearDate:
		fields = bson.D{
			{Key: "DayOfMonth", Value: int64(s.DayOfMonth)},
			{Key: "HourOfDay", Value: int64(s.HourOfDay)},
			{Key: "MinuteOfHour", Value: int64(s.MinuteOfHour)},
			{Key: "Month", Value: int64(s.Month)},
		}
	case model.ScheduleYearWeekday:
		fields = bson.D{
			{Key: "DaySelector", Value: s.DaySelector},
			{Key: "HourOfDay", Value: int64(s.HourOfDay)},
			{Key: "MinuteOfHour", Value: int64(s.MinuteOfHour)},
			{Key: "Month", Value: int64(s.Month)},
			{Key: "Weekday", Value: s.Weekday},
		}
	}
	return append(head, fields...), nil
}

// Parse converts a stored document to the semantic type. MicroflowID holds the
// by-name microflow reference (BSON "Microflow"), not an element ID.
func Parse(doc bson.M, id, containerID model.ID) *model.ScheduledEvent {
	ev := &model.ScheduledEvent{ContainerID: containerID}
	ev.ID = id
	ev.TypeName = TypeName
	ev.Name, _ = doc["Name"].(string)
	ev.Documentation, _ = doc["Documentation"].(string)
	if mf, ok := doc["Microflow"].(string); ok {
		ev.MicroflowID = model.ID(mf)
	}
	ev.Enabled, _ = doc["Enabled"].(bool)
	ev.Excluded, _ = doc["Excluded"].(bool)
	ev.ExportLevel, _ = doc["ExportLevel"].(string)
	ev.OnOverlap, _ = doc["OnOverlap"].(string)
	ev.TimeZone, _ = doc["TimeZone"].(string)
	ev.IntervalType, _ = doc["IntervalType"].(string)
	ev.Interval = anyInt(doc["Interval"])
	if dt, ok := doc["StartDateTime"].(primitive.DateTime); ok {
		t := time.UnixMilli(int64(dt)).UTC()
		ev.StartDateTime = &t
	}
	if sched, ok := doc["Schedule"].(bson.M); ok {
		ev.Schedule = ParseSchedule(sched)
	}
	return ev
}

// ParseSchedule dispatches on $Type before reading any field: the variants
// differ in arity, so inferring the kind from which keys are present would
// mis-read a schedule whose fields overlap another's.
func ParseSchedule(doc bson.M) *model.Schedule {
	typeName, _ := doc["$Type"].(string)
	var kind model.ScheduleKind
	for k, name := range ScheduleTypeNames {
		if name == typeName {
			kind = k
			break
		}
	}
	if kind == "" {
		return nil
	}
	s := &model.Schedule{Kind: kind}
	switch kind {
	case model.ScheduleMinute:
		s.Multiplier = anyInt(doc["Multiplier"])
	case model.ScheduleHour:
		s.Multiplier = anyInt(doc["Multiplier"])
		s.MinuteOffset = anyInt(doc["MinuteOffset"])
	case model.ScheduleDay:
		s.HourOfDay = anyInt(doc["HourOfDay"])
		s.MinuteOfHour = anyInt(doc["MinuteOfHour"])
	case model.ScheduleWeek:
		s.HourOfDay = anyInt(doc["HourOfDay"])
		s.MinuteOfHour = anyInt(doc["MinuteOfHour"])
		for i, day := range WeekdayNames {
			s.Weekdays[i], _ = doc[day].(bool)
		}
	case model.ScheduleMonthDate:
		s.Multiplier = anyInt(doc["Multiplier"])
		s.MonthOffset = anyInt(doc["MonthOffset"])
		s.DayOfMonth = anyInt(doc["DayOfMonth"])
		s.HourOfDay = anyInt(doc["HourOfDay"])
		s.MinuteOfHour = anyInt(doc["MinuteOfHour"])
	case model.ScheduleMonthWeekday:
		s.Multiplier = anyInt(doc["Multiplier"])
		s.MonthOffset = anyInt(doc["MonthOffset"])
		s.DaySelector, _ = doc["DaySelector"].(string)
		s.Weekday, _ = doc["Weekday"].(string)
		s.HourOfDay = anyInt(doc["HourOfDay"])
		s.MinuteOfHour = anyInt(doc["MinuteOfHour"])
	case model.ScheduleYearDate:
		s.Month = anyInt(doc["Month"])
		s.DayOfMonth = anyInt(doc["DayOfMonth"])
		s.HourOfDay = anyInt(doc["HourOfDay"])
		s.MinuteOfHour = anyInt(doc["MinuteOfHour"])
	case model.ScheduleYearWeekday:
		s.Month = anyInt(doc["Month"])
		s.DaySelector, _ = doc["DaySelector"].(string)
		s.Weekday, _ = doc["Weekday"].(string)
		s.HourOfDay = anyInt(doc["HourOfDay"])
		s.MinuteOfHour = anyInt(doc["MinuteOfHour"])
	}
	return s
}

// anyInt accepts every numeric width a writer might have produced. Studio Pro
// writes int64; older mxcli builds and hand-made fixtures write int32 (#585).
func anyInt(v any) int {
	switch n := v.(type) {
	case int64:
		return int(n)
	case int32:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}
