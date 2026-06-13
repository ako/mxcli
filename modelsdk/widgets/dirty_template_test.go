// SPDX-License-Identifier: Apache-2.0

package widgets

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestEmbeddedTemplates_NoDirtyBindings guards against the CE0463-class regression
// where an embedded widget template was extracted from a *configured* Studio Pro
// instance rather than a freshly-dropped one. The ComboBox template (fixed in
// 827bffd4b) had carried a System.UserRole association datasource as its "default";
// the page builder then applied the new widget's properties on top without resetting
// it, producing an Object inconsistent with the schema → CE0463.
//
// A neutral template's Object must reference NO concrete entity, attribute, or
// microflow/nanoflow. This is the reliable dirty signal: it is deterministic and
// false-positive-free (a genuinely clean template has zero concrete references).
//
// Note: this does NOT guard non-default *primitive* drift (e.g. ComboBox's
// optionsSourceType="association"). That is not a reliable dirty signal — a fresh
// Studio Pro widget legitimately instantiates values that differ from the schema's
// declared DefaultValue (see reference_ce0463_tolerance_spike.md / the proposal's
// Phase-1 result), so guarding on it would flag correct templates. The cross-version
// mx-check matrix (proposed separately) is what catches primitive-level problems.
//
// The test is self-discovering: it scans every embedded template, so new templates
// are covered automatically.
func TestEmbeddedTemplates_NoDirtyBindings(t *testing.T) {
	const dir = "templates/mendix-11.6"
	entries, err := templateFS.ReadDir(dir)
	if err != nil {
		t.Fatalf("read embedded templates: %v", err)
	}

	scanned := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		scanned++
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			data, err := templateFS.ReadFile(dir + "/" + name)
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			var tmpl WidgetTemplate
			if err := json.Unmarshal(data, &tmpl); err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}
			for _, b := range dirtyBindings(tmpl.Object) {
				t.Errorf("%s: template Object contains a concrete instance binding %q — "+
					"templates must be extracted from a FRESH (unconfigured) Studio Pro widget", name, b)
			}
		})
	}

	if scanned == 0 {
		t.Fatal("no embedded templates found to audit")
	}
}

// TestDirtyBindings_DetectsDirt proves the guard is not vacuous: a synthetic Object
// shaped like the original dirty ComboBox (a System.UserRole association datasource)
// must be flagged.
func TestDirtyBindings_DetectsDirt(t *testing.T) {
	dirty := map[string]any{
		"$Type": "CustomWidgets$WidgetObject",
		"Properties": []any{float64(2),
			map[string]any{
				"$Type": "CustomWidgets$WidgetProperty",
				"Value": map[string]any{
					"$Type": "CustomWidgets$WidgetValue",
					"DataSource": map[string]any{
						"$Type": "CustomWidgets$CustomWidgetXPathSource",
						"EntityRef": map[string]any{
							"$Type":  "DomainModels$DirectEntityRef",
							"Entity": "System.UserRole",
						},
					},
				},
			},
		},
	}
	got := dirtyBindings(dirty)
	if len(got) == 0 {
		t.Fatal("dirtyBindings failed to detect a System.UserRole binding")
	}
	if got[0] != "DirectEntityRef→System.UserRole" {
		t.Errorf("dirtyBindings = %v, want [DirectEntityRef→System.UserRole]", got)
	}
}

// dirtyBindings returns every concrete entity/attribute/flow reference found
// anywhere in a widget Object. A neutral (freshly-extracted) template has none.
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
