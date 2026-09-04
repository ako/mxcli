// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"encoding/json"
	"strings"
	"testing"
)

// serveFailureBody is the shape mxbuild 11.13's serve /build returns when the
// model does not deploy, trimmed to the fields that matter.
//
// Captured from a real response rather than written from the docs: nothing in
// this repo had ever parsed a failing build, and the nesting is easy to get
// wrong — `problems` is an OBJECT with its own `problems` list inside, and the
// outer `errors` list carries only the summary sentence, not the consistency
// errors. The ratio is real too: a blank app returns 16 warnings and a
// deprecation alongside the single error that stopped the build.
const serveFailureBody = `{
  "status": "Failure",
  "message": "The project cannot be deployed, because it contains errors.",
  "problems": {
    "errors": [{"message": "The project cannot be deployed, because it contains errors.", "details": ""}],
    "problems": [
      {
        "severity": "Warning",
        "message": "No 'On click' action specified.",
        "errorCode": "CW0055",
        "locations": [{"element": "Menu item", "document": "Menu 'Tablet_Menu'", "module": "Atlas_Core"}]
      },
      {
        "severity": "Deprecation",
        "message": "Something is deprecated.",
        "errorCode": "CD0001",
        "locations": []
      },
      {
        "severity": "Error",
        "message": "Undefined variable 'nosuchvar'.",
        "errorCode": "CE0109",
        "locations": [{"element": "End event", "document": "Microflow 'Test_test_3'", "module": "MxTest"}]
      }
    ]
  }
}`

func TestBuildResultErrors(t *testing.T) {
	var r BuildResult
	if err := json.Unmarshal([]byte(serveFailureBody), &r); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if r.OK() {
		t.Fatal("a Failure status must not read as OK")
	}

	// The filter is the whole point: 3 problems in, 1 error out.
	errs := r.Errors()
	if len(errs) != 1 {
		t.Fatalf("Errors() = %d, want 1 (warnings and deprecations must not be reported as errors)", len(errs))
	}
	if errs[0].ErrorCode != "CE0109" {
		t.Errorf("errorCode = %q, want CE0109", errs[0].ErrorCode)
	}
	if got := errs[0].Where(); got != "MxTest / Microflow 'Test_test_3' / End event" {
		t.Errorf("Where() = %q", got)
	}
}

func TestBuildResultErrorSummary(t *testing.T) {
	var r BuildResult
	if err := json.Unmarshal([]byte(serveFailureBody), &r); err != nil {
		t.Fatalf("decode: %v", err)
	}

	summary := r.ErrorSummary()
	for _, want := range []string{"CE0109", "Undefined variable 'nosuchvar'", "Microflow 'Test_test_3'"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary %q does not carry %q", summary, want)
		}
	}
	// The noise the summary exists to drop.
	for _, unwanted := range []string{"CW0055", "Tablet_Menu", "Deprecation"} {
		if strings.Contains(summary, unwanted) {
			t.Errorf("summary should not carry the warning %q", unwanted)
		}
	}
}

// TestBuildFailureDetailFallsBackToRaw guards the case that keeps this safe on a
// future mxbuild: if the response carries no structured errors, the caller must
// still see the body rather than an empty message.
func TestBuildFailureDetailFallsBackToRaw(t *testing.T) {
	r := &BuildResult{Status: "Failure", Message: "nope", Raw: json.RawMessage(`{"status":"Failure"}`)}
	if got := buildFailureDetail(r); !strings.Contains(got, `"status":"Failure"`) {
		t.Errorf("detail = %q, want the raw body as a fallback", got)
	}

	// And with structured errors it must prefer them.
	var parsed BuildResult
	if err := json.Unmarshal([]byte(serveFailureBody), &parsed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	parsed.Raw = json.RawMessage(serveFailureBody)
	got := buildFailureDetail(&parsed)
	if strings.Contains(got, "CW0055") {
		t.Errorf("detail should be the filtered summary, not the raw body: %q", got)
	}
	if !strings.Contains(got, "CE0109") {
		t.Errorf("detail %q lost the error", got)
	}
}

// TestBuildFailedErrorMessage covers what a user sees when a build fails.
func TestBuildFailedErrorMessage(t *testing.T) {
	var r BuildResult
	if err := json.Unmarshal([]byte(serveFailureBody), &r); err != nil {
		t.Fatalf("decode: %v", err)
	}
	err := &BuildFailedError{Result: &r}
	msg := err.Error()
	for _, want := range []string{"build failed", "cannot be deployed", "CE0109", "Test_test_3"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q does not carry %q", msg, want)
		}
	}
	if len(err.BuildErrors()) != 1 {
		t.Errorf("BuildErrors() = %d, want 1", len(err.BuildErrors()))
	}
}
