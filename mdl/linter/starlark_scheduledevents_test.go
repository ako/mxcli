// SPDX-License-Identifier: Apache-2.0

package linter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/catalog"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
	"github.com/mendixlabs/mxcli/sdk/pages"
	"github.com/mendixlabs/mxcli/sdk/security"
)

// minimalReader is a test double for LintReader that returns empty/nil for
// everything except ListScheduledEvents, which is configurable via a func field.
type minimalReader struct {
	listScheduledEvents func() ([]*model.ScheduledEvent, error)
}

func (m *minimalReader) GetMicroflow(_ model.ID) (*microflows.Microflow, error) {
	return nil, nil
}
func (m *minimalReader) ListMicroflows() ([]*microflows.Microflow, error)       { return nil, nil }
func (m *minimalReader) GetProjectSecurity() (*security.ProjectSecurity, error) { return nil, nil }
func (m *minimalReader) GetNavigation() (*types.NavigationDocument, error)      { return nil, nil }
func (m *minimalReader) ListPages() ([]*pages.Page, error)                      { return nil, nil }
func (m *minimalReader) ListModules() ([]*model.Module, error)                  { return nil, nil }
func (m *minimalReader) ListFolders() ([]*types.FolderInfo, error)              { return nil, nil }
func (m *minimalReader) GetRawUnit(_ model.ID) (map[string]any, error)          { return nil, nil }
func (m *minimalReader) ListScheduledEvents() ([]*model.ScheduledEvent, error) {
	if m.listScheduledEvents != nil {
		return m.listScheduledEvents()
	}
	return nil, nil
}

// TestScheduleSeconds checks the interval a rule sees, exercised through the
// exported ScheduledEvents iterator.
//
// It is derived from the Schedule child, NOT from the stored
// Interval/IntervalType pair: Studio Pro writes that pair and does not keep it
// in sync with Schedule — Workflow Commons 4.11.0 ships an event storing
// 0/"Minute" beside a DaySchedule of 01:00 — so a rule keyed on the legacy pair
// reads a daily job as firing every 0 seconds. TestScheduleSeconds_IgnoresStaleLegacyPair
// below is that exact document.
func TestScheduleSeconds(t *testing.T) {
	tests := []struct {
		name  string
		sched *model.Schedule
		want  int
	}{
		{"minutely x2", &model.Schedule{Kind: model.ScheduleMinute, Multiplier: 2}, 120},
		{"hourly x3", &model.Schedule{Kind: model.ScheduleHour, Multiplier: 3}, 10800},
		{"daily", &model.Schedule{Kind: model.ScheduleDay}, 86400},
		{"weekly, one day", &model.Schedule{Kind: model.ScheduleWeek,
			Weekdays: [7]bool{false, true, false, false, false, false, false}}, 604800},
		// Two selected days means it fires twice a week.
		{"weekly, two days", &model.Schedule{Kind: model.ScheduleWeek,
			Weekdays: [7]bool{false, true, false, false, false, true, false}}, 302400},
		{"monthly", &model.Schedule{Kind: model.ScheduleMonthDate, Multiplier: 1}, 2592000},
		{"yearly", &model.Schedule{Kind: model.ScheduleYearDate}, 31536000},
		// An unstated multiplier is 1, not 0 — a 0 would read as "never".
		{"multiplier defaults to 1", &model.Schedule{Kind: model.ScheduleHour}, 3600},
		{"no schedule", nil, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scheduledEventInterval(t, &model.ScheduledEvent{
				ContainerID: model.ID("mod-1"), Name: "SE", Schedule: tt.sched, Enabled: true,
			}); got != tt.want {
				t.Errorf("IntervalSeconds = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestScheduleSeconds_IgnoresStaleLegacyPair is the regression: this is the
// Workflow Commons document, whose legacy pair disagrees with its Schedule.
// Reading the pair gives 0; reading the Schedule gives a day.
func TestScheduleSeconds_IgnoresStaleLegacyPair(t *testing.T) {
	got := scheduledEventInterval(t, &model.ScheduledEvent{
		ContainerID:  model.ID("mod-1"),
		Name:         "SE_WorkflowAuditTrailRecord_CleanUp",
		Interval:     0,
		IntervalType: "Minute",
		Schedule:     &model.Schedule{Kind: model.ScheduleDay, HourOfDay: 1},
		Enabled:      false,
	})
	if got != 86400 {
		t.Errorf("IntervalSeconds = %d, want 86400 — the legacy Interval/IntervalType pair must not be used", got)
	}
}

// TestScheduledEventExposesSchedule checks the fields a rule can branch on.
func TestScheduledEventExposesSchedule(t *testing.T) {
	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{{
				ContainerID: model.ID("mod-1"), Name: "SE",
				Schedule:  &model.Schedule{Kind: model.ScheduleMonthWeekday, Multiplier: 3},
				OnOverlap: "SkipNext", TimeZone: "Server", Enabled: true,
			}}, nil
		},
	}
	ctx := linter.NewLintContext(newSingleModuleCatalog(t, model.ID("mod-1")), reader)
	found := false
	for se := range ctx.ScheduledEvents() {
		found = true
		if se.Repeat != "MonthWeekday" {
			t.Errorf("Repeat = %q", se.Repeat)
		}
		if se.OnOverlap != "SkipNext" {
			t.Errorf("OnOverlap = %q", se.OnOverlap)
		}
		if se.TimeZone != "Server" {
			t.Errorf("TimeZone = %q", se.TimeZone)
		}
	}
	if !found {
		t.Fatal("no scheduled event yielded")
	}
}

// scheduledEventInterval runs one event through the iterator and returns the
// interval a rule would see.
func scheduledEventInterval(t *testing.T, ev *model.ScheduledEvent) int {
	t.Helper()
	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{ev}, nil
		},
	}
	ctx := linter.NewLintContext(newSingleModuleCatalog(t, ev.ContainerID), reader)
	got := 0
	for se := range ctx.ScheduledEvents() {
		got = se.IntervalSeconds
	}
	return got
}

// newSingleModuleCatalog builds a catalog holding one module, so the iterator
// can resolve the container to a module name.
func newSingleModuleCatalog(t *testing.T, containerID model.ID) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	if _, err := cat.CatalogDB().Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		string(containerID), "MyModule", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	cat.Close()
	return cat
}

