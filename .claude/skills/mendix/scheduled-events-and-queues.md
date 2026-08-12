# Scheduled Events and Task Queues

## When to Use This Skill

Use this skill when the user wants to:
- Run a microflow on a schedule ("every night at 4", "hourly", "cron", "batch job")
- Inspect or change an existing scheduled event
- Limit how many background tasks run at once (a task queue)
- Understand why `mxcli` refuses to rewrite a microflow that has a queued call

**These two features are unrelated.** A scheduled event does **not** go through a
task queue. Its own concurrency control is `OnOverlap`.

## Scheduled Events

Mendix's cron: run a microflow on a repeating schedule.

```sql
-- Inspect
list scheduled events;
list scheduled events in Ops;
describe scheduled event Ops.NightlyCleanup;   -- re-executable MDL

-- Create
create scheduled event Ops.NightlyCleanup (
  Microflow: Ops.SE_Cleanup,
  Repeat: Daily,
  HourOfDay: 4,
  MinuteOfHour: 0,
  TimeZone: Server,
  Enabled: true
);

drop scheduled event Ops.NightlyCleanup;
```

`Microflow` and `Repeat` are **always required**. `show` is a synonym for `list`.

### Pick the Repeat first, then use only its fields

Mendix stores the repeat rule as one of eight types, and they differ in **which
fields they carry** — not just in their values. Naming a field from another
repeat is an error, not a no-op:

```
Error: Repeat Daily does not have Multiplier — it takes HourOfDay, MinuteOfHour
```

| Repeat | Fields | Means |
|--------|--------|-------|
| `Minutely` | `Multiplier` | every N minutes |
| `Hourly` | `Multiplier`, `MinuteOffset` | every N hours, at :MM past |
| `Daily` | `HourOfDay`, `MinuteOfHour` | every day at HH:MM (**no multiplier**) |
| `Weekly` | `Weekdays`, `HourOfDay`, `MinuteOfHour` | on the named days at HH:MM |
| `MonthlyByDate` | `Multiplier`, `MonthOffset`, `DayOfMonth`, `HourOfDay`, `MinuteOfHour` | the Dth of every N months |
| `MonthlyByWeekday` | `Multiplier`, `MonthOffset`, `DaySelector`, `Weekday`, `HourOfDay`, `MinuteOfHour` | the last Friday of every N months |
| `YearlyByDate` | `Month`, `DayOfMonth`, `HourOfDay`, `MinuteOfHour` | every 2 January |
| `YearlyByWeekday` | `Month`, `DaySelector`, `Weekday`, `HourOfDay`, `MinuteOfHour` | the first Monday of March |

Field values:

| Field | Value |
|-------|-------|
| `Multiplier` | 1 or more (defaults to 1) |
| `MinuteOffset` | 0–59 |
| `MonthOffset` | 0-based: which month of a multi-month cycle fires |
| `HourOfDay` / `MinuteOfHour` | 0–23 / 0–59 |
| `DayOfMonth` / `Month` | 1–31 / 1–12 |
| `Weekdays` | quoted list: `'Monday, Friday'` (case-insensitive) |
| `DaySelector` | `First`, `Second`, `Third`, `Fourth`, `Last` |
| `Weekday` | `Sunday` … `Saturday` |

Optional on any repeat:

| Property | Values | Default |
|----------|--------|---------|
| `Enabled` | `true` / `false` | `false` — **a new event does not run until you enable it** |
| `OnOverlap` | `DelayNext` / `SkipNext` | `DelayNext` |
| `TimeZone` | `UTC` / `Server` | `UTC` |
| `StartDateTime` | RFC 3339, e.g. `'2026-01-01T04:00:00Z'` | none |
| `Documentation` | free text | none |

`SkipNext` drops a run that would overlap the previous one; `DelayNext` waits.

### More examples

```sql
-- Every two hours, 23 minutes past
create scheduled event Ops.HourlyPing (
  Microflow: Ops.SE_Ping,
  Repeat: Hourly,
  Multiplier: 2,
  MinuteOffset: 23
);

-- Mondays and Fridays at 09:30
create scheduled event Ops.WeeklyReport (
  Microflow: Ops.SE_Report,
  Repeat: Weekly,
  Weekdays: 'Monday, Friday',
  HourOfDay: 9,
  MinuteOfHour: 30
);

-- The last Friday of every third month, 18:00
create scheduled event Ops.QuarterEnd (
  Microflow: Ops.SE_Close,
  Repeat: MonthlyByWeekday,
  Multiplier: 3,
  MonthOffset: 2,
  DaySelector: Last,
  Weekday: Friday,
  HourOfDay: 18
);
```

