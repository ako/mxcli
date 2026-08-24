// SPDX-License-Identifier: Apache-2.0

package mpk

import "testing"

// The `defaultType` attribute is read from the widget XML. Without this the
// generator has nothing to carry, and every property that declares one is stored
// as "None" — CE0463. Six of File Uploader 2.5.0's properties declare it; no other
// widget in a stock 11.13.0 app declares any, which is why it went unnoticed.
func TestParse_ReadsDefaultTypeAttribute(t *testing.T) {
	const widgetXML = `<?xml version="1.0" encoding="utf-8"?>
<widget id="com.example.W" pluginWidget="true" needsEntityContext="true" xmlns="http://www.mendix.com/widget/1.0/">
  <name>W</name>
  <properties>
    <propertyGroup caption="General">
      <property key="associatedFiles" type="datasource" isList="true" defaultType="Association" defaultValue="M.Ctx/M.Assoc/M.File">
        <caption>Associated files</caption><description/>
      </property>
      <property key="createFileAction" type="action" defaultType="CallNanoflow" defaultValue="M.ACT_Create">
        <caption>Create</caption><description/>
      </property>
      <property key="plainAction" type="action">
        <caption>Plain</caption><description/>
      </property>
    </propertyGroup>
  </properties>
</widget>`

	def := parseWidgetXML(t, widgetXML)
	want := map[string]string{
		"associatedFiles":  "Association",
		"createFileAction": "CallNanoflow",
		"plainAction":      "",
	}
	for _, p := range def.Properties {
		if w, ok := want[p.Key]; ok {
			if p.DefaultType != w {
				t.Errorf("%s: DefaultType = %q, want %q", p.Key, p.DefaultType, w)
			}
			delete(want, p.Key)
		}
	}
	for k := range want {
		t.Errorf("property %q was not parsed at all", k)
	}
}
