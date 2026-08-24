// SPDX-License-Identifier: Apache-2.0

// A conditional expression INSIDE a hidePropertiesIn array.
//
// The extractor already handles a ternary wrapped AROUND a hide call. It did not
// handle one used as an array ELEMENT — and it did not skip it either: it
// harvested every string literal in the array, so
//
//	hidePropertiesIn(t, e, ["files" === e.uploadMode ? "associatedImages" : "associatedFiles", ...])
//
// produced three wrong rules — `files` (the comparison literal, not a property at
// all), plus BOTH branches marked hidden under the OUTER guard only, with the
// ternary's own condition lost.
//
// Measured on File Uploader 2.5.0: `associatedImages` was therefore not pruned
// when uploadMode is "files", and since visibility is applied at SERIALIZATION
// time (widget_engine.go → ApplyPropertyVisibility), authoring the widget's
// datasource wrote BOTH association properties and the build failed CE0463
// (upstream #956).
package executor

import "testing"

func TestExtractVisibility_TernaryInsideTheHideArray(t *testing.T) {
	// Shape taken verbatim from FileUploader 2.5.0's editorConfig.js.
	// Shape taken verbatim from FileUploader 2.5.0: an outer readOnlyMode guard,
	// with a ternary as the FIRST element of the hidden-property array.
	js := `exports.getProperties=function(e,t){return e.readOnlyMode?A.hidePropertiesIn(t,e,["files"===e.uploadMode?"associatedImages":"associatedFiles","createImageAction","maxFileSize"]):null}`

	rules, _ := extractVisibilityRulesFromJS(js)

	byKey := map[string][]string{} // propertyKey -> rendered conditions
	for _, r := range rules {
		cond := "<none>"
		if r.HiddenWhen != nil {
			cond = r.HiddenWhen.PropertyKey + " " + r.HiddenWhen.Operator + " " + r.HiddenWhen.Value
		}
		byKey[r.PropertyKey] = append(byKey[r.PropertyKey], cond)
	}

	// 1. The comparison literal is not a property.
	if _, ok := byKey["files"]; ok {
		t.Errorf("rule invented for %q — that is the ternary's comparison literal, not a widget property; rules=%v", "files", byKey)
	}

	// 2. Each branch keeps its own condition, and they are opposites.
	img, okImg := byKey["associatedImages"]
	fil, okFil := byKey["associatedFiles"]
	if !okImg || !okFil {
		t.Fatalf("both ternary branches must yield a rule; got %v", byKey)
	}
	if !containsCond(img, "uploadMode eq files") {
		t.Errorf("associatedImages should be hidden when uploadMode == files, got %v", img)
	}
	if !containsCond(fil, "uploadMode ne files") {
		t.Errorf("associatedFiles should be hidden when uploadMode != files, got %v", fil)
	}

	// 3. A plain element of the same array keeps the OUTER guard, unchanged.
	if !containsCond(byKey["maxFileSize"], "readOnlyMode truthy ") {
		t.Errorf("a non-conditional element of the same array lost its outer guard; got %v", byKey["maxFileSize"])
	}
}

func containsCond(got []string, want string) bool {
	for _, g := range got {
		if g == want {
			return true
		}
	}
	return false
}
