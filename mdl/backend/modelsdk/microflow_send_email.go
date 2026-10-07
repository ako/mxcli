// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// The Send Email activity (Microflows$SendEmailAction, Studio Pro 11.13+ beta).
//
// Both directions work on raw keys rather than the gen type: modelsdk/gen was
// generated before 11.12 and binds EmailMessage's Subject/MessageBody* as
// expressions, where 11.13 deleted those and stores SubjectTemplate,
// MessageBodyPlainTextTemplate and MessageBodyHtmlTemplate (Microflows$StringTemplate)
// plus a CustomHeaders list (mendixmodelsdk 4.116 StructureVersionInfo). The
// shape below is the one Studio Pro 11.15.0-rc.4 saved in ako/TestApp
// (Email.EmailMF), key for key.

func init() {
	// An EmailMessage always carries its CustomHeaders list, empty as marker 3
	// (measured: the TestEmailMessage of ako/TestApp Email.EmailMF).
	codec.RegisterTypeDefaults("Microflows$EmailMessage", codec.TypeDefaults{
		MandatoryLists: []string{"CustomHeaders"},
	})
}

// sendEmailActionFromRaw reads a stored SendEmailAction. It returns nil — so the
// activity stays an UnsupportedAction, describable but never rewritten — for any
// stored shape the semantic model cannot restate:
//
//   - authentication by an Authentication document (AuthenticationDocumentConfig,
//     or OptForAuthDocument set), which MDL has no spelling for;
//   - a pre-11.13 message, whose subject and bodies are expressions in Subject /
//     MessageBody* instead of the three templates, or which uses the
//     pre-11.3 Attachments list;
//   - a protocol other than SMTP.
//
// Writing any of these back from MDL would silently replace what is stored.
func sendEmailActionFromRaw(raw bson.Raw) *microflows.SendEmailAction {
	conn, ok := raw.Lookup("EmailConnectionConfig").DocumentOK()
	if !ok {
		return nil
	}
	msg, ok := raw.Lookup("EmailMessage").DocumentOK()
	if !ok {
		return nil
	}
	if p := rawStr(conn, "Protocol"); p != "" && p != "SMTP" {
		return nil
	}

	out := &microflows.SendEmailAction{
		ErrorHandlingType:   microflows.ErrorHandlingType(orDefault(rawStr(raw, "ErrorHandlingType"), string(microflows.ErrorHandlingTypeRollback))),
		From:                rawStr(conn, "EmailId"),
		Host:                rawStr(conn, "Host"),
		Port:                rawStr(conn, "Port"),
		SecurityType:        microflows.EmailSecurityType(orDefault(rawStr(conn, "SecurityType"), string(microflows.EmailSecurityTLS))),
		CheckServerIdentity: rawBool(conn, "CheckServerIdentity"),
		ConnectionTimeout:   microflows.DefaultEmailConnectionTimeout,
	}
	if n, ok := rawInteger(conn, "ConnectionTimeout"); ok {
		out.ConnectionTimeout = n
	}

	if v, err := raw.LookupErr("EmailAuthenticationConfig"); err == nil && v.Type != bson.TypeNull {
		auth, ok := v.DocumentOK()
		if !ok || rawStr(auth, "$Type") != "Microflows$BasicAuthConfig" || rawBool(auth, "OptForAuthDocument") {
			return nil
		}
		out.Username = rawStr(auth, "Username")
		out.Password = rawStr(auth, "Password")
		// A BasicAuthConfig holding nothing authenticates with nothing: it is the
		// same action as one without the config, and `auth basic  password` is
		// not MDL.
		out.UseAuthentication = out.Username != "" || out.Password != ""
	}

	// Pre-11.13 message shape: refuse rather than read the empty templates of a
	// message whose content lives in keys this reader does not map.
	for _, legacy := range []string{"Subject", "MessageBodyPlainText", "MessageBodyHtml"} {
		if rawStr(msg, legacy) != "" {
			return nil
		}
	}
	if len(rawDocElements(msg, "Attachments")) > 0 || rawStrings(msg, "Attachments") > 0 {
		return nil
	}
	subj, ok := emailTemplateFromRaw(msg, "SubjectTemplate")
	if !ok {
		return nil
	}
	out.Subject = subj
	out.BodyPlainText, _ = emailTemplateFromRaw(msg, "MessageBodyPlainTextTemplate")
	out.BodyHTML, _ = emailTemplateFromRaw(msg, "MessageBodyHtmlTemplate")
	out.To = rawStr(msg, "To")
	out.Cc = rawStr(msg, "Cc")
	out.Bcc = rawStr(msg, "Bcc")
	out.Attachment = rawStr(msg, "Attachment")
	for _, h := range rawDocElements(msg, "CustomHeaders") {
		out.CustomHeaders = append(out.CustomHeaders, microflows.EmailCustomHeader{
			Name:  rawStr(h, "Name"),
			Value: rawStr(h, "Value"),
		})
	}
	return out
}

