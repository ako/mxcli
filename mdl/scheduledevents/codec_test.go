// SPDX-License-Identifier: Apache-2.0

package scheduledevents

import (
	"encoding/hex"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

// Three complete, unedited scheduled-event documents lifted out of Mendix's own
// marketplace modules — the only Studio Pro-authored references available
// without Studio Pro. Between them they cover both schedule variants that ship
// in Mendix modules (Day and Hour), both TimeZone values, both Enabled values,
// documented and undocumented, and a zero Interval.
//
//	oidcHex   OIDC SSO 4.6.0            CleanupOldAuthAttempts        HourSchedule
//	samlHex   SAML 4.2.1                SE_LogCleanUp                 DaySchedule, TimeZone Server
//	wfcHex    Workflow Commons 4.11.0   SE_WorkflowAuditTrailRecord…  DaySchedule, Interval 0
const (
	oidcHex = "c00100000524494400100000000059533bc5d678bb40835cd6028304521f022454797065001f0000005363686564756c65644576656e7473245363686564756c65644576656e740002446f63756d656e746174696f6e00010000000008456e61626c65640001084578636c756465640000024578706f72744c6576656c000700000048696464656e0012496e74657276616c00010000000000000002496e74657276616c547970650005000000486f757200024d6963726f666c6f7700200000004f4944432e5355425f436c65616e75704f6c6441757468417474656d70747300024e616d650017000000436c65616e75704f6c6441757468417474656d70747300024f6e4f7665726c6170000a00000044656c61794e65787400035363686564756c65007100000005244944001000000000c154dede71230641b986faef8e304829022454797065001d0000005363686564756c65644576656e747324486f75725363686564756c6500124d696e7574654f6666736574001700000000000000124d756c7469706c696572000100000000000000000953746172744461746554696d650058a17738730100000254696d655a6f6e6500040000005554430000"
	samlHex = "63020000052449440010000000007e890ddd9ec5a444b417ef5992c27101022454797065001f0000005363686564756c65644576656e7473245363686564756c65644576656e740002446f63756d656e746174696f6e00b80000005468697320616374696f6e2077696c6c20636c65616e757020746865206c6f676c696e65732e20546865206c6f677320656e74726965732077696c6c20626520617263686976656420666f722074686520616d6f756e74206f6620646179732073706563696669656420696e2074686520636f6e66696775726174696f6e2c20746865206c6f6720656e74726965732074686174207265616368656420746865206c696d69742077696c6c2062652064656c657465642e0008456e61626c65640000084578636c756465640000024578706f72744c6576656c000700000048696464656e0012496e74657276616c00010000000000000002496e74657276616c54797065000400000044617900024d6963726f666c6f77001500000053414d4c32302e53455f4c6f67436c65616e557000024e616d65000e00000053455f4c6f67436c65616e557000024f6e4f7665726c6170000a00000044656c61794e65787400035363686564756c65006f00000005244944001000000000df856dc51b45c24d89325077b18be82b022454797065001c0000005363686564756c65644576656e7473244461795363686564756c650012486f75724f66446179000400000000000000124d696e7574654f66486f7572000000000000000000000953746172744461746554696d6500002ea2b8390100000254696d655a6f6e6500070000005365727665720000"
	wfcHex  = "e1010000052449440010000000003d7bd7ba3c94a84d816c130543adf825022454797065001f0000005363686564756c65644576656e7473245363686564756c65644576656e740002446f63756d656e746174696f6e00010000000008456e61626c65640000084578636c756465640000024578706f72744c6576656c000700000048696464656e0012496e74657276616c00000000000000000002496e74657276616c5479706500070000004d696e75746500024d6963726f666c6f770034000000576f726b666c6f77436f6d6d6f6e732e53455f576f726b666c6f774175646974547261696c5265636f72645f436c65616e557000024e616d65002400000053455f576f726b666c6f774175646974547261696c5265636f72645f436c65616e557000024f6e4f7665726c6170000a00000044656c61794e65787400035363686564756c65006f0000000524494400100000000081a8e58306a48748bdc93d240d67c64b022454797065001c0000005363686564756c65644576656e7473244461795363686564756c650012486f75724f66446179000100000000000000124d696e7574654f66486f7572000000000000000000000953746172744461746554696d6500db6b4292900100000254696d655a6f6e6500040000005554430000"
)

func mustDecode(t *testing.T, h string) bson.M {
	t.Helper()
	raw, err := hex.DecodeString(h)
	if err != nil {
		t.Fatalf("hex: %v", err)
	}
	var doc bson.M
	if err := bson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return doc
}

// TestParseStudioProDocuments checks the reader against the real documents,
// including the two properties the generated metamodel gets wrong: every
// integer is int64 (gen says int32) and StartDateTime is a BSON datetime (gen
// says string).
func TestParseStudioProDocuments(t *testing.T) {
	tests := []struct {
		name      string
		hexDoc    string
		want      model.ScheduledEvent
		wantSched model.Schedule
	}{
		{
			name:   "OIDC CleanupOldAuthAttempts",
			hexDoc: oidcHex,
			want: model.ScheduledEvent{
				Name: "CleanupOldAuthAttempts", MicroflowID: "OIDC.SUB_CleanupOldAuthAttempts",
				Enabled: true, Interval: 1, IntervalType: "Hour",
				OnOverlap: "DelayNext", TimeZone: "UTC", ExportLevel: "Hidden",
			},
			wantSched: model.Schedule{Kind: model.ScheduleHour, Multiplier: 1, MinuteOffset: 23},
		},
		{
			name:   "SAML SE_LogCleanUp",
			hexDoc: samlHex,
			want: model.ScheduledEvent{
				Name: "SE_LogCleanUp", MicroflowID: "SAML20.SE_LogCleanUp",
				Enabled: false, Interval: 1, IntervalType: "Day",
				OnOverlap: "DelayNext", TimeZone: "Server", ExportLevel: "Hidden",
			},
			wantSched: model.Schedule{Kind: model.ScheduleDay, HourOfDay: 4, MinuteOfHour: 0},
		},
		{
			name:   "Workflow Commons cleanup",
			hexDoc: wfcHex,
			want: model.ScheduledEvent{
				Name: "SE_WorkflowAuditTrailRecord_CleanUp",
				// Interval 0 / "Minute" alongside a DaySchedule of 01:00: Studio
				// Pro does not keep the legacy pair in sync with Schedule, which
				// is why the writer carries them through instead of deriving them.
				MicroflowID: "WorkflowCommons.SE_WorkflowAuditTrailRecord_CleanUp",
				Enabled:     false, Interval: 0, IntervalType: "Minute",
				OnOverlap: "DelayNext", TimeZone: "UTC", ExportLevel: "Hidden",
			},
			wantSched: model.Schedule{Kind: model.ScheduleDay, HourOfDay: 1, MinuteOfHour: 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := Parse(mustDecode(t, tt.hexDoc), "id-1", "mod-1")
			if ev.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", ev.Name, tt.want.Name)
			}
			if ev.MicroflowID != tt.want.MicroflowID {
				t.Errorf("MicroflowID = %q, want %q", ev.MicroflowID, tt.want.MicroflowID)
			}
			if ev.Enabled != tt.want.Enabled {
				t.Errorf("Enabled = %v", ev.Enabled)
			}
			if ev.Interval != tt.want.Interval {
				t.Errorf("Interval = %d, want %d (stored as int64)", ev.Interval, tt.want.Interval)
			}
			if ev.IntervalType != tt.want.IntervalType {
				t.Errorf("IntervalType = %q", ev.IntervalType)
			}
			if ev.OnOverlap != tt.want.OnOverlap {
				t.Errorf("OnOverlap = %q", ev.OnOverlap)
			}
			if ev.TimeZone != tt.want.TimeZone {
				t.Errorf("TimeZone = %q", ev.TimeZone)
			}
			if ev.ExportLevel != tt.want.ExportLevel {
				t.Errorf("ExportLevel = %q", ev.ExportLevel)
			}
			if ev.StartDateTime == nil {
				t.Error("StartDateTime not read — it is a BSON datetime, not the string gen declares")
			}
			if ev.Schedule == nil {
				t.Fatal("Schedule not read")
			}
			if *ev.Schedule != tt.wantSched {
				t.Errorf("Schedule = %+v, want %+v", *ev.Schedule, tt.wantSched)
			}
		})
	}
}

