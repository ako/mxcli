// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/settingsoverlay"
	"github.com/mendixlabs/mxcli/model"
)

// UpdateProjectSettings rewrites the Settings$ProjectSettings unit using the
// raw-part overlay strategy (ADR-0005 guard-don't-drop): the Settings array is
// rebuilt from ps.RawParts (captured on read), and only the parsed-and-modified
// parts (Model / Configuration / Language / Workflows) have their fields overlaid
// onto the preserved raw part. Every other part (WebUI, Convention, Integration,
// Certificate, JarDeployment, Distribution, …) passes through byte-for-byte.
func (b *Backend) UpdateProjectSettings(ps *model.ProjectSettings) error {
	if ps == nil {
		return fmt.Errorf("UpdateProjectSettings: nil settings")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateProjectSettings: not connected for writing")
	}
	// Without the raw parts there is nothing to overlay onto, and writing the
	// document anyway would replace every settings part with an empty array —
	// the whole Project Settings dialog silently reset. Refuse instead.
	if len(ps.RawParts) == 0 {
		return fmt.Errorf("UpdateProjectSettings: no raw settings parts captured on read; " +
			"refusing to write a settings document that would drop every part")
	}

	settings := bson.A{int32(2)} // versioned array prefix
	for _, rawPart := range ps.RawParts {
		typeName, _ := rawPart["$Type"].(string)
		switch typeName {
		case "Settings$ModelSettings":
			if ps.Model != nil {
				settings = append(settings, overlayModelSettings(ps.Model, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$ConfigurationSettings":
			if ps.Configuration != nil {
				settings = append(settings, settingsoverlay.Configurations(ps.Configuration, rawPart))
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$LanguageSettings":
			if ps.Language != nil {
				rawPart["DefaultLanguageCode"] = ps.Language.DefaultLanguageCode
				settings = append(settings, rawPart)
			} else {
				settings = append(settings, rawPart)
			}
		case "Settings$WorkflowsProjectSettingsPart":
			if ps.Workflows != nil {
				rawPart["UserEntity"] = ps.Workflows.UserEntity
				rawPart["DefaultTaskParallelism"] = settingsoverlay.SafeInt64(ps.Workflows.DefaultTaskParallelism)
				rawPart["WorkflowEngineParallelism"] = settingsoverlay.SafeInt64(ps.Workflows.WorkflowEngineParallelism)
				settings = append(settings, rawPart)
			} else {
				settings = append(settings, rawPart)
			}
		default:
			// Preserve raw part as-is.
			settings = append(settings, rawPart)
		}
	}

	doc := bson.M{
		"$ID":      bsonutil.IDToBsonBinary(string(ps.ID)),
		"$Type":    "Settings$ProjectSettings",
		"Settings": settings,
	}
	// Mendix 11.12+ requires "$ID" first in every storage object; the raw-part
	// overlay above works with unordered maps, so order the whole tree on write.
	contents, err := bson.Marshal(bsonutil.OrderStorageValue(doc))
	if err != nil {
		return fmt.Errorf("UpdateProjectSettings: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(ps.ID), contents)
}

func overlayModelSettings(ms *model.ModelSettings, raw map[string]any) map[string]any {
	return settingsoverlay.SetModelSettings(ms, raw)
}