// emailTemplateFromRaw reads one Microflows$StringTemplate child. ok is false
// when the key is absent or not a document.
func emailTemplateFromRaw(msg bson.Raw, key string) (microflows.EmailTemplate, bool) {
	doc, ok := msg.Lookup(key).DocumentOK()
	if !ok {
		return microflows.EmailTemplate{}, false
	}
	t := microflows.EmailTemplate{Text: rawStr(doc, "Text")}
	for _, p := range rawDocElements(doc, "Parameters") {
		t.Parameters = append(t.Parameters, rawStr(p, "Expression"))
	}
	return t, true
}

// sendEmailActionToGen builds the stored SendEmailAction. Keys are in the order
// Studio Pro saves them.
//
// TestEmailMessage is Studio Pro's Test Email tab — a draft used only to send a
// trial mail from the editor, never at runtime. MDL does not carry it (the
// issue scoped it out), so it is written empty, exactly as a fresh activity
// has it, and UseTemplateForTest false.
func sendEmailActionToGen(a *microflows.SendEmailAction) element.Element {
	g := newElem("Microflows$SendEmailAction", string(a.ID))

	if a.UseAuthentication {
		auth := newElem("Microflows$BasicAuthConfig", "")
		addBool(auth, "OptForAuthDocument", false)
		addStr(auth, "Password", a.Password)
		addStr(auth, "Username", a.Username)
		addPart(g, "EmailAuthenticationConfig", auth)
	} else {
		addNull(g, "EmailAuthenticationConfig")
	}

	conn := newElem("Microflows$EmailConnectionConfig", "")
	addBool(conn, "CheckServerIdentity", a.CheckServerIdentity)
	timeout := a.ConnectionTimeout
	if timeout == 0 {
		timeout = microflows.DefaultEmailConnectionTimeout
	}
	addInt64(conn, "ConnectionTimeout", int64(timeout))
	addStr(conn, "EmailId", a.From)
	addStr(conn, "Host", a.Host)
	addStr(conn, "Port", a.Port)
	addStr(conn, "Protocol", "SMTP")
	addStr(conn, "SecurityType", orDefault(string(a.SecurityType), string(microflows.EmailSecurityTLS)))
	addPart(g, "EmailConnectionConfig", conn)

	addPart(g, "EmailMessage", emailMessageElem(a))
	addStr(g, "ErrorHandlingType", orDefault(string(a.ErrorHandlingType), string(microflows.ErrorHandlingTypeRollback)))
	addPart(g, "TestEmailMessage", emailMessageElem(&microflows.SendEmailAction{}))
	addBool(g, "UseTemplateForTest", false)
	return g
}

func emailMessageElem(a *microflows.SendEmailAction) element.Element {
	m := newElem("Microflows$EmailMessage", "")
	addStr(m, "Attachment", a.Attachment)
	addStr(m, "Bcc", a.Bcc)
	addStr(m, "Cc", a.Cc)
	if len(a.CustomHeaders) > 0 {
		headers := make([]element.Element, 0, len(a.CustomHeaders))
		for _, h := range a.CustomHeaders {
			he := newElem("Microflows$EmailCustomHeader", "")
			addStr(he, "Name", h.Name)
			addStr(he, "Value", h.Value)
			headers = append(headers, he)
		}
		addPartList(m, "CustomHeaders", headers)
	}
	addPart(m, "MessageBodyHtmlTemplate", stringTemplateElem(a.BodyHTML.Text, a.BodyHTML.Parameters))
	addPart(m, "MessageBodyPlainTextTemplate", stringTemplateElem(a.BodyPlainText.Text, a.BodyPlainText.Parameters))
	addPart(m, "SubjectTemplate", stringTemplateElem(a.Subject.Text, a.Subject.Parameters))
	addStr(m, "To", a.To)
	return m
}

// rawBool reads a boolean field, false when absent.
func rawBool(doc bson.Raw, key string) bool {
	v, _ := doc.Lookup(key).BooleanOK()
	return v
}

// rawInteger reads an integer field stored as either int32 or int64.
func rawInteger(doc bson.Raw, key string) (int, bool) {
	v := doc.Lookup(key)
	if n, ok := v.Int64OK(); ok {
		return int(n), true
	}
	if n, ok := v.Int32OK(); ok {
		return int(n), true
	}
	return 0, false
}

// rawStrings counts the string entries of an array field.
func rawStrings(doc bson.Raw, key string) int {
	arr, ok := doc.Lookup(key).ArrayOK()
	if !ok {
		return 0
	}
	vals, _ := arr.Values()
	n := 0
	for _, v := range vals {
		if _, ok := v.StringValueOK(); ok {
			n++
		}
	}
	return n
}
