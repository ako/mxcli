# Scheduled Events and Task Queues

Two Mendix features for work that runs outside a user request. They are unrelated
and easy to confuse:

- A **scheduled event** is Mendix's cron — it runs a microflow on a repeating
  schedule.
- A **task queue** bounds how many queued microflow calls run at once.

A scheduled event does **not** go through a task queue. Its own concurrency
control is `OnOverlap`, which decides what happens when a run is still going when
the next one is due.

## Scheduled Events

### Inspecting

```sql
-- All scheduled events, or one module's
LIST SCHEDULED EVENTS;
LIST SCHEDULED EVENTS IN Ops;

-- Re-executable MDL for one event
DESCRIBE SCHEDULED EVENT Ops.NightlyCleanup;
```

`SHOW` is accepted as a synonym for `LIST`.

### CREATE SCHEDULED EVENT

```sql
CREATE [OR MODIFY] SCHEDULED EVENT <Module>.<Name> (
  Microflow: <Module>.<Microflow>,
  Repeat: <repeat>,
  <fields of that repeat>,
  <optional properties>
);
```

`Microflow` and `Repeat` are always required.

#### Repeat variants

The repeat rule is stored as one of eight Mendix schedule types, and they differ
in **which fields they carry** — not just in their values. MDL mirrors that: each
repeat takes only its own fields, and a field belonging to another repeat is
refused by both `mxcli check` (rule `MDL-SCHED01`) and `mxcli exec` rather than
silently dropped.

| Repeat | Fields | Reads as |
|--------|--------|----------|
| `Minutely` | `Multiplier` | every N minutes |
| `Hourly` | `Multiplier`, `MinuteOffset` | every N hours, at :MM |
| `Daily` | `HourOfDay`, `MinuteOfHour` | every day at HH:MM |
| `Weekly` | `Weekdays`, `HourOfDay`, `MinuteOfHour` | on the named days at HH:MM |
| `MonthlyByDate` | `Multiplier`, `MonthOffset`, `DayOfMonth`, `HourOfDay`, `MinuteOfHour` | the Dth of every N months |
| `MonthlyByWeekday` | `Multiplier`, `MonthOffset`, `DaySelector`, `Weekday`, `HourOfDay`, `MinuteOfHour` | the last Friday of every N months |
| `YearlyByDate` | `Month`, `DayOfMonth`, `HourOfDay`, `MinuteOfHour` | every 2 January |
| `YearlyByWeekday` | `Month`, `DaySelector`, `Weekday`, `HourOfDay`, `MinuteOfHour` | the first Monday of March |

Field values:

| Field | Value |
|-------|-------|
| `Multiplier` | how many units between runs (1 or more; defaults to 1) |
| `MinuteOffset` | 0–59, the minute past the hour |
| `MonthOffset` | 0-based, which month of a multi-month cycle fires |
| `HourOfDay` / `MinuteOfHour` | 0–23 / 0–59 |
| `DayOfMonth` / `Month` | 1–31 / 1–12 |
| `Weekdays` | a quoted list, e.g. `'Monday, Friday'` (case-insensitive) |
| `DaySelector` | `First`, `Second`, `Third`, `Fourth`, `Last` |
| `Weekday` | `Sunday` … `Saturday` |

A value outside its range is an error, not a truncation: a schedule that is
stored but can never fire is worse than a refusal, because nothing downstream
reports it.

#### Optional properties

| Property | Values | Default |
|----------|--------|---------|
| `Enabled` | `true` / `false` | `false` |
| `OnOverlap` | `DelayNext` / `SkipNext` | `DelayNext` |
| `TimeZone` | `UTC` / `Server` | `UTC` |
| `StartDateTime` | an RFC 3339 timestamp; the event does not run before it | none |
| `Documentation` | free text | none |

`SkipNext` drops a run that would overlap the previous one; `DelayNext` queues it
until the previous one finishes.

### Examples

```sql
-- Every night at 04:00 in the server's timezone
CREATE SCHEDULED EVENT Ops.NightlyCleanup (
  Microflow: Ops.SE_Cleanup,
  Repeat: Daily,
  HourOfDay: 4,
  MinuteOfHour: 0,
  TimeZone: Server,
  Enabled: true
);

-- Every two hours, 23 minutes past
CREATE SCHEDULED EVENT Ops.HourlyPing (
  Microflow: Ops.SE_Ping,
  Repeat: Hourly,
  Multiplier: 2,
  MinuteOffset: 23
);

-- Mondays and Fridays at 09:30
CREATE SCHEDULED EVENT Ops.WeeklyReport (
  Microflow: Ops.SE_Report,
  Repeat: Weekly,
  Weekdays: 'Monday, Friday',
  HourOfDay: 9,
  MinuteOfHour: 30
);

-- The last Friday of every third month, at 18:00
CREATE SCHEDULED EVENT Ops.QuarterEnd (
  Microflow: Ops.SE_Close,
  Repeat: MonthlyByWeekday,
  Multiplier: 3,
  MonthOffset: 2,
  DaySelector: Last,
  Weekday: Friday,
  HourOfDay: 18
);

DROP SCHEDULED EVENT Ops.HourlyPing;
```

### A note on `Interval` / `IntervalType`

Stored events also carry an `Interval` and `IntervalType` pair. These predate the
`Schedule` child and Studio Pro does **not** keep them in sync with it — a real
Mendix module ships an event storing `0` / `Minute` next to a daily schedule of
01:00. MDL has no syntax for them: a new event gets the pair that matches its
repeat, and `CREATE OR MODIFY` carries whatever is stored through untouched.
`DESCRIBE` reports them as a comment so the output stays re-executable.

## Task Queues

A task queue bounds how many instances of a queued microflow call run at once.

```sql
CREATE [OR MODIFY] QUEUE <Module>.<Name> [(
  Parallelism: <expression>,
  ClusterWide: true|false,
  Documentation: '<text>'
)];

LIST QUEUES [IN <Module>];
DESCRIBE QUEUE <Module>.<Name>;
DROP QUEUE <Module>.<Name>;
```

| Property | Meaning | Default |
|----------|---------|---------|
| `Parallelism` | how many tasks run at once — an **expression**, not a number | `1` |
| `ClusterWide` | `true` applies the limit across the cluster; `false` per runtime instance | `false` |

Mendix stores parallelism as an expression string, so a bare integer and a quoted
one mean the same thing and an arbitrary expression is legal:

```sql
CREATE QUEUE Ops.OrderProcessing ( Parallelism: 3, ClusterWide: true );
CREATE QUEUE Ops.Mail;                      -- defaults: 1, per-instance
CREATE OR MODIFY QUEUE Ops.OrderProcessing ( Parallelism: '$MyModule.Workers' );
```

### Binding a call to a queue is not yet expressible

MDL cannot yet author a *queued call* — the binding lives on the call activity
inside a microflow, not on the queue. Because rebuilding a microflow would drop
an existing binding, `CREATE OR REPLACE|MODIFY MICROFLOW` is **refused** when the
stored microflow has a queued call, naming the queues that would be lost. Change
those microflows in Studio Pro.

Without that refusal the binding was written back as null and the project then
looked *healthier* than before — `mx check` stopped reporting
`CE1613 "The selected task queue no longer exists"`, because the configuration
the error was about had been deleted.