// TestSerializeMatchesStudioProDocuments is the shape pin: read each real
// document, write it back, and require the result to be identical apart from
// the two generated $ID values. It compares raw BSON elements, so key ORDER and
// BSON TYPE (int64 vs int32, datetime vs string) are covered as well as values —
// none of which a field-by-field assertion would catch.
func TestSerializeMatchesStudioProDocuments(t *testing.T) {
	for _, tt := range []struct{ name, hexDoc string }{
		{"OIDC", oidcHex},
		{"SAML", samlHex},
		{"WorkflowCommons", wfcHex},
	} {
		t.Run(tt.name, func(t *testing.T) {
			originalBytes, err := hex.DecodeString(tt.hexDoc)
			if err != nil {
				t.Fatalf("hex: %v", err)
			}
			ev := Parse(mustDecode(t, tt.hexDoc), "11111111-1111-1111-1111-111111111111", "mod-1")

			out, err := Serialize(ev)
			if err != nil {
				t.Fatalf("Serialize: %v", err)
			}
			assertSameExceptIDs(t, "", bson.Raw(originalBytes), bson.Raw(out))
		})
	}
}

// assertSameExceptIDs compares two raw documents element by element, in order,
// requiring the same keys in the same positions with the same BSON types and
// values. $ID is compared for presence only: it is regenerated on every write.
func assertSameExceptIDs(t *testing.T, path string, want, got bson.Raw) {
	t.Helper()
	wantEls, err := want.Elements()
	if err != nil {
		t.Fatalf("%s: elements: %v", path, err)
	}
	gotEls, err := got.Elements()
	if err != nil {
		t.Fatalf("%s: elements: %v", path, err)
	}
	if len(wantEls) != len(gotEls) {
		t.Fatalf("%s: wrote %d properties, Studio Pro wrote %d\n  want %v\n  got  %v",
			path, len(gotEls), len(wantEls), keysOf(wantEls), keysOf(gotEls))
	}
	for i, we := range wantEls {
		ge := gotEls[i]
		if we.Key() != ge.Key() {
			t.Errorf("%sproperty %d is %q, want %q (order differs)", path, i, ge.Key(), we.Key())
			continue
		}
		wv, gv := we.Value(), ge.Value()
		if wv.Type != gv.Type {
			t.Errorf("%s%s: BSON type %s, want %s", path, we.Key(), gv.Type, wv.Type)
			continue
		}
		if we.Key() == "$ID" {
			continue
		}
		if wv.Type == bson.TypeEmbeddedDocument {
			assertSameExceptIDs(t, path+we.Key()+".", wv.Document(), gv.Document())
			continue
		}
		if !wv.Equal(gv) {
			t.Errorf("%s%s = %v, want %v", path, we.Key(), gv, wv)
		}
	}
}

