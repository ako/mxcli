// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/mdl/settingsoverlay"
	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// safeInt64 converts an int to int64.
func safeInt64(v int) int64 {
	return int64(v)
}

// UpdateProjectSettings updates the project settings document.
// The project settings document always exists, so this only needs update, not create/delete.
func (w *Writer) UpdateProjectSettings(ps *model.ProjectSettings) error {
	contents, err := w.serializeProjectSettings(ps)
	if err != nil {
		return fmt.Errorf("failed to serialize project settings: %w", err)
	}

	return w.updateUnit(string(ps.ID), contents)
}

// serializeProjectSettings converts ProjectSettings to BSON bytes.
// It uses the RawParts for round-trip fidelity, updating only the parts
// that have been parsed and modified.
func (w *Writer) serializeProjectSettings(ps *model.ProjectSettings) ([]byte, error) {
	// Without the raw parts there is nothing to overlay onto, and writing the
	// document anyway would replace every settings part with an empty array — the
	// whole Project Settings dialog silently reset. Refuse instead.
	if len(ps.RawParts) == 0 {
		return nil, fmt.Errorf("no raw settings parts captured on read; " +
			"refusing to write a settings document that would drop every part")
	}

	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ps.ID))},
		{Key: "$Type", Value: "Settings$ProjectSettings"},
	}

	// Rebuild the Settings array from RawParts, overwriting modified parts
	settings := bson.A{int32(2)} // versioned array prefix

	for _, rawPart := range ps.RawParts {
		typeName, _ := rawPart["$Type"].(string)
		switch typeName {
		case "Settings$ModelSettings":
			if ps.Model != nil {
				settings = append(settings, serializeModelSettings(ps.Model, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$ConfigurationSettings":
			if ps.Configuration != nil {
				settings = append(settings, serializeConfigurationSettings(ps.Configuration, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$LanguageSettings":
			if ps.Language != nil {
				settings = append(settings, serializeLanguageSettings(ps.Language, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$WorkflowsProjectSettingsPart":
			if ps.Workflows != nil {
				settings = append(settings, serializeWorkflowsSettings(ps.Workflows, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		default:
			// Preserve raw part as-is (WebUI, Integration, Certificate, JarDeployment, Distribution, Convention)
			settings = append(settings, rawPart)
		}
	}

	doc = append(doc, bson.E{Key: "Settings", Value: settings})
	// The Settings array carries parsed parts as Go maps (RawParts); marshalling
	// a map randomizes key order, so hoist "$ID"/"$Type" first per 11.12 (#nightly).
	return marshalUnitIDFirst(doc)
}

// serializeModelSettings overlays the modified model settings onto the raw BSON
// part. The overlay is shared with the codec engine so the two write paths cannot
// drift (see mdl/settingsoverlay), and is presence-gated so a write never
// introduces a property this Mendix version does not store.
func serializeModelSettings(ms *model.ModelSettings, raw map[string]any) map[string]any {
	return settingsoverlay.SetModelSettings(ms, raw)
}

// serializeConfigurationSettings overlays the modified configuration settings onto
// the raw BSON part. The overlay is shared with the codec engine so the two write
// paths cannot drift (see mdl/settingsoverlay and mendixlabs/mxcli#801).
func serializeConfigurationSettings(cs *model.ConfigurationSettings, raw map[string]any) map[string]any {
	return settingsoverlay.Configurations(cs, raw)
}

// serializeLanguageSettings updates the raw BSON map with modified language settings.
func serializeLanguageSettings(ls *model.LanguageSettings, raw map[string]any) map[string]any {
	raw["DefaultLanguageCode"] = ls.DefaultLanguageCode
	return raw
}

// serializeWorkflowsSettings updates the raw BSON map with modified workflow settings.
func serializeWorkflowsSettings(ws *model.WorkflowsSettings, raw map[string]any) map[string]any {
	raw["UserEntity"] = ws.UserEntity
	raw["DefaultTaskParallelism"] = safeInt64(ws.DefaultTaskParallelism)
	raw["WorkflowEngineParallelism"] = safeInt64(ws.WorkflowEngineParallelism)
	return raw
}
