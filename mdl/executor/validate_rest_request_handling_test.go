// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/mdl/backend/mock"
	"github.com/mendixlabs/mxcli/model"
)

// storedRestCall wraps a RequestHandling document the way a stored microflow
// nests it: the action sits inside an activity inside the object collection.
func storedRestCall(requestHandling map[string]any) map[string]any {
	return map[string]any{
		"$Type": "Microflows$Microflow",
		"ObjectCollection": map[string]any{
			"Objects": []any{
				map[string]any{
					"$Type": "Microflows$ActionActivity",
					"Action": map[string]any{
						"$Type":           "Microflows$RestCallAction",
						"RequestHandling": requestHandling,
					},
				},
			},
		},
	}
}

// A form-data body cannot be written, and a rewrite would drop it in silence:
// DESCRIBE omits the clause entirely (measured on a Studio Pro microflow —
// ako/TestApp `Mappings.PostFormData` describes as a REST call with no body),
// and a REST call with no body builds clean, so the app posts nothing and every
// signal says fine. Refuse instead (guard-don't-drop, ADR-0005).
func TestCheckNoUnwritableRestBody_RefusesFormData(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return storedRestCall(map[string]any{
				"$Type": "Microflows$FormDataRequestHandling",
				"Parts": []any{},
			}), nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))

	err := checkNoUnwritableRestBody(ctx, "mf-1", "M.ACT_Post")
	if err == nil {
		t.Fatal("expected a refusal for a microflow whose REST call posts form data")
	}
	for _, want := range []string{"M.ACT_Post", "form data", "post nothing"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q:\n%s", want, err)
		}
	}
}

// The writable handlers must pass, or every REST call becomes unmaintainable.
// Binary is in this list deliberately: it was unwritable until it was wired up,
// and the guard has to stop refusing the moment a type becomes expressible.
func TestCheckNoUnwritableRestBody_AllowsWritableHandlers(t *testing.T) {
	for _, typeName := range []string{
		"Microflows$CustomRequestHandling",
		"Microflows$MappingRequestHandling",
		"Microflows$BinaryRequestHandling",
		"Microflows$SimpleRequestHandling",
	} {
		t.Run(typeName, func(t *testing.T) {
			mb := &mock.MockBackend{
				IsConnectedFunc: func() bool { return true },
				GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
					return storedRestCall(map[string]any{"$Type": typeName}), nil
				},
			}
			ctx, _ := newMockCtx(t, withBackend(mb))
			if err := checkNoUnwritableRestBody(ctx, "mf-1", "M.ACT_Post"); err != nil {
				t.Errorf("must be allowed through: %v", err)
			}
		})
	}
}

// A microflow with no REST call at all must not be touched by this guard.
func TestCheckNoUnwritableRestBody_AllowsMicroflowWithoutRestCall(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetRawUnitFunc: func(id model.ID) (map[string]any, error) {
			return map[string]any{"$Type": "Microflows$Microflow"}, nil
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	if err := checkNoUnwritableRestBody(ctx, "mf-1", "M.ACT_Plain"); err != nil {
		t.Errorf("must be allowed through: %v", err)
	}
}
