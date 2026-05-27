---
title: SHOW/DESCRIBE Scheduled Events
status: proposed
---

# Proposal: SHOW/DESCRIBE Scheduled Events

## Overview

**Document type:** `ScheduledEvents$ScheduledEvent`
**Prevalence:** 19 across test projects (4 Enquiries, 10 Evora, 5 Lato)
**Priority:** Medium — common in production apps for background processing

Scheduled Events trigger microflow execution on a timer. They specify which microflow to call, the schedule interval, overlap behavior, and timezone settings.

## What Already Exists

| Layer | Status | Location |
|-------|--------|----------|
| **Go type** | Partial | `model/types.go` line 246 — missing `Excluded`, `ExportLevel`, `OnOverlap`, `Schedule`, `TimeZone` |
| **Parser** | Partial | `sdk/mpr/parser_enumeration.go` line 171 — parses Name, Documentation, Microflow, Enabled, Interval, IntervalType |
| **Reader** | Yes | `ListScheduledEvents()` in `sdk/mpr/reader_documents.go` |
| **Generated metamodel** | Yes | `generated/metamodel/types.go` line 8363 |
| **AST** | No | — |
| **Executor** | No | — |

## BSON Structure (from test projects)

```
ScheduledEvents$ScheduledEvent:
  Name: string
  documentation: string
  Enabled: bool
  Excluded: bool
  ExportLevel: string ("Hidden", "api")
  microflow: string (qualified name, e.g., "MyModule.CleanupOldRecords")
  Interval: int32
  IntervalType: string ("Second", "Minute", "Hour", "Day", "Week", "Month", "Year")
  OnOverlap: string ("SkipNext", "DelayNext")
  StartDateTime: datetime
  TimeZone: string ("Server", "UTC")
  Schedule: polymorphic (see subtypes below)
```

### Schedule Subtypes

| Type | Fields |
|------|--------|
| `HourSchedule` | MinuteOffset, Multiplier |
| `MinuteSchedule` | Multiplier |
| `DaySchedule` | HourOfDay, MinuteOfHour |
| `WeekSchedule` | Monday-Sunday (bools), HourOfDay, MinuteOfHour |
| `MonthDateSchedule` | DayOfMonth, HourOfDay, MinuteOfHour, Multiplier |
| `MonthWeekdaySchedule` | DaySelector, Weekday, HourOfDay, MinuteOfHour, Multiplier |
| `YearDateSchedule` | DayOfMonth, Month, HourOfDay, MinuteOfHour |
| `YearWeekdaySchedule` | DaySelector, Month, Weekday, HourOfDay, MinuteOfHour |

## Proposed MDL Syntax

### SHOW SCHEDULED EVENTS

```
show SCHEDULED events [in module]
```

| Qualified Name | Module | Name | Enabled | Microflow | Interval | On Overlap |
|----------------|--------|------|---------|-----------|----------|------------|

### DESCRIBE SCHEDULED EVENT

```
describe SCHEDULED event Module.Name
```

Output format:

```
/**
 * Cleans up expired sessions every hour
 */
SCHEDULED event MyModule.CleanupSessions
  microflow MyModule.CleanupExpiredSessions
  ENABLED
  INTERVAL 1 HOUR
  on OVERLAP SkipNext
  TIMEZONE UTC
  START '2024-01-01T00:00:00';
/
```

For a disabled event with a weekly schedule:

```
SCHEDULED event MyModule.WeeklyReport
  microflow MyModule.GenerateWeeklyReport
  DISABLED
  SCHEDULE WEEKLY on Monday, Wednesday, Friday AT 08:30
  on OVERLAP DelayNext
  TIMEZONE Server;
/
```

## Implementation Steps

### 1. Enhance Model Type (model/types.go)

Add missing fields to existing `ScheduledEvent` struct:
- `Excluded`, `ExportLevel`, `OnOverlap`, `TimeZone`, `StartDateTime`
- `Schedule` (polymorphic — can be represented as a formatted string)

### 2. Enhance Parser (sdk/mpr/parser_enumeration.go)

Extend existing `parseScheduledEvent()` to capture all fields. Parse the polymorphic `Schedule` sub-object.

### 3. Add AST Types

```go
ShowScheduledEvents    // in ShowObjectType enum
DescribeScheduledEvent // in DescribeObjectType enum
```

### 4. Add Grammar Rules

```antlr
SCHEDULED: 'SCHEDULED';
event: 'EVENT';
events: 'EVENTS';

// show SCHEDULED events [in module]
// describe SCHEDULED event qualifiedName
```

### 5. Add Executor (mdl/executor/cmd_scheduled_events.go)

Standard show/describe pattern.

### 6. Add Autocomplete

```go
func (e *Executor) GetScheduledEventNames(moduleFilter string) []string
```

## Testing

- Verify against Evora project (10 scheduled events — most comprehensive)
