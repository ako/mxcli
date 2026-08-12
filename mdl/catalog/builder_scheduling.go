// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	sched "github.com/mendixlabs/mxcli/mdl/scheduledevents"
	"github.com/mendixlabs/mxcli/model"
)

// Catalog rows for the two scheduling document types.
//
// Both read through the raw-unit surface and decode with the same codec the
// writers use, so the catalog cannot drift from what is stored. Scheduled events
// get their own builder rather than going through buildSimpleNamedDocs because
// the interesting columns — which microflow runs, how often, whether it is
// enabled — are the whole reason to query them.

// scheduledEventRef is one event → microflow edge, held until buildReferences.
type scheduledEventRef struct{ qualifiedName, moduleName, microflow string }

// buildScheduledEvents catalogs ScheduledEvents$ScheduledEvent units.
func (b *Builder) buildScheduledEvents() error {
	units, err := b.reader.ListRawUnitsByType(sched.TypeName)
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(
		`INSERT INTO scheduled_events_data
		 (Id, Name, QualifiedName, ModuleName, Folder, Description, Microflow,
		  Repeat, RepeatDescription, IntervalSeconds, Enabled, TimeZone, OnOverlap,
		  ProjectId, SnapshotId)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			continue
		}
		ev := sched.Parse(doc, model.ID(u.ID), model.ID(u.ContainerID))
		if ev.Name == "" {
			continue
		}
		moduleID := b.hierarchy.findModuleID(u.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		repeat, desc := "", ""
		if ev.Schedule != nil {
			repeat = string(ev.Schedule.Kind)
			desc = describeSchedule(ev.Schedule)
		}
		if _, err := stmt.Exec(
			string(u.ID), ev.Name, moduleName+"."+ev.Name, moduleName,
			b.hierarchy.buildFolderPath(u.ContainerID), ev.Documentation,
			string(ev.MicroflowID), repeat, desc,
			scheduleIntervalSeconds(ev.Schedule), boolToInt(ev.Enabled),
			ev.TimeZone, ev.OnOverlap, projectID, snapshotID,
		); err != nil {
			return err
		}
		if ev.MicroflowID != "" {
			b.scheduledEventRefs = append(b.scheduledEventRefs, scheduledEventRef{
				qualifiedName: moduleName + "." + ev.Name,
				moduleName:    moduleName,
				microflow:     string(ev.MicroflowID),
			})
		}
		count++
	}

	b.report("Scheduled Events", count)
	return nil
}

// buildQueues catalogs Queues$Queue units.
func (b *Builder) buildQueues() error {
	units, err := b.reader.ListRawUnitsByType("Queues$Queue")
	if err != nil {
		return err
	}

	stmt, err := b.tx.Prepare(
		`INSERT INTO queues_data
		 (Id, Name, QualifiedName, ModuleName, Folder, Description, Parallelism,
		  ClusterWide, ProjectId, SnapshotId)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	projectID, snapshotID := b.snapshotMeta()

	count := 0
	for _, u := range units {
		var doc bson.M
		if err := bson.Unmarshal(u.Contents, &doc); err != nil {
			continue
		}
		name, _ := doc["Name"].(string)
		if name == "" {
			continue
		}
		documentation, _ := doc["Documentation"].(string)
		// Parallelism lives on the nested Config node and is an EXPRESSION
		// string, so it is stored as text rather than an integer column.
		parallelism := ""
		clusterWide := false
		if cfg, ok := doc["Config"].(bson.M); ok {
			parallelism, _ = cfg["ParallelismExpression"].(string)
			clusterWide, _ = cfg["ClusterWide"].(bool)
		}
		moduleID := b.hierarchy.findModuleID(u.ContainerID)
		moduleName := b.hierarchy.getModuleName(moduleID)

		if _, err := stmt.Exec(
			string(u.ID), name, moduleName+"."+name, moduleName,
			b.hierarchy.buildFolderPath(u.ContainerID), documentation,
			parallelism, boolToInt(clusterWide), projectID, snapshotID,
		); err != nil {
			return err
		}
		count++
	}

	b.report("Task Queues", count)
	return nil
}

// scheduleIntervalSeconds is how often the event fires, derived from the
// Schedule child.
//
// It deliberately does NOT use the stored Interval/IntervalType pair, which is a
// legacy sibling of Schedule that Studio Pro writes and does not keep in sync —
// Workflow Commons ships an event storing 0/"Minute" beside a DaySchedule of
// 01:00, which would catalog as "fires every 0 seconds".
//
// The month and year figures are averages (30 and 365 days); the column is for
// ordering and thresholds ("anything under a minute"), not for arithmetic on
// calendar dates.
func scheduleIntervalSeconds(s *model.Schedule) int64 {
	if s == nil {
		return 0
	}
	mult := int64(s.Multiplier)
	if mult < 1 {
		mult = 1
	}
	const day = int64(86400)
	switch s.Kind {
	case model.ScheduleMinute:
		return mult * 60
	case model.ScheduleHour:
		return mult * 3600
	case model.ScheduleDay:
		return day
	case model.ScheduleWeek:
		// A weekly schedule can name several days, so the gap between runs is
		// the week divided by how many are selected.
		n := int64(0)
		for _, on := range s.Weekdays {
			if on {
				n++
			}
		}
		if n == 0 {
			return 7 * day
		}
		return (7 * day) / n
	case model.ScheduleMonthDate, model.ScheduleMonthWeekday:
		return mult * 30 * day
	case model.ScheduleYearDate, model.ScheduleYearWeekday:
		return 365 * day
	}
	return 0
}

// describeSchedule renders the repeat rule as a short human phrase, so a catalog
// query is readable without decoding the variant's fields.
func describeSchedule(s *model.Schedule) string {
	if s == nil {
		return ""
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
		var days []string
		for i, on := range s.Weekdays {
			if on {
				days = append(days, sched.WeekdayNames[i][:3])
			}
		}
		if days == nil {
			days = []string{"(no days)"}
		}
		return "weekly " + strings.Join(days, "/") + " at " + at
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
