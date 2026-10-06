// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// studioProSendEmail is the Send Email action Studio Pro 11.15.0-rc.4 saved in
// ako/TestApp (Email.EmailMF), transcribed key for key. $IDs are strings here;
// the shape comparison ignores their values.
func studioProSendEmail() bson.D {
	tpl := func(id, text string) bson.D {
		return bson.D{
			{Key: "$ID", Value: id},
			{Key: "$Type", Value: "Microflows$StringTemplate"},
			{Key: "Parameters", Value: bson.A{int32(2)}},
			{Key: "Text", Value: text},
		}
	}
	return bson.D{
		{Key: "$ID", Value: "act"},
		{Key: "$Type", Value: "Microflows$SendEmailAction"},
		{Key: "EmailAuthenticationConfig", Value: bson.D{
			{Key: "$ID", Value: "auth"},
			{Key: "$Type", Value: "Microflows$BasicAuthConfig"},
			{Key: "OptForAuthDocument", Value: false},
			{Key: "Password", Value: "'def'"},
			{Key: "Username", Value: "'abc'"},
		}},
		{Key: "EmailConnectionConfig", Value: bson.D{
			{Key: "$ID", Value: "conn"},
			{Key: "$Type", Value: "Microflows$EmailConnectionConfig"},
			{Key: "CheckServerIdentity", Value: false},
			{Key: "ConnectionTimeout", Value: int64(20000)},
			{Key: "EmailId", Value: "'uiser@exxxample.com'"},
			{Key: "Host", Value: "'smtp.example.com'"},
			{Key: "Port", Value: "25"},
			{Key: "Protocol", Value: "SMTP"},
			{Key: "SecurityType", Value: "TLS"},
		}},
		{Key: "EmailMessage", Value: bson.D{
			{Key: "$ID", Value: "msg"},
			{Key: "$Type", Value: "Microflows$EmailMessage"},
			{Key: "Attachment", Value: ""},
			{Key: "Bcc", Value: "'wqedcqw@awercvwaeef.com'"},
			{Key: "Cc", Value: "'wefdwe@wervwe.com'"},
			{Key: "CustomHeaders", Value: bson.A{int32(3), bson.D{
				{Key: "$ID", Value: "hdr"},
				{Key: "$Type", Value: "Microflows$EmailCustomHeader"},
				{Key: "Name", Value: "qweq"},
				{Key: "Value", Value: "qweq"},
			}}},
			{Key: "MessageBodyHtmlTemplate", Value: tpl("html", "'<body><h1>Hello</h1></body>'")},
			{Key: "MessageBodyPlainTextTemplate", Value: tpl("text", "'qwec qwec qwec qwec ew'")},
			{Key: "SubjectTemplate", Value: tpl("subj", "'qwec qwec qwe'")},
			{Key: "To", Value: "'asdfv@asdvas.nl'"},
		}},
		{Key: "ErrorHandlingType", Value: "Rollback"},
		{Key: "TestEmailMessage", Value: bson.D{
			{Key: "$ID", Value: "test"},
			{Key: "$Type", Value: "Microflows$EmailMessage"},
			{Key: "Attachment", Value: ""},
			{Key: "Bcc", Value: ""},
			{Key: "Cc", Value: ""},
			{Key: "CustomHeaders", Value: bson.A{int32(3)}},
			{Key: "MessageBodyHtmlTemplate", Value: tpl("t1", "")},
			{Key: "MessageBodyPlainTextTemplate", Value: tpl("t2", "")},
			{Key: "SubjectTemplate", Value: tpl("t3", "")},
			{Key: "To", Value: ""},
		}},
		{Key: "UseTemplateForTest", Value: false},
	}
}