func TestScheduledEvents_MicroflowNameResolution(t *testing.T) {
	containerID := model.ID("mod-uuid")
	mfID := model.ID("mf-uuid-1234")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{
				{
					ContainerID:  containerID,
					Name:         "SEWithCatalog",
					MicroflowID:  mfID,
					Interval:     1,
					IntervalType: "Hour",
					Enabled:      true,
				},
				{
					ContainerID:  containerID,
					Name:         "SEWithoutCatalog",
					MicroflowID:  model.ID("unknown-uuid"),
					Interval:     1,
					IntervalType: "Hour",
					Enabled:      false,
				},
			}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		string(containerID), "MyModule", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?)`,
		string(mfID), "ACT_DoSomething", "MyModule.ACT_DoSomething", "MyModule", "Microflow", "default", "s1",
	); err != nil {
		t.Fatalf("insert microflow: %v", err)
	}

	ctx := linter.NewLintContext(cat, reader)
	events := make(map[string]linter.ScheduledEvent)
	for se := range ctx.ScheduledEvents() {
		events[se.Name] = se
	}

	// When the microflow ID is in the catalog, MicroflowName must be the qualified name.
	if got := events["SEWithCatalog"].MicroflowName; got != "MyModule.ACT_DoSomething" {
		t.Errorf("SEWithCatalog.MicroflowName = %q, want %q", got, "MyModule.ACT_DoSomething")
	}

	// When the microflow ID is not in the catalog, fall back to the raw UUID.
	if got := events["SEWithoutCatalog"].MicroflowName; got != "unknown-uuid" {
		t.Errorf("SEWithoutCatalog.MicroflowName = %q, want raw UUID %q", got, "unknown-uuid")
	}
}

func TestScheduledEvents_ExcludedModules(t *testing.T) {
	containerID := model.ID("mod-excl")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{{
				ContainerID: containerID,
				Name:        "ExcludedSE",
				Schedule:    &model.Schedule{Kind: model.ScheduleDay},
				Enabled:     true,
			}}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		string(containerID), "SystemModule", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}

	ctx := linter.NewLintContext(cat, reader)
	ctx.SetExcludedModules([]string{"SystemModule"})

	var count int
	for range ctx.ScheduledEvents() {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events after exclusion, got %d", count)
	}
}

func TestScheduledEvents_IncludedModules(t *testing.T) {
	modA := model.ID("mod-a")
	modB := model.ID("mod-b")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{
				{ContainerID: modA, Name: "SE_A", Schedule: &model.Schedule{Kind: model.ScheduleHour, Multiplier: 1}, Enabled: true},
				{ContainerID: modB, Name: "SE_B", Schedule: &model.Schedule{Kind: model.ScheduleHour, Multiplier: 1}, Enabled: true},
			}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	for _, row := range []struct{ id, name string }{{string(modA), "ModA"}, {string(modB), "ModB"}} {
		if _, err := db.Exec(
			`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
			row.id, row.name, "default", "s1",
		); err != nil {
			t.Fatalf("insert module %s: %v", row.name, err)
		}
	}

	ctx := linter.NewLintContext(cat, reader)
	ctx.SetIncludedModules([]string{"ModA"}) // only ModA is in scope

	var names []string
	for se := range ctx.ScheduledEvents() {
		names = append(names, se.ModuleName)
	}
	if len(names) != 1 || names[0] != "ModA" {
		t.Errorf("expected [ModA], got %v", names)
	}
}

