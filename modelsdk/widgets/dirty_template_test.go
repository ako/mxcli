// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEmbeddedTemplates_NoDirtyBindings guards against the CE0463-class regression
// where an embedded widget template was extracted from a *configured* Studio Pro
// instance rather than a freshly-dropped one. The ComboBox template (fixed in
// 827bffd4b) had carried a System.UserRole association datasource as its "default";
// the page builder then applied the new widget's properties on top without resetting
// it, producing an Object inconsistent with the schema → CE0463. The Image template
// (fixed in 549c44f) carried a baked-in static image (Atlas_Core.Content.Mendix) for
// the same reason.
//
// A neutral template's Object must reference NO concrete entity, attribute,
// microflow/nanoflow, image asset, page, or configured client action. These are
// reliable dirty signals: deterministic and false-positive-free (a genuinely clean
// template has none of them — an unconfigured widget's action is Forms$NoAction and
// its image/page/binding slots are empty).
//
// Note: this does NOT guard non-default *primitive* drift (e.g. ComboBox's
// optionsSourceType="association"). That is not a reliable dirty signal — a fresh
// Studio Pro widget legitimately instantiates values that differ from the schema's
// declared DefaultValue (see reference_ce0463_tolerance_spike.md / the proposal's
// Phase-1 result), so guarding on it would flag correct templates. The cross-version
// mx-check matrix (proposed separately) is what catches primitive-level problems.
//
// The test is self-discovering (scans every template) and covers BOTH engines'
// template sets, so a regression in either is caught.
func TestEmbeddedTemplates_NoDirtyBindings(t *testing.T) {
	// Both engines' template dirs, relative to this package. The modelsdk set is
	// read via the embed FS (the exact bytes the binary ships); the legacy sdk set
	// is read from disk (kept byte-identical in tandem, but guarded independently).
	scanned := 0
	scan := func(label string, data []byte) {
		scanned++
		t.Run(label, func(t *testing.T) {
			var tmpl WidgetTemplate
			if err := json.Unmarshal(data, &tmpl); err != nil {
				t.Fatalf("parse %s: %v", label, err)
			}
			for _, b := range dirtyBindings(tmpl.Object) {
				t.Errorf("%s: template Object contains a concrete instance binding %q — "+
					"templates must be extracted from a FRESH (unconfigured) Studio Pro widget. "+
					"Re-extract clean or clear the offending default. See "+
					"PROPOSAL_multi_version_pluggable_widgets.md (Phase 1).", label, b)
			}
		})
	}

	// modelsdk engine — embedded bytes.
	const embDir = "templates/mendix-11.6"
	entries, err := templateFS.ReadDir(embDir)
	if err != nil {
		t.Fatalf("read embedded templates: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := templateFS.ReadFile(embDir + "/" + e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		scan("modelsdk/"+e.Name(), data)
	}

	// legacy sdk engine — sibling package's template files on disk (kept
	// byte-identical to the modelsdk set, but guarded independently).
	const sdkDir = "../../sdk/widgets/templates/mendix-11.6"
	sdkEntries, err := os.ReadDir(sdkDir)
	if err != nil {
		t.Fatalf("read sdk templates (%s): %v", sdkDir, err)
	}
	for _, e := range sdkEntries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(sdkDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		scan("sdk/"+e.Name(), data)
	}

	if scanned == 0 {
		t.Fatal("no templates found to audit")
	}
}

// TestDirtyBindings_DetectsDirt proves the guard is not vacuous across every dirty
// class it covers: an entity-ref datasource (the original ComboBox bug), an image
// asset (the Image bug, 549c44f), a page reference, and a configured client action.
func TestDirtyBindings_DetectsDirt(t *testing.T) {
	cases := []struct {
		name  string
		value map[string]any
		want  string
	}{
		{
			name: "entity-ref datasource",
			value: map[string]any{
				"$Type": "CustomWidgets$WidgetValue",
				"DataSource": map[string]any{
					"$Type": "CustomWidgets$CustomWidgetXPathSource",
					"EntityRef": map[string]any{
						"$Type":  "DomainModels$DirectEntityRef",
						"Entity": "System.UserRole",
					},
				},
			},
			want: "DirectEntityRef→System.UserRole",
		},
		{
			name:  "image asset",
			value: map[string]any{"$Type": "CustomWidgets$WidgetValue", "Image": "Atlas_Core.Content.Mendix"},
			want:  "Image→Atlas_Core.Content.Mendix",
		},
		{
			name:  "page reference",
			value: map[string]any{"$Type": "CustomWidgets$WidgetValue", "Form": "MyModule.SomePage"},
			want:  "Form→MyModule.SomePage",
		},
		{
			name: "configured client action",
			value: map[string]any{
				"$Type":  "CustomWidgets$WidgetValue",
				"Action": map[string]any{"$Type": "Forms$DeleteClientAction"},
			},
			want: "Action→Forms$DeleteClientAction",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			obj := map[string]any{
				"$Type":      "CustomWidgets$WidgetObject",
				"Properties": []any{float64(2), map[string]any{"$Type": "CustomWidgets$WidgetProperty", "Value": tc.value}},
			}
			got := dirtyBindings(obj)
			if len(got) == 0 {
				t.Fatalf("dirtyBindings failed to detect %s", tc.name)
			}
			found := false
			for _, g := range got {
				if g == tc.want {
					found = true
				}
			}
			if !found {
				t.Errorf("dirtyBindings = %v, want to contain %q", got, tc.want)
			}
		})
	}
}

