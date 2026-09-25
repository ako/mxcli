// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// A DomainModels$AttributeRef whose Attribute is not Module.Entity.Attr makes
// the project unloadable: an image URL parameter written as a bare `ImageB64`
// (Feedback v4.0.2's ShareFeedback_Logo, under a data view whose flow the
// project lacks) left `mx check` unable to LOAD the project —
// ArgumentNullException setting 'Attribute', Mendix 11.13.0 — while the page
// was excluded. Studio Pro qualifies every one: 72 of 72 AttributeRefs across
// that project's pages, snippets and layouts. So the writer refuses the bare
// form, naming it, instead of storing it.
func TestRefuseBareAttributeRefs(t *testing.T) {
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
	err := refuseBareAttributeRefs(doc("ImageB64"))
	if err == nil || !strings.Contains(err.Error(), "ImageB64") {
		t.Fatalf("a bare attribute reference must be refused, naming it; got %v", err)
	}
	for _, ok := range []string{"FeedbackModule.Feedback.ImageB64", ""} {
		if err := refuseBareAttributeRefs(doc(ok)); err != nil {
			t.Errorf("%q must be accepted: %v", ok, err)
		}
	}
}