// mendixlabs/mxcli#1315: the Send Email activity had no reader case, so
// describe printed `-- Unsupported action: Microflows$SendEmailAction` and a
// describe → exec round trip deleted the activity.
func TestActionFromGen_SendEmail(t *testing.T) {
	act := decodeAction(t, studioProSendEmail())
	got, ok := act.(*microflows.SendEmailAction)
	if !ok {
		t.Fatalf("actionFromGen → %T, want *microflows.SendEmailAction "+
			"(nil renders \"-- Unsupported action: Microflows$SendEmailAction\")", act)
	}
	want := &microflows.SendEmailAction{
		ErrorHandlingType:   microflows.ErrorHandlingTypeRollback,
		From:                "'uiser@exxxample.com'",
		Host:                "'smtp.example.com'",
		Port:                "25",
		SecurityType:        microflows.EmailSecurityTLS,
		ConnectionTimeout:   20000,
		UseAuthentication:   true,
		Username:            "'abc'",
		Password:            "'def'",
		To:                  "'asdfv@asdvas.nl'",
		Cc:                  "'wefdwe@wervwe.com'",
		Bcc:                 "'wqedcqw@awercvwaeef.com'",
		Subject:             microflows.EmailTemplate{Text: "'qwec qwec qwe'"},
		BodyPlainText:       microflows.EmailTemplate{Text: "'qwec qwec qwec qwec ew'"},
		BodyHTML:            microflows.EmailTemplate{Text: "'<body><h1>Hello</h1></body>'"},
		CustomHeaders:       []microflows.EmailCustomHeader{{Name: "qweq", Value: "qweq"}},
		CheckServerIdentity: false,
	}
	want.ID = got.ID
	if g, w := fmt.Sprintf("%+v", got), fmt.Sprintf("%+v", want); g != w {
		t.Errorf("read mismatch\n got  %s\n want %s", g, w)
	}
}

// Shapes MDL cannot restate stay UnsupportedAction (actionFromGen nil): reading
// them as a SendEmailAction would let a describe → exec round trip replace
// what is stored with something smaller.
func TestActionFromGen_SendEmail_UnrestatableShapesStayUnsupported(t *testing.T) {
	set := func(doc bson.D, path []string, v any) bson.D {
		var walk func(d bson.D, p []string) bson.D
		walk = func(d bson.D, p []string) bson.D {
			out := append(bson.D{}, d...)
			for i, e := range out {
				if e.Key != p[0] {
					continue
				}
				if len(p) == 1 {
					out[i].Value = v
				} else {
					out[i].Value = walk(e.Value.(bson.D), p[1:])
				}
				return out
			}
			if len(p) == 1 {
				return append(out, bson.E{Key: p[0], Value: v})
			}
			return out
		}
		return walk(doc, path)
	}
	drop := func(doc bson.D, parent, key string) bson.D {
		out := append(bson.D{}, doc...)
		for i, e := range out {
			if e.Key == parent {
				inner := bson.D{}
				for _, ie := range e.Value.(bson.D) {
					if ie.Key != key {
						inner = append(inner, ie)
					}
				}
				out[i].Value = inner
			}
		}
		return out
	}

	cases := map[string]bson.D{
		"authentication document": set(studioProSendEmail(), []string{"EmailAuthenticationConfig"}, bson.D{
			{Key: "$ID", Value: "auth"},
			{Key: "$Type", Value: "Microflows$AuthenticationDocumentConfig"},
			{Key: "AuthenticationDocument", Value: "MyModule.SmtpAuth"},
			{Key: "OptForAuthDocument", Value: true},
		}),
		"basic auth opting for a document": set(studioProSendEmail(),
			[]string{"EmailAuthenticationConfig", "OptForAuthDocument"}, true),
		"pre-11.13 expression subject": set(studioProSendEmail(),
			[]string{"EmailMessage", "Subject"}, "'Hello'"),
		"no subject template": drop(studioProSendEmail(), "EmailMessage", "SubjectTemplate"),
		"pre-11.3 attachments list": set(studioProSendEmail(),
			[]string{"EmailMessage", "Attachments"}, bson.A{int32(1), "MyModule.Doc"}),
		"non-SMTP protocol": set(studioProSendEmail(),
			[]string{"EmailConnectionConfig", "Protocol"}, "Unknown"),
	}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			if act := decodeAction(t, doc); act != nil {
				t.Fatalf("actionFromGen → %T, want nil (UnsupportedAction)", act)
			}
		})
	}
}