// TestDirtyBindings_CleanIsClean proves a neutral WidgetValue (empty slots,
// Forms$NoAction) produces no findings — guarding against false positives.
func TestDirtyBindings_CleanIsClean(t *testing.T) {
	clean := map[string]any{
		"$Type": "CustomWidgets$WidgetObject",
		"Properties": []any{float64(2), map[string]any{
			"$Type": "CustomWidgets$WidgetProperty",
			"Value": map[string]any{
				"$Type":      "CustomWidgets$WidgetValue",
				"AttributeRef": nil,
				"DataSource": map[string]any{ // empty source slot — legitimate on data widgets
					"$Type":     "CustomWidgets$CustomWidgetXPathSource",
					"EntityRef": nil, "XPathConstraint": "",
				},
				"EntityRef": nil, "Image": "", "Form": "", "Microflow": "", "Nanoflow": "",
				"Action":         map[string]any{"$Type": "Forms$NoAction"},
				"PrimitiveValue": "image", "Selection": "None", // legitimate non-empty defaults
			},
		}},
	}
	if got := dirtyBindings(clean); len(got) != 0 {
		t.Errorf("neutral Object flagged as dirty: %v", got)
	}
}

// dirtyBindings returns every concrete entity/attribute/flow/image/page/action
// reference found anywhere in a widget Object. A neutral (freshly-extracted)
// template has none.
func dirtyBindings(obj map[string]any) []string {
	var out []string
	var walk func(any)
	walk = func(o any) {
		switch v := o.(type) {
		case map[string]any:
			switch v["$Type"] {
			case "DomainModels$DirectEntityRef":
				if e, _ := v["Entity"].(string); e != "" {
					out = append(out, "DirectEntityRef→"+e)
				}
			case "DomainModels$EntityRefStep":
				if a, _ := v["Association"].(string); a != "" {
					out = append(out, "EntityRefStep.Association→"+a)
				}
				if d, _ := v["DestinationEntity"].(string); d != "" {
					out = append(out, "EntityRefStep.DestinationEntity→"+d)
				}
			case "DomainModels$AttributeRef":
				if a, _ := v["Attribute"].(string); a != "" {
					out = append(out, "AttributeRef→"+a)
				}
			case "CustomWidgets$WidgetValue":
				// A concrete image-collection asset (the Image dirty-default class,
				// 549c44f). A fresh widget's image slot is empty.
				if img, _ := v["Image"].(string); img != "" {
					out = append(out, "Image→"+img)
				}
				// A concrete page reference — a fresh widget targets no page.
				if form, _ := v["Form"].(string); form != "" {
					out = append(out, "Form→"+form)
				}
				// A configured client action — a fresh widget's action is Forms$NoAction.
				if act, ok := v["Action"].(map[string]any); ok {
					if at, _ := act["$Type"].(string); at != "" && at != "Forms$NoAction" {
						out = append(out, "Action→"+at)
					}
				}
			}
			for k, val := range v {
				if (k == "Microflow" || k == "Nanoflow") && val != nil {
					if s, _ := val.(string); s != "" {
						out = append(out, k+"→"+s)
					}
				}
				walk(val)
			}
		case []any:
			for _, item := range v {
				walk(item)
			}
		}
	}
	walk(obj)
	return out
}
