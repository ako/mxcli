// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"

	"go.mongodb.org/mongo-driver/bson"
)

// parseRule parses a Microflows$Rule document. A rule shares a microflow's
// object collection, flows, parameters and return type, so this mirrors
// parseNanoflow with two differences measured against Studio Pro-authored rules
// (ako/TestApp, Mendix 11.13.0):
//
//   - a rule stores no AllowedModuleRoles — it is not independently callable, so
//     it has no module-role security;
//   - gen declares a ReturnType string beside MicroflowReturnType, but Studio Pro
//     does not write it and generated/metamodel does not list it, so it is not
//     read and must not be written.
func (r *Reader) parseRule(unitID, containerID string, contents []byte) (*microflows.Rule, error) {
	contents, err := r.resolveContents(unitID, contents)
	if err != nil {
		return nil, err
	}

	var raw map[string]any
	if err := bson.Unmarshal(contents, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal BSON: %w", err)
	}

	rule := &microflows.Rule{}
	rule.ID = model.ID(unitID)
	rule.TypeName = "Microflows$Rule"
	rule.ContainerID = model.ID(containerID)

	if name, ok := raw["Name"].(string); ok {
		rule.Name = name
	}
	if doc, ok := raw["Documentation"].(string); ok {
		rule.Documentation = doc
	}
	if excluded, ok := raw["Excluded"].(bool); ok {
		rule.Excluded = excluded
	}
	if markAsUsed, ok := raw["MarkAsUsed"].(bool); ok {
		rule.MarkAsUsed = markAsUsed
	}
	if applyEntityAccess, ok := raw["ApplyEntityAccess"].(bool); ok {
		rule.ApplyEntityAccess = applyEntityAccess
	}
	if returnVariableName, ok := raw["ReturnVariableName"].(string); ok {
		rule.ReturnVariableName = returnVariableName
	}

	// Return type — Boolean or an enumeration, under the microflow's BSON key.
	if rt, ok := raw["MicroflowReturnType"].(map[string]any); ok {
		rule.ReturnType = parseMicroflowDataType(rt)
	}

	if oc := extractBsonMap(raw["ObjectCollection"]); oc != nil {
		rule.ObjectCollection = parseMicroflowObjectCollection(oc)
		for _, obj := range extractBsonSlice(oc["Objects"]) {
			if objMap := extractBsonMap(obj); objMap != nil {
				if typeName, _ := objMap["$Type"].(string); typeName == "Microflows$MicroflowParameter" {
					rule.Parameters = append(rule.Parameters, parseMicroflowParameter(objMap))
				}
			}
		}
	}

	if flowsRaw := raw["Flows"]; flowsRaw != nil {
		if rule.ObjectCollection == nil {
			rule.ObjectCollection = &microflows.MicroflowObjectCollection{}
		}
		for _, f := range extractBsonSlice(flowsRaw) {
			flowMap := extractBsonMap(f)
			if flowMap == nil {
				continue
			}
			typeName, _ := flowMap["$Type"].(string)
			switch typeName {
			case "Microflows$AnnotationFlow":
				if af := parseAnnotationFlow(flowMap); af != nil {
					rule.ObjectCollection.AnnotationFlows = append(rule.ObjectCollection.AnnotationFlows, af)
				}
			default:
				if flow := parseSequenceFlow(flowMap); flow != nil {
					rule.ObjectCollection.Flows = append(rule.ObjectCollection.Flows, flow)
				}
			}
		}
	}

	return rule, nil
}
