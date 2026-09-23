// SPDX-License-Identifier: Apache-2.0

package settingsoverlay

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
)

func TestArrayMarker(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want int32
	}{
		{"int32 marker", bson.A{int32(3), bson.M{}}, 3},
		{"int64 marker", bson.A{int64(2)}, 2},
		{"int marker", bson.A{5}, 5},
		{"empty array falls back", bson.A{}, 7},
		{"not an array falls back", "nope", 7},
		{"nil falls back", nil, 7},
		{"unmarked array falls back", bson.A{bson.M{"$Type": "x"}}, 7},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ArrayMarker(tc.in, 7); got != tc.want {
				t.Errorf("ArrayMarker(%#v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestArrayElements_SkipsMarkerAndScalars(t *testing.T) {
	in := bson.A{int32(3), bson.M{"Name": "a"}, "junk", map[string]any{"Name": "b"}}
	got := ArrayElements(in)
	if len(got) != 2 {
		t.Fatalf("ArrayElements returned %d elements, want 2: %#v", len(got), got)
	}
	if got[0]["Name"] != "a" || got[1]["Name"] != "b" {
		t.Errorf("ArrayElements = %#v", got)
	}
}

// TestServerConfiguration_PreservesUnknownKeys pins the core of #801: keys the
// semantic model does not carry must pass through the overlay untouched.
func TestServerConfiguration_PreservesUnknownKeys(t *testing.T) {
	raw := map[string]any{
		"$ID":            "existing-id-sentinel",
		"$Type":          "Settings$ServerConfiguration",
		"Name":           "Default",
		"OpenAdminPort":  true,
		"OpenHttpPort":   true,
		"CustomSettings": bson.A{int32(3), bson.M{"Name": "Foo", "Value": "1"}},
		"Tracing":        bson.M{"$Type": "Settings$Tracing", "Level": "Feedback"},
		"SomeFutureKey":  "keep me",
		"ConstantValues": bson.A{int32(3)},
	}
	cfg := &model.ServerConfiguration{Name: "Default", HttpPortNumber: 8123}

	got := ServerConfiguration(cfg, raw, nil)

	if got["OpenAdminPort"] != true || got["OpenHttpPort"] != true {
		t.Errorf("Open*Port reset: OpenAdminPort=%#v OpenHttpPort=%#v", got["OpenAdminPort"], got["OpenHttpPort"])
	}
	if len(ArrayElements(got["CustomSettings"])) != 1 {
		t.Errorf("CustomSettings dropped: %#v", got["CustomSettings"])
	}
	if _, ok := AsMap(got["Tracing"]); !ok {
		t.Errorf("Tracing nulled: %#v", got["Tracing"])
	}
	if got["SomeFutureKey"] != "keep me" {
		t.Errorf("unknown key dropped: %#v", got["SomeFutureKey"])
	}
	if got["$ID"] != "existing-id-sentinel" {
		t.Errorf("$ID rewritten: %#v", got["$ID"])
	}
	if got["HttpPortNumber"] != int64(8123) {
		t.Errorf("HttpPortNumber = %#v, want 8123", got["HttpPortNumber"])
	}
	if m := ArrayMarker(got["ConstantValues"], -1); m != 3 {
		t.Errorf("ConstantValues marker = %d, want the stored 3", m)
	}
}

func TestConstantValues_UpdatesNestedShapeInPlace(t *testing.T) {
	raw := bson.A{
		int32(3),
		bson.M{
			"$ID":        "cv-id",
			"$Type":      "Settings$ConstantValue",
			"ConstantId": "Mod.C1",
			"Value":      "stale-flat",
			"SharedOrPrivateValue": bson.M{
				"$Type": "Settings$SharedValue",
				"Value": "old",
			},
		},
	}
	got := ConstantValues([]*model.ConstantValue{{ConstantId: "Mod.C1", Value: "new"}}, raw)

	cvs := ArrayElements(got)
	if len(cvs) != 1 {
		t.Fatalf("got %d overrides, want 1: %#v", len(cvs), got)
	}
	shared, ok := AsMap(cvs[0]["SharedOrPrivateValue"])
	if !ok {
		t.Fatalf("SharedOrPrivateValue dropped: %#v", cvs[0])
	}
	if shared["Value"] != "new" {
		t.Errorf("nested Value = %#v, want new", shared["Value"])
	}
	// The stale flat sibling is cleared so the reader cannot report it instead.
	if cvs[0]["Value"] != "" {
		t.Errorf("flat Value = %#v, want cleared", cvs[0]["Value"])
	}
	if cvs[0]["$ID"] != "cv-id" {
		t.Errorf("$ID rewritten: %#v", cvs[0]["$ID"])
	}
}

func TestConstantValues_KeepsLegacyFlatShape(t *testing.T) {
	raw := bson.A{
		int32(3),
		bson.M{"$ID": "cv-id", "$Type": "Settings$ConstantValue", "ConstantId": "Mod.C1", "Value": "old"},
	}
	cvs := ArrayElements(ConstantValues([]*model.ConstantValue{{ConstantId: "Mod.C1", Value: "new"}}, raw))
	if len(cvs) != 1 {
		t.Fatalf("got %d overrides, want 1", len(cvs))
	}
	if cvs[0]["Value"] != "new" {
		t.Errorf("flat Value = %#v, want new", cvs[0]["Value"])
	}
	if _, ok := cvs[0]["SharedOrPrivateValue"]; ok {
		t.Errorf("a nested value was added to a flat-only override: %#v", cvs[0])
	}
}

// TestConstantValues_NewOverrideIsNested: a brand-new override has no stored shape,
// so it must be written in the shape Studio Pro and mxbuild actually read.
func TestConstantValues_NewOverrideIsNested(t *testing.T) {
	cvs := ArrayElements(ConstantValues([]*model.ConstantValue{{ConstantId: "Mod.New", Value: "v"}}, bson.A{int32(3)}))
	if len(cvs) != 1 {
		t.Fatalf("got %d overrides, want 1", len(cvs))
	}
	if cvs[0]["ConstantId"] != "Mod.New" {
		t.Errorf("ConstantId = %#v", cvs[0]["ConstantId"])
	}
	shared, ok := AsMap(cvs[0]["SharedOrPrivateValue"])
	if !ok {
		t.Fatalf("new override is not nested: %#v", cvs[0])
	}
	if shared["Value"] != "v" || shared["$Type"] != "Settings$SharedValue" {
		t.Errorf("SharedOrPrivateValue = %#v", shared)
	}
	if _, ok := cvs[0]["Value"]; ok {
		t.Errorf("new override also wrote a flat Value: %#v", cvs[0])
	}
}

// TestConstantValues_MatchesByConstantKey covers the "Constant" spelling the gen type
// binds, alongside the "ConstantId" Studio Pro writes.
func TestConstantValues_MatchesByConstantKey(t *testing.T) {
	raw := bson.A{
		int32(3),
		bson.M{"$Type": "Settings$ConstantValue", "Constant": "Mod.C1", "Value": "old"},
	}
	cvs := ArrayElements(ConstantValues([]*model.ConstantValue{{ConstantId: "Mod.C1", Value: "new"}}, raw))
	if len(cvs) != 1 || cvs[0]["Value"] != "new" {
		t.Errorf("override matched by Constant key not updated in place: %#v", cvs)
	}
}

func TestConfigurations_MatchesByNameAndDrops(t *testing.T) {
	part := map[string]any{
		"$Type": "Settings$ConfigurationSettings",
		"Configurations": bson.A{
			int32(3),
			bson.M{"$ID": "id-default", "Name": "Default", "SomeFutureKey": "a"},
			bson.M{"$ID": "id-prod", "Name": "Production", "SomeFutureKey": "b"},
		},
	}
	cs := &model.ConfigurationSettings{
		// Production dropped; Default matched case-insensitively.
		Configurations: []*model.ServerConfiguration{{Name: "default", HttpPortNumber: 9000}},
	}

	got := Configurations(cs, part)
	cfgs := ArrayElements(got["Configurations"])
	if len(cfgs) != 1 {
		t.Fatalf("got %d configurations, want 1 (Production dropped): %#v", len(cfgs), got["Configurations"])
	}
	if cfgs[0]["$ID"] != "id-default" {
		t.Errorf("matched the wrong raw configuration: %#v", cfgs[0])
	}
	if cfgs[0]["SomeFutureKey"] != "a" {
		t.Errorf("unknown key dropped on the matched configuration: %#v", cfgs[0])
	}
	if m := ArrayMarker(got["Configurations"], -1); m != 3 {
		t.Errorf("Configurations marker = %d, want the stored 3", m)
	}
}

// TestConfigurations_NewConfigurationUsesSiblingShape: CREATE CONFIGURATION has no
// raw document, so the shape is taken from a sibling — minus its identity and its
// per-configuration collections.
func TestConfigurations_NewConfigurationUsesSiblingShape(t *testing.T) {
	part := map[string]any{
		"Configurations": bson.A{
			int32(3),
			bson.M{
				"$ID":            "id-default",
				"Name":           "Default",
				"OpenAdminPort":  true,
				"CustomSettings": bson.A{int32(3), bson.M{"Name": "Foo"}},
				"ConstantValues": bson.A{int32(3), bson.M{"ConstantId": "Mod.C1", "Value": "x"}},
				"Tracing":        bson.M{"$Type": "Settings$Tracing"},
			},
		},
	}
	cs := &model.ConfigurationSettings{
		Configurations: []*model.ServerConfiguration{
			{Name: "Default"},
			{Name: "Acceptance", DatabaseType: "PostgreSQL"},
		},
	}

	cfgs := ArrayElements(Configurations(cs, part)["Configurations"])
	if len(cfgs) != 2 {
		t.Fatalf("got %d configurations, want 2", len(cfgs))
	}
	var added map[string]any
	for _, c := range cfgs {
		if c["Name"] == "Acceptance" {
			added = c
		}
	}
	if added == nil {
		t.Fatalf("new configuration not written: %#v", cfgs)
	}
	if added["$ID"] == nil || added["$ID"] == "id-default" {
		t.Errorf("new configuration must get a fresh $ID, got %#v", added["$ID"])
	}
	if added["OpenAdminPort"] != true {
		t.Errorf("sibling shape not inherited: OpenAdminPort=%#v", added["OpenAdminPort"])
	}
	if got := ArrayElements(added["CustomSettings"]); len(got) != 0 {
		t.Errorf("new configuration inherited the sibling's custom settings: %#v", got)
	}
	if got := ArrayElements(added["ConstantValues"]); len(got) != 0 {
		t.Errorf("new configuration inherited the sibling's constant overrides: %#v", got)
	}
	if added["DatabaseType"] != "PostgreSQL" {
		t.Errorf("DatabaseType = %#v, want PostgreSQL", added["DatabaseType"])
	}
	// The sibling itself is untouched by having been used as a template.
	for _, c := range cfgs {
		if c["Name"] != "Default" {
			continue
		}
		if len(ArrayElements(c["CustomSettings"])) != 1 {
			t.Errorf("template sibling lost its custom settings: %#v", c["CustomSettings"])
		}
	}
}

// TestServerConfiguration_NoSiblings covers a project with no configuration at all:
// the fallback field set must still carry the keys Studio Pro expects.
func TestServerConfiguration_NoSiblings(t *testing.T) {
	got := ServerConfiguration(&model.ServerConfiguration{Name: "Default", HttpPortNumber: 8080}, nil, nil)
	for _, key := range []string{"$ID", "$Type", "Name", "CustomSettings", "ConstantValues", "OpenAdminPort", "OpenHttpPort"} {
		if _, ok := got[key]; !ok {
			t.Errorf("fallback configuration is missing %q: %#v", key, got)
		}
	}
	// The tracing property is version-specific — Mendix 11.6 stores "Tracing",
	// 11.12 "OpenTelemetry" — and with no sibling to copy there is nothing to tell
	// the two apart. Writing either spelling risks a property the version's
	// metamodel does not define, which is what Studio Pro fails to resolve
	// (mendixlabs/mxcli#759); an absent optional property is filled in on load.
	for _, key := range []string{"Tracing", "OpenTelemetry"} {
		if v, ok := got[key]; ok {
			t.Errorf("fallback configuration invented %q = %#v", key, v)
		}
	}
	if m := ArrayMarker(got["CustomSettings"], -1); m != DefaultListMarker {
		t.Errorf("CustomSettings marker = %d, want %d", m, DefaultListMarker)
	}
}

func TestSafeInt64_Bounds(t *testing.T) {
	const maxSafe = int64(1) << 53
	if got := SafeInt64(8080); got != 8080 {
		t.Errorf("SafeInt64(8080) = %d", got)
	}
	if got := SafeInt64(int(maxSafe) + 10); got != maxSafe {
		t.Errorf("SafeInt64 above range = %d, want clamp to %d", got, maxSafe)
	}
	if got := SafeInt64(-int(maxSafe) - 10); got != -maxSafe {
		t.Errorf("SafeInt64 below range = %d, want clamp to %d", got, -maxSafe)
	}
}

// TestJavaVersionKey_FollowsDocument covers mendixlabs/mxcli#759: Mendix renamed
// the runtime Java version property between 11.6 ("JavaVersion" = "Java21") and
// 11.12 ("JavaMajorVersion" = "21"). The overlay must write back to the key the
// document already carries and invent neither, because a property the version's
// metamodel does not define is what Studio Pro fails to resolve on open.
func TestJavaVersionKey_FollowsDocument(t *testing.T) {
	tests := []struct {
		name    string
		raw     map[string]any
		wantKey string
		wantVal string
	}{
		{
			name:    "mendix 11.6",
			raw:     map[string]any{"JavaVersion": "Java21"},
			wantKey: "JavaVersion",
			wantVal: "Java21",
		},
		{
			name:    "mendix 11.12",
			raw:     map[string]any{"JavaMajorVersion": "21"},
			wantKey: "JavaMajorVersion",
			wantVal: "21",
		},
		{
			name:    "neither key",
			raw:     map[string]any{"HashAlgorithm": "BCrypt"},
			wantKey: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := JavaVersionKey(tc.raw); got != tc.wantKey {
				t.Fatalf("JavaVersionKey = %q, want %q", got, tc.wantKey)
			}
			if got := JavaVersion(tc.raw); got != tc.wantVal {
				t.Errorf("JavaVersion = %q, want %q", got, tc.wantVal)
			}

			SetJavaVersion(tc.raw, "written")
			if tc.wantKey == "" {
				for _, k := range []string{"JavaVersion", "JavaMajorVersion"} {
					if v, ok := tc.raw[k]; ok {
						t.Errorf("SetJavaVersion invented %q = %#v", k, v)
					}
				}
				return
			}
			if tc.raw[tc.wantKey] != "written" {
				t.Errorf("%s = %#v, want %q", tc.wantKey, tc.raw[tc.wantKey], "written")
			}
			for _, k := range []string{"JavaVersion", "JavaMajorVersion"} {
				if k == tc.wantKey {
					continue
				}
				if v, ok := tc.raw[k]; ok {
					t.Errorf("SetJavaVersion also wrote %q = %#v", k, v)
				}
			}
		})
	}
}

// TestJavaVersionValue_MatchesKeyDialect is the follow-up to
// TestJavaVersionKey_FollowsDocument: the rename changed the value format along
// with the key, so following only the key still produced a project Mendix 11.12
// refuses to load. Its JavaVersionExtensions.fromString parses JavaMajorVersion as
// a bare major and throws ArgumentOutOfRangeException ("majorVersion is an
// unsupported value: Java21") on the 11.6 spelling.
func TestJavaVersionValue_MatchesKeyDialect(t *testing.T) {
	tests := []struct {
		key  string
		in   string
		want string
	}{
		{JavaMajorVersionKey, "Java21", "21"},
		{JavaMajorVersionKey, "21", "21"},
		{JavaMajorVersionKey, "java17", "17"},
		{JavaVersionEnumKey, "Java21", "Java21"},
		{JavaVersionEnumKey, "21", "Java21"},
		{JavaVersionEnumKey, " 17 ", "Java17"},
		// Not a recognisable version: passed through so the typo surfaces as a
		// Mendix error rather than as a silently mangled setting.
		{JavaMajorVersionKey, "Temurin", "Temurin"},
		{JavaVersionEnumKey, "Java-21", "Java-21"},
		{JavaMajorVersionKey, "", ""},
	}
	for _, tc := range tests {
		if got := JavaVersionValue(tc.key, tc.in); got != tc.want {
			t.Errorf("JavaVersionValue(%q, %q) = %q, want %q", tc.key, tc.in, got, tc.want)
		}
	}
}

// TestSetJavaVersion_ConvertsToStoredDialect: one MDL statement must work on both
// Mendix versions, whichever spelling the author used.
func TestSetJavaVersion_ConvertsToStoredDialect(t *testing.T) {
	mendix1112 := map[string]any{JavaMajorVersionKey: "21"}
	SetJavaVersion(mendix1112, "Java17")
	if got := mendix1112[JavaMajorVersionKey]; got != "17" {
		t.Errorf("%s = %#v, want %q", JavaMajorVersionKey, got, "17")
	}

	mendix116 := map[string]any{JavaVersionEnumKey: "Java21"}
	SetJavaVersion(mendix116, "17")
	if got := mendix116[JavaVersionEnumKey]; got != "Java17" {
		t.Errorf("%s = %#v, want %q", JavaVersionEnumKey, got, "Java17")
	}
}

// TestWorkflowGroups_ShapePinnedToStudioPro pins the document mxcli writes for a
// workflow group against what a real Mendix project stores.
//
// The reference is a blank 11.13.0 project created with `mxcli new`: its
// workflows settings part stores `Groups: [2]` — an empty typed array whose
// marker is 2, not the 3 the other settings child lists use. Settings$WorkflowGroup
// declares exactly Name and Description (modelsdk/gen, generated/metamodel and
// mendixmodelsdk 4.115.0 all agree, and nothing else appears in the type), so a
// group document is those two keys plus $ID and $Type and nothing more. Writing a
// key the type does not declare is the mendixlabs/mxcli#759 failure shape: mxbuild
// accepts it and Studio Pro cannot open the project.
func TestWorkflowGroups_ShapePinnedToStudioPro(t *testing.T) {
	raw := map[string]any{
		"$Type":                     "Settings$WorkflowsProjectSettingsPart",
		"DefaultTaskParallelism":    int64(3),
		"Groups":                    bson.A{int32(2)},
		"OnWorkflowEvent":           bson.A{int32(2)},
		"UserEntity":                "System.User",
		"WorkflowEngineParallelism": int64(5),
	}
	ws := &model.WorkflowsSettings{
		Groups: []model.WorkflowGroup{{Name: "Approvers", Description: "Primary approval group"}},
	}

	out := WorkflowGroups(ws, raw)

	groups, ok := out["Groups"].(bson.A)
	if !ok || len(groups) != 2 {
		t.Fatalf("Groups = %#v, want a 2-element array (marker + one group)", out["Groups"])
	}
	if groups[0] != int32(2) {
		t.Errorf("Groups marker = %#v, want int32(2) — the marker the stored list carried", groups[0])
	}
	g, ok := groups[1].(map[string]any)
	if !ok {
		t.Fatalf("group = %#v, want a document", groups[1])
	}
	if g["$Type"] != "Settings$WorkflowGroup" {
		t.Errorf("$Type = %v, want Settings$WorkflowGroup", g["$Type"])
	}
	if g["Name"] != "Approvers" || g["Description"] != "Primary approval group" {
		t.Errorf("group = %#v, want Name/Description from the model", g)
	}
	if g["$ID"] == nil {
		t.Error("a new group got no $ID")
	}
	// Exactly the four keys and no fifth: a property Settings$WorkflowGroup does
	// not declare makes a document Studio Pro refuses to open, and mxbuild — which
	// tolerates unknown properties — would never report it.
	for k := range g {
		switch k {
		case "$ID", "$Type", "Name", "Description":
		default:
			t.Errorf("group carries undeclared key %q = %#v", k, g[k])
		}
	}
	// Every other key of the settings part passes through untouched.
	if out["UserEntity"] != "System.User" || out["OnWorkflowEvent"] == nil {
		t.Errorf("overlay disturbed a sibling key: %#v", out)
	}
}

// TestWorkflowGroups_PreservesStoredIdentityAndUnknownKeys covers the overlay half
// of guard-don't-drop: a group that is already on disk keeps its $ID and any key
// mxcli does not model, so MODIFY rewrites a description rather than replacing the
// element. A fresh $ID here would make Studio Pro paint the group as new.
func TestWorkflowGroups_PreservesStoredIdentityAndUnknownKeys(t *testing.T) {
	raw := map[string]any{
		"$Type": "Settings$WorkflowsProjectSettingsPart",
		"Groups": bson.A{int32(2), map[string]any{
			"$ID":           "stored-id-sentinel",
			"$Type":         "Settings$WorkflowGroup",
			"Name":          "Approvers",
			"Description":   "old",
			"SomeFutureKey": "keep me",
		}},
	}
	ws := &model.WorkflowsSettings{
		Groups: []model.WorkflowGroup{{Name: "Approvers", Description: "new"}},
	}

	groups := WorkflowGroups(ws, raw)["Groups"].(bson.A)
	g := groups[1].(map[string]any)
	if g["$ID"] != "stored-id-sentinel" {
		t.Errorf("$ID = %v, want the stored one preserved", g["$ID"])
	}
	if g["Description"] != "new" {
		t.Errorf("Description = %v, want the modelled value", g["Description"])
	}
	if g["SomeFutureKey"] != "keep me" {
		t.Errorf("a key mxcli does not model was dropped: %#v", g)
	}
}

// TestWorkflowGroups_MatchesStoredGroupCaseInsensitively mirrors how the executor
// addresses a group. Two groups differing only in case would be one group as far
// as every statement is concerned, so the overlay must not treat them as two
// documents and mint a second $ID.
func TestWorkflowGroups_MatchesStoredGroupCaseInsensitively(t *testing.T) {
	raw := map[string]any{
		"Groups": bson.A{int32(2), map[string]any{
			"$ID": "stored-id-sentinel", "$Type": "Settings$WorkflowGroup", "Name": "Approvers",
		}},
	}
	ws := &model.WorkflowsSettings{Groups: []model.WorkflowGroup{{Name: "APPROVERS"}}}
	g := WorkflowGroups(ws, raw)["Groups"].(bson.A)[1].(map[string]any)
	if g["$ID"] != "stored-id-sentinel" {
		t.Errorf("$ID = %v, want the stored group matched case-insensitively", g["$ID"])
	}
}

// TestWorkflowGroups_RemovedGroupLeavesTheList is the REMOVE half: the list is
// rebuilt from the model, so a group the model no longer carries must not survive
// in the preserved document.
func TestWorkflowGroups_RemovedGroupLeavesTheList(t *testing.T) {
	raw := map[string]any{
		"Groups": bson.A{int32(2),
			map[string]any{"$Type": "Settings$WorkflowGroup", "Name": "Approvers"},
			map[string]any{"$Type": "Settings$WorkflowGroup", "Name": "Reviewers"},
		},
	}
	ws := &model.WorkflowsSettings{Groups: []model.WorkflowGroup{{Name: "Approvers"}}}
	groups := WorkflowGroups(ws, raw)["Groups"].(bson.A)
	if len(groups) != 2 {
		t.Fatalf("Groups = %#v, want marker + one surviving group", groups)
	}
	if groups[1].(map[string]any)["Name"] != "Approvers" {
		t.Errorf("wrong group survived: %#v", groups[1])
	}
}