func TestScheduledEvents_NilReader(t *testing.T) {
	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()

	ctx := linter.NewLintContext(cat, nil)
	var count int
	for range ctx.ScheduledEvents() {
		count++
	}
	if count != 0 {
		t.Errorf("expected 0 events with nil reader, got %d", count)
	}
}

// TestStarlarkScheduledEventsBuiltin exercises the scheduled_events() Starlark builtin.
const scheduledEventsRule = `
RULE_ID = "TEST_SE001"
RULE_NAME = "scheduled events builtin"
DESCRIPTION = "exercises the scheduled_events() builtin"
CATEGORY = "test"
SEVERITY = "info"

def check():
    out = []
    for se in scheduled_events():
        out.append(violation(message = "se %s mf %s secs %d enabled %s" % (
            se.qualified_name,
            se.microflow_name,
            se.interval_seconds,
            "yes" if se.enabled else "no",
        )))
    return out
`

func TestStarlarkScheduledEventsBuiltin(t *testing.T) {
	containerID := model.ID("mod-starlark")
	mfID := model.ID("mf-starlark-uuid")

	reader := &minimalReader{
		listScheduledEvents: func() ([]*model.ScheduledEvent, error) {
			return []*model.ScheduledEvent{{
				ContainerID: containerID,
				Name:        "DailySE",
				MicroflowID: mfID,
				// The interval a rule sees comes from Schedule. The legacy pair
				// is left deliberately inconsistent here (it claims 2 days) to
				// prove the builtin does not read it.
				Interval:     2,
				IntervalType: "Day",
				Schedule:     &model.Schedule{Kind: model.ScheduleHour, Multiplier: 2},
				Enabled:      true,
			}}, nil
		},
	}

	cat, err := catalog.NewFromFile(filepath.Join(t.TempDir(), "cat.db"))
	if err != nil {
		t.Fatalf("NewFromFile: %v", err)
	}
	defer cat.Close()
	db := cat.CatalogDB()

	if _, err := db.Exec(
		`INSERT INTO modules_data (Id, Name, ProjectId, SnapshotId) VALUES (?,?,?,?)`,
		string(containerID), "Billing", "default", "s1",
	); err != nil {
		t.Fatalf("insert module: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO microflows_data (Id, Name, QualifiedName, ModuleName, MicroflowType, ProjectId, SnapshotId)
		 VALUES (?,?,?,?,?,?,?)`,
		string(mfID), "SUB_DailyJob", "Billing.SUB_DailyJob", "Billing", "Microflow", "default", "s1",
	); err != nil {
		t.Fatalf("insert microflow: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "se_test.star")
	if err := os.WriteFile(path, []byte(scheduledEventsRule), 0644); err != nil {
		t.Fatal(err)
	}

	rule, err := linter.LoadStarlarkRule(path)
	if err != nil {
		t.Fatalf("LoadStarlarkRule: %v", err)
	}

	ctx := linter.NewLintContext(cat, reader)
	violations := rule.Check(ctx)

	var msgs []string
	for _, v := range violations {
		msgs = append(msgs, v.Message)
	}
	joined := strings.Join(msgs, "\n")

	for _, want := range []string{
		"se Billing.DailySE",
		"mf Billing.SUB_DailyJob",
		"secs 7200", // 2 hours, from the Schedule — NOT the 172800 the legacy pair implies
		"enabled yes",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in violations:\n%s", want, joined)
		}
	}
}