## Task Queues

A task queue bounds how many queued microflow calls run at once.

```sql
list queues;
describe queue Ops.OrderProcessing;

create queue Ops.OrderProcessing ( Parallelism: 3, ClusterWide: true );
create queue Ops.Mail;                     -- defaults: parallelism 1, per-instance

create or modify queue Ops.OrderProcessing ( Parallelism: '$MyModule.Workers' );
drop queue Ops.Mail;
```

| Property | Meaning | Default |
|----------|---------|---------|
| `Parallelism` | how many run at once — an **expression**, not a number | `1` |
| `ClusterWide` | `true` = across the cluster, `false` = per runtime instance | `false` |

Mendix stores parallelism as an expression string, so `3` and `'3'` are the same
thing and an arbitrary expression is legal.

## Common Mistakes

| Mistake | Symptom | Fix |
|---------|---------|-----|
| `Multiplier` on a `Daily` repeat | `Repeat Daily does not have Multiplier` | Daily has no multiplier — use `HourOfDay`/`MinuteOfHour`, or switch to `Hourly` |
| Forgetting `Enabled: true` | The event is in the model but never runs | Set `Enabled: true` (the default is false) |
| `TimeZone: server` | `has the wrong casing — Mendix stores it as "Server"` | Use the exact spelling: `Server`, `UTC`, `DelayNext`, `SkipNext`, `Last`, `Friday` |
| `HourOfDay: 24` | `it must be between 0 and 23` | Hours are 0–23; midnight is `0` |
| Expecting a queue to throttle a scheduled event | Nothing changes | They are unrelated — use `OnOverlap` |

## Rewriting a Microflow with a Queued Call Is Refused

MDL cannot yet author a *queued call* — the binding lives on the call activity
inside a microflow, not on the queue. So `create or replace|modify microflow` is
refused when the stored microflow has one:

```
Error: microflow Ops.ACT_Caller has 1 call(s) bound to a task queue (Ops.MyQueue),
and rewriting it would silently drop that binding
```

This is deliberate. Change that microflow in Studio Pro, or remove the task queue
from the call first. Without the refusal the binding was written back as null and
the project then looked *healthier* than before — `mx check` stopped reporting
`CE1613 "The selected task queue no longer exists"`, because the configuration
the error was about had been deleted.

## Validation Checklist

Before presenting a script:

```bash
mxcli check script.mdl                       # catches wrong-repeat fields (MDL-SCHED01)
mxcli check script.mdl -p app.mpr --references
```

- [ ] Every scheduled event has `Microflow` and `Repeat`
- [ ] Only that repeat's fields are used
- [ ] `Enabled: true` if it is meant to run
- [ ] Enum values spelled exactly (`Server`, `DelayNext`, `Last`, `Monday`)
- [ ] The target microflow exists and takes no parameters

## Querying and Linting

Both document types are in the catalog after `refresh catalog`:

```sql
-- Anything that fires more often than once a minute
select QualifiedName, RepeatDescription, Microflow
from CATALOG.SCHEDULED_EVENTS
where Enabled = 1 and IntervalSeconds < 60;

select QualifiedName, Parallelism, ClusterWide from CATALOG.QUEUES;
```

A scheduled event counts as a caller of the microflow it runs, so
`show callers of Ops.SE_Cleanup` lists it and the lint rule for orphaned
microflows (QUAL004) does not flag it. `IntervalSeconds` is derived from the
schedule, not from the legacy `Interval`/`IntervalType` pair Mendix also stores.

Starlark lint rules can iterate both: `scheduled_events()` yields
`repeat`, `interval_seconds`, `on_overlap`, `time_zone`, `enabled`, and
`microflow_name`; `queues()` yields `parallelism` (a string) and `cluster_wide`.

## Related

- `mxcli syntax scheduled-event`, `mxcli syntax queue` — full syntax reference
- `write-microflows.md` — writing the microflow the event calls
- `project-settings.md` — after-startup / before-shutdown microflows
