// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"testing"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// mendixlabs/mxcli#1078, the legacy engine's half.
//
// Fixing the describer made the default (modelsdk) engine round-trip an error
// handler again, and MXCLI_ENGINE=legacy still dropped it — for a second,
// independent reason: nine parse functions never read ErrorHandlingType off the
// BSON at all, so the value was gone before the describer could be asked about
// it. Measured on the same project: 0 errors on modelsdk, handler still missing
// on legacy, until these were fixed too.
//
// The two defects are stacked, which is why fixing one looked like fixing both.
func TestParse1078_ActionsReadErrorHandlingType(t *testing.T) {
	// "Custom" is what Studio Pro's "custom with rollback" stores, and it is the
	// value the reporter's create-variable activity carried.
	const custom = "Custom"

	for _, tc := range []struct {
		name string
		got  func() microflows.ErrorHandlingType
	}{
		{"create variable", func() microflows.ErrorHandlingType {
			return parseCreateVariableAction(map[string]any{
				"$ID": "a", "VariableName": "name", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"change variable", func() microflows.ErrorHandlingType {
			return parseChangeVariableAction(map[string]any{
				"$ID": "a", "ChangeVariableName": "name", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"create object", func() microflows.ErrorHandlingType {
			return parseCreateObjectAction(map[string]any{
				"$ID": "a", "Entity": "Mod.Car", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"change object", func() microflows.ErrorHandlingType {
			return parseChangeObjectAction(map[string]any{
				"$ID": "a", "ChangeVariableName": "Car", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"close page", func() microflows.ErrorHandlingType {
			return parseClosePageAction(map[string]any{
				"$ID": "a", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"log message", func() microflows.ErrorHandlingType {
			return parseLogMessageAction(map[string]any{
				"$ID": "a", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"show message", func() microflows.ErrorHandlingType {
			return parseShowMessageAction(map[string]any{
				"$ID": "a", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"show page", func() microflows.ErrorHandlingType {
			return parseShowPageAction(map[string]any{
				"$ID": "a", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
		{"validation feedback", func() microflows.ErrorHandlingType {
			return parseValidationFeedbackAction(map[string]any{
				"$ID": "a", "ErrorHandlingType": custom,
			}).ErrorHandlingType
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got(); got != microflows.ErrorHandlingTypeCustom {
				t.Errorf("ErrorHandlingType = %q, want %q — the whole error branch is "+
					"dropped from DESCRIBE when this is empty",
					got, microflows.ErrorHandlingTypeCustom)
			}
		})
	}
}

// Control. An absent ErrorHandlingType must stay empty rather than being invented:
// #840 established that a rendered `on error rollback` puts a clause in the
// user's script they never wrote, and these parsers are read by the same
// describer.
func TestParse1078_AbsentErrorHandlingTypeStaysEmpty(t *testing.T) {
	if got := parseCreateVariableAction(map[string]any{
		"$ID": "a", "VariableName": "name",
	}).ErrorHandlingType; got != "" {
		t.Errorf("ErrorHandlingType = %q, want empty for BSON that carries none", got)
	}
}
