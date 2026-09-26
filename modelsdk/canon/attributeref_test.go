// SPDX-License-Identifier: Apache-2.0

package canon

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Moved from mdl/backend/modelsdk (page_bare_attributeref_test.go) when the
// guard moved to the write choke point. The shape is Feedback v4.0.2's
// ShareFeedback_Logo: an image URL parameter written as a bare `ImageB64`
// under a data view whose flow the project lacks, which left `mx check` unable
// to LOAD the project (ArgumentNullException setting 'Attribute', 11.13.0).
func TestBareAttributeRefError(t *testing.T) {
	attrRef := func(a string) bson.D {
		return bson.D{{Key: "$Type", Value: "DomainModels$AttributeRef"}, {Key: "Attribute", Value: a}, {Key: "EntityRef", Value: nil}}
	}
	doc := func(a string) []byte {
		b, err := bson.Marshal(bson.D{
			{Key: "$Type", Value: "Forms$Page"},
			{Key: "Widgets", Value: bson.A{int32(2), bson.D{
				{Key: "$Type", Value: "CustomWidgets$CustomWidget"},
				{Key: "Name", Value: "image1"},
				{Key: "Params", Value: bson.A{int32(2), bson.D{{Key: "AttributeRef", Value: attrRef(a)}}}},
			}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, bare := range []string{"ImageB64", "Feedback.ImageB64"} {
		err := BareAttributeRefError("page X", doc(bare))
		if err == nil || !strings.Contains(err.Error(), `"`+bare+`"`) || !strings.Contains(err.Error(), "image1") {
			t.Fatalf("a bare attribute reference must be refused, naming it and its widget; got %v", err)
		}
	}
	for _, ok := range []string{"FeedbackModule.Feedback.ImageB64", ""} {
		if err := BareAttributeRefError("page X", doc(ok)); err != nil {
			t.Errorf("%q must be accepted: %v", ok, err)
		}
	}
	if got := BareAttributeRefs([]byte{1, 2, 3}); got != nil {
		t.Errorf("unreadable bytes must yield nothing, got %v", got)
	}
}