func keysOf(els []bson.RawElement) []string {
	out := make([]string, len(els))
	for i, e := range els {
		out[i] = e.Key()
	}
	return out
}

// TestSerializeSchedule_AllVariants covers the six variants no Mendix module
// ships, so the field sets are metamodel-derived rather than observed. The
// assertion that matters is that each variant writes ONLY its own fields: a
// merged field set is the shape that mxbuild accepts and Studio Pro refuses to
// open.
func TestSerializeSchedule_AllVariants(t *testing.T) {
	full := model.Schedule{
		Multiplier: 2, MinuteOffset: 5, MonthOffset: 1,
		HourOfDay: 6, MinuteOfHour: 30, DayOfMonth: 15, Month: 3,
		DaySelector: "Last", Weekday: "Friday",
		Weekdays: [7]bool{false, true, false, false, false, true, false},
	}
	wantKeys := map[model.ScheduleKind][]string{
		model.ScheduleMinute:       {"Multiplier"},
		model.ScheduleHour:         {"MinuteOffset", "Multiplier"},
		model.ScheduleDay:          {"HourOfDay", "MinuteOfHour"},
		model.ScheduleWeek:         {"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "HourOfDay", "MinuteOfHour"},
		model.ScheduleMonthDate:    {"Multiplier", "MonthOffset", "DayOfMonth", "HourOfDay", "MinuteOfHour"},
		model.ScheduleMonthWeekday: {"Multiplier", "MonthOffset", "DaySelector", "Weekday", "HourOfDay", "MinuteOfHour"},
		model.ScheduleYearDate:     {"Month", "DayOfMonth", "HourOfDay", "MinuteOfHour"},
		model.ScheduleYearWeekday:  {"Month", "DaySelector", "Weekday", "HourOfDay", "MinuteOfHour"},
	}

	for kind, keys := range wantKeys {
		t.Run(string(kind), func(t *testing.T) {
			s := full
			s.Kind = kind
			doc, err := SerializeSchedule(&s)
			if err != nil {
				t.Fatalf("SerializeSchedule: %v", err)
			}
			got := map[string]bool{}
			for _, e := range doc {
				got[e.Key] = true
			}
			if !got["$ID"] || !got["$Type"] {
				t.Error("missing $ID/$Type")
			}
			delete(got, "$ID")
			delete(got, "$Type")

			for _, k := range keys {
				if !got[k] {
					t.Errorf("missing %s", k)
				}
				delete(got, k)
			}
			for k := range got {
				t.Errorf("%s is not a property of %s — writing it produces a document Studio Pro cannot open", k, ScheduleTypeNames[kind])
			}
		})
	}
}

func TestSerializeSchedule_UnknownKindIsRefused(t *testing.T) {
	if _, err := SerializeSchedule(&model.Schedule{Kind: "Fortnightly"}); err == nil {
		t.Fatal("expected an error for an unknown schedule kind")
	}
}

// TestSerialize_Defaults documents the values supplied when the caller leaves
// them empty, all taken from the reference documents.
func TestSerialize_Defaults(t *testing.T) {
	ev := &model.ScheduledEvent{Name: "SE_Plain", MicroflowID: "M.MF"}
	ev.ID = "22222222-2222-2222-2222-222222222222"

	out, err := Serialize(ev)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	var doc bson.M
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc["ExportLevel"] != "Hidden" {
		t.Errorf("ExportLevel = %v, want Hidden", doc["ExportLevel"])
	}
	if doc["OnOverlap"] != "DelayNext" {
		t.Errorf("OnOverlap = %v, want DelayNext (SkipNext silently drops a run)", doc["OnOverlap"])
	}
	if doc["TimeZone"] != "UTC" {
		t.Errorf("TimeZone = %v, want UTC", doc["TimeZone"])
	}
	if _, ok := doc["Interval"].(int64); !ok {
		t.Errorf("Interval = %#v, want an int64 — Studio Pro writes int64, gen declares int32", doc["Interval"])
	}
	if _, ok := doc["Schedule"]; ok {
		t.Error("a nil schedule must not be written as an empty child")
	}
}

func TestSerialize_StartDateTimeRoundTrips(t *testing.T) {
	want := time.Date(2024, 7, 8, 12, 12, 24, 923_000_000, time.UTC)
	ev := &model.ScheduledEvent{Name: "SE", MicroflowID: "M.MF", StartDateTime: &want}
	ev.ID = "33333333-3333-3333-3333-333333333333"

	out, err := Serialize(ev)
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	var doc bson.M
	if err := bson.Unmarshal(out, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := Parse(doc, "id", "mod")
	if got.StartDateTime == nil || !got.StartDateTime.Equal(want) {
		t.Errorf("StartDateTime = %v, want %v", got.StartDateTime, want)
	}
}