// The writer must emit the document Studio Pro saves, key for key, BSON type
// for BSON type, array marker for array marker. Comparing the shape rather than
// a few fields is what catches a constant the writer forgot (the
// rewrite-drops-unauthored-state audit), an int32 where Studio Pro stores an
// int64, or a list written with the wrong marker.
func TestSendEmailActionToGen_MatchesStudioProShape(t *testing.T) {
	act := decodeAction(t, studioProSendEmail()).(*microflows.SendEmailAction)
	out, err := (&codec.Encoder{}).Encode(sendEmailActionToGen(act))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := bsonShape(bson.Raw(out), "")
	want := bsonShape(mustMarshalFlow(studioProSendEmail()), "")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("written shape differs from Studio Pro's\n got:\n  %s\n want:\n  %s",
			strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

// Without an `auth basic` clause the slot is written as an explicit null, and
// reads back as no authentication.
func TestSendEmailActionToGen_NoAuthIsNull(t *testing.T) {
	act := &microflows.SendEmailAction{From: "'a@x'", To: "'b@x'", Host: "'h'", Port: "25",
		Subject: microflows.EmailTemplate{Text: "Hi"}}
	out, err := (&codec.Encoder{}).Encode(sendEmailActionToGen(act))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	v, err := bson.Raw(out).LookupErr("EmailAuthenticationConfig")
	if err != nil || v.Type != bson.TypeNull {
		t.Fatalf("EmailAuthenticationConfig = %v (err %v), want null", v, err)
	}
	back := sendEmailActionFromRaw(bson.Raw(out))
	if back == nil || back.UseAuthentication {
		t.Fatalf("read back %+v, want a SendEmailAction without authentication", back)
	}
}

// Every field MDL can author survives write → read, including the ones the
// Studio Pro fixture leaves at their defaults.
func TestMicroflowRoundTrip_SendEmail(t *testing.T) {
	act := &microflows.SendEmailAction{
		ErrorHandlingType:   microflows.ErrorHandlingTypeCustomWithoutRollback,
		From:                "$Msg/Sender",
		Host:                "@Mail.SmtpHost",
		Port:                "@Mail.SmtpPort",
		SecurityType:        microflows.EmailSecuritySSL,
		CheckServerIdentity: true,
		ConnectionTimeout:   30000,
		UseAuthentication:   true,
		Username:            "@Mail.SmtpUser",
		Password:            "@Mail.SmtpPassword",
		To:                  "$Msg/Recipient",
		Cc:                  "'cc@example.com'",
		Bcc:                 "'bcc@example.com'",
		Subject:             microflows.EmailTemplate{Text: "Order {1} for {2}", Parameters: []string{"$Order/Number", "$Msg/Name"}},
		BodyPlainText:       microflows.EmailTemplate{Text: "Hello {1}", Parameters: []string{"$Msg/Name"}},
		BodyHTML:            microflows.EmailTemplate{Text: "<p>Hello</p>"},
		CustomHeaders: []microflows.EmailCustomHeader{
			{Name: "X-Correlation-Id", Value: "abc-123"},
			{Name: "X-Priority", Value: "1"},
		},
		Attachment: "Invoice",
	}
	act.ID = model.ID("mail-1")
	activity := &microflows.ActionActivity{Action: act}
	activity.ID = model.ID("act-1")
	mf := &microflows.Microflow{
		Name: "SUB_Send",
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{activity},
		},
	}
	mf.ID = model.ID("mf-1")

	got := roundTripMicroflow(t, mf)
	var found *microflows.SendEmailAction
	for _, obj := range got.ObjectCollection.Objects {
		if aa, ok := obj.(*microflows.ActionActivity); ok {
			if s, ok := aa.Action.(*microflows.SendEmailAction); ok {
				found = s
			}
		}
	}
	if found == nil {
		t.Fatal("no SendEmailAction survived the round trip")
	}
	found.ID = act.ID
	if g, w := fmt.Sprintf("%+v", found), fmt.Sprintf("%+v", act); g != w {
		t.Errorf("round trip changed the action\n got  %s\n want %s", g, w)
	}
}

// bsonShape flattens a document to sorted "path: bsontype" lines, with array
// markers spelled out and $ID values ignored.
func bsonShape(doc bson.Raw, prefix string) []string {
	var out []string
	els, _ := doc.Elements()
	for _, e := range els {
		path := prefix + e.Key()
		v := e.Value()
		switch v.Type {
		case bson.TypeEmbeddedDocument:
			out = append(out, path+": document")
			out = append(out, bsonShape(v.Document(), path+".")...)
		case bson.TypeArray:
			vals, _ := v.Array().Values()
			out = append(out, fmt.Sprintf("%s: array(%d items)", path, len(vals)))
			for i, item := range vals {
				ip := fmt.Sprintf("%s[%d]", path, i)
				if i == 0 {
					out = append(out, fmt.Sprintf("%s: marker %v", ip, item))
					continue
				}
				if item.Type == bson.TypeEmbeddedDocument {
					out = append(out, bsonShape(item.Document(), ip+".")...)
				} else {
					out = append(out, ip+": "+item.Type.String())
				}
			}
		default:
			if e.Key() == "$ID" {
				out = append(out, path+": id")
				continue
			}
			out = append(out, path+": "+v.Type.String())
		}
	}
	sort.Strings(out)
	return out
}
