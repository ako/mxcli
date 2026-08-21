// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestGenPropertyKeysAgainstMetamodel cross-checks every property key
// modelsdk/gen binds against the storage name generated/metamodel records.
//
// Mendix's reflection data carries TWO names per property: an SDK-facing Name
// and the StorageName that is the actual BSON key. cmd/codegen — the generator
// that IS in this repo — keeps them apart, emitting the storage name as the
// json tag:
//
//	generated/metamodel/types.go
//	    RegularExpression model.QualifiedName `json:"regExIdentifier,omitempty"`
//	                    ^ SDK name                     ^ storage name
//
// The out-of-tree generator that produced modelsdk/gen kept only the SDK name,
// so ~100 properties are bound to a key Mendix does not read. gen cannot be
// regenerated here (cmd/modelsdk-codegen and internal/codegen/supplements.json
// have never existed in this repo, and /reference/ is gitignored), so each one
// is fixed by hand as a STORAGE-NAME OVERRIDE in its init<Type> function.
//
// This test is a LEDGER, not a clean bill of health. knownWrongKeys lists the
// unfixed ones. It fails when:
//
//   - a NEW mismatch appears — someone re-vendored gen and dropped an override,
//     which is otherwise invisible until a user's document loses a property;
//   - a listed one is FIXED without being struck off, so the ledger stays honest.
//
// Three entries here have been confirmed against real Studio Pro documents:
// RegularExpression.Expression (five documents), RegExRuleInfo.RegExIdentifier
// (CE0135 on the wrong key, mxbuild 11.13) and Attribute.GUID (worked around in
// codec/encoder.go). Fixing the rest needs the same per-property evidence — see
// CLAUDE.md, "modelsdk/gen Binds Some Properties Under the Wrong BSON Key".
type keyMismatch struct {
	TypeName   string // BSON $Type
	GenKey     string // what modelsdk/gen binds (the SDK name)
	StorageKey string // what Mendix actually stores
}

var knownWrongKeys = []keyMismatch{
	{"CustomWidgets$CustomWidgetType", "NeedsEntityContext", "WidgetNeedsEntityContext"},
	{"CustomWidgets$CustomWidgetType", "PluginWidget", "WidgetPluginWidget"},
	{"CustomWidgets$CustomWidgetType", "Name", "WidgetName"},
	{"CustomWidgets$CustomWidgetType", "Description", "WidgetDescription"},
	{"CustomWidgets$WidgetEnumerationValue", "Key", "_Key"},
	{"CustomWidgets$WidgetPropertyType", "Key", "PropertyKey"},
	{"CustomWidgets$WidgetValue", "Page", "Form"},
	{"CustomWidgets$WidgetValueType", "AttributeTypes", "AllowedTypes"},
	{"DataSets$DataSetNumericConstraint", "Begin", "_Begin"},
	{"DataSets$DataSetNumericConstraint", "End", "_End"},
	{"DataSets$DataSetObjectConstraint", "Constraint", "XPathConstraint"},
	{"DataSets$OqlDataSetSource", "IgnoreErrorsInQuery", "IEIQ"},
	{"DataTransformers$DataTransformer", "RootElement", "RootElementPointer"},
	{"DataTransformers$Step", "InputElement", "InputElementPointer"},
	{"DataTransformers$Step", "OutputElement", "OutputElementPointer"},
	{"DataTransformers$StructureAttribute", "Value", "ValuePointer"},
	{"DocumentTemplates$ConditionSettings", "Conditions", "NewConditions"},
	{"DocumentTemplates$DataGrid", "SortBar", "DbSortBar"},
	{"DocumentTemplates$DocumentTemplate", "ShowHeaderAndFooterOnFirstPage", "ShowHeaderFooterFirstPage"},
	{"DocumentTemplates$DynamicImageViewer", "DefaultImage", "DefaultImageName"},
	{"DocumentTemplates$DynamicLabel", "RenderXHTML", "RenderHTML"},
	{"DocumentTemplates$StaticImageViewer", "Image", "ImageId"},
	{"DocumentTemplates$Style", "OverrideBold", "OverrideFontWeight"},
	{"DocumentTemplates$Style", "OverrideItalic", "OverrideFontStyle"},
	{"DocumentTemplates$Style", "CustomStyles", "CustomCss"},
	{"DocumentTemplates$TemplateGrid", "SortBar", "DbSortBar"},
	{"DocumentTemplates$TemplateGrid", "OddRowsContents", "OddRowsDropZone"},
	{"DocumentTemplates$TemplateGrid", "EvenRowsContents", "EvenRowsDropZone"},
	{"DomainModels$Association", "DataStorageGuid", "GUID"},
	{"DomainModels$Attribute", "DataStorageGuid", "GUID"},
	{"DomainModels$CrossAssociation", "DataStorageGuid", "GUID"},
	{"DomainModels$EqualsToRuleInfo", "EqualsToValue", "Value"},
	{"ExportMappings$ExportMapping", "RootElementName", "XsdRootElementName"},
	{"ExportMappings$ExportMapping", "ImportedWebService", "WsdlFile"},
	{"ExportMappings$ExportMapping", "IsHeader", "IsHeaderParameter"},
	{"Images$Image", "ImageData", "Image"},
	{"ImportMappings$ImportMapping", "RootElementName", "XsdRootElementName"},
	{"ImportMappings$ImportMapping", "ImportedWebService", "WsdlFile"},
	{"JavaActions$JavaAction", "ActionTypeParameters", "TypeParameters"},
	{"JavaActions$JavaAction", "ActionReturnType", "JavaReturnType"},
	{"JavaActions$JavaAction", "ModelerActionInfo", "MicroflowActionInfo"},
	{"JavaActions$JavaAction", "ActionParameters", "Parameters"},
	{"JavaActions$JavaActionParameter", "ActionParameterType", "ParameterType"},
	{"JavaScriptActions$JavaScriptAction", "ActionTypeParameters", "TypeParameters"},
	{"JavaScriptActions$JavaScriptAction", "ActionReturnType", "JavaReturnType"},
	{"JavaScriptActions$JavaScriptAction", "ModelerActionInfo", "MicroflowActionInfo"},
	{"JavaScriptActions$JavaScriptAction", "ActionParameters", "Parameters"},
	{"JavaScriptActions$JavaScriptActionParameter", "ActionParameterType", "ParameterType"},
	{"Microflows$Contains", "ListVariableName", "ListName"},
	{"Microflows$Contains", "SecondListOrObjectVariableName", "SecondListOrObjectName"},
	{"Microflows$Filter", "ListVariableName", "ListName"},
	{"Microflows$FilterByExpression", "ListVariableName", "ListName"},
	{"Microflows$Find", "ListVariableName", "ListName"},
	{"Microflows$FindByExpression", "ListVariableName", "ListName"},
	{"Microflows$Head", "ListVariableName", "ListName"},
	{"Microflows$HttpConfiguration", "UseAuthentication", "UseHttpAuthentication"},
	{"Microflows$HttpConfiguration", "AuthenticationPassword", "HttpAuthenticationPassword"},
	{"Microflows$HttpConfiguration", "HeaderEntries", "HttpHeaderEntries"},
	{"Microflows$HttpConfiguration", "NewHttpMethod", "HttpMethod"},
	{"Microflows$ImportMappingCall", "Mapping", "ReturnValueMapping"},
	{"Microflows$ImportMappingCall", "MappingArgumentVariableName", "ParameterVariableName"},
	{"Microflows$Intersect", "ListVariableName", "ListName"},
	{"Microflows$Intersect", "SecondListOrObjectVariableName", "SecondListOrObjectName"},
	{"Microflows$JavaActionParameterMapping", "ParameterValue", "Value"},
	{"Microflows$ListEquals", "ListVariableName", "ListName"},
	{"Microflows$ListEquals", "SecondListOrObjectVariableName", "SecondListOrObjectName"},
	{"Microflows$ListRange", "ListVariableName", "ListName"},
	{"Microflows$MappingRequestHandling", "Mapping", "MappingId"},
	{"Microflows$MappingRequestHandling", "MappingArgumentVariableName", "MappingVariableName"},
	// {"Microflows$RuleCall", "Rule", "Microflow"} — FIXED (#939): a rule split
	// wrote the reference under "Rule", which Mendix never reads, so the decision
	// lost its condition (CE0080). Overridden in initRuleCall + InitFromRaw.
	{"Microflows$Sort", "ListVariableName", "ListName"},
	{"Microflows$Sort", "SortItemList", "Sortings"},
	{"Microflows$Subtract", "ListVariableName", "ListName"},
	{"Microflows$Subtract", "SecondListOrObjectVariableName", "SecondListOrObjectName"},
	{"Microflows$Tail", "ListVariableName", "ListName"},
	{"Microflows$Union", "ListVariableName", "ListName"},
	{"Microflows$Union", "SecondListOrObjectVariableName", "SecondListOrObjectName"},
	{"Microflows$WebServiceOperationAdvancedParameterMapping", "MappingArgumentVariableName", "MappingVariableName"},
	{"ODataPublish$EntitySet", "EntityType", "EntityTypePointer"},
	{"Projects$ProjectConversion", "Markers", "OneTimeConversions"},
	{"RegularExpressions$RegularExpression", "RegEx", "Expression"},
	{"Reports$BasicReportColumn", "Width", "Weight"},
	{"Reports$ReportZoomInfo", "TargetPage", "TargetForm"},
	{"Reports$ReportZoomMapping", "TargetParameterName", "Parameter"},
	{"Rest$ODataRemoteEntitySource", "EntityTypeName", "RemoteName"},
	{"Rest$ODataRemoteEntitySource", "EntitySetName", "EntitySet"},
	{"Security$ProjectSecurity", "AdminUserRoleName", "AdminUserRole"},
	{"Security$ProjectSecurity", "GuestUserRoleName", "GuestUserRole"},
	{"Settings$Configuration", "RuntimePortNumber", "HttpPortNumber"},
	{"Settings$Configuration", "AdminPortNumber", "ServerPortNumber"},
	{"Settings$Configuration", "RuntimePortOnlyLocal", "OpenHttpPort"},
	{"Settings$Configuration", "AdminPortOnlyLocal", "OpenAdminPort"},
	{"Settings$ConstantValue", "Constant", "ConstantId"},
	{"Texts$SystemText", "Key", "InternalKey"},
	{"WebServices$ImportedWebService", "WsdlDescription", "Description"},
	{"WebServices$PublishedParameter", "Parameter", "MicroflowParameter"},
	{"WebServices$PublishedParameter", "EntityExposedName", "ElementName"},
	{"WebServices$PublishedParameter", "EntityExposedItemName", "ObjectElementName"},
	{"WebServices$SystemIdDataAttribute", "ExposedName", "ElementName"},
	{"WebServices$WsdlDescription", "WsdlEntries", "WsdlContentss"},
	{"WebServices$WsdlDescription", "SchemaEntries", "SchemaContentss"},
	{"XmlSchemas$XmlSchema", "Entries", "SchemaContentss"},
}

var (
	reInitFunc  = regexp.MustCompile(`(?sm)^func init(\w+)\(\) \*\w+ \{\n(.*?)^\}`)
	reTypeName  = regexp.MustCompile(`SetTypeName\("([^"]+)"\)`)
	rePropKey   = regexp.MustCompile(`property\.New\w+(?:\[[^\]]+\])?\("([^"]+)"`)
	reStructDef = regexp.MustCompile(`(?sm)^type (\w+) struct \{\n(.*?)^\}`)
	reField     = regexp.MustCompile("(?m)^\t(\\w+)\\s+[^`\n]+`json:\"([^\",]+)")
)

func TestGenPropertyKeysAgainstMetamodel(t *testing.T) {
	// generated/metamodel: struct name -> SDK field name -> storage key.
	metaSrc, err := os.ReadFile(filepath.Join("..", "..", "generated", "metamodel", "types.go"))
	if err != nil {
		t.Fatalf("read metamodel: %v", err)
	}
	meta := map[string]map[string]string{}
	for _, m := range reStructDef.FindAllStringSubmatch(string(metaSrc), -1) {
		fields := map[string]string{}
		for _, f := range reField.FindAllStringSubmatch(m[2], -1) {
			// The json tag lowercases the first letter of the storage name
			// (Expression -> expression, GUID -> gUID); undo that.
			fields[f[1]] = strings.ToUpper(f[2][:1]) + f[2][1:]
		}
		meta[m[1]] = fields
	}
	if len(meta) == 0 {
		t.Fatal("parsed no structs from generated/metamodel — the parser has drifted, not the code")
	}

	files, err := filepath.Glob(filepath.Join("*", "types.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no gen packages found: %v", err)
	}

	var found []keyMismatch
	checked := 0
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, fn := range reInitFunc.FindAllStringSubmatch(string(src), -1) {
			tn := reTypeName.FindStringSubmatch(fn[2])
			if tn == nil {
				continue
			}
			typeName := tn[1]

			// "DomainModels$RegExRuleInfo" -> struct "DomainModelsRegExRuleInfo"
			domain, short, ok := strings.Cut(typeName, "$")
			if !ok {
				continue
			}
			fields, ok := meta[domain+short]
			if !ok {
				continue // no metamodel counterpart to compare against
			}
			checked++

			storage := map[string]bool{}
			for _, key := range fields {
				storage[strings.ToLower(key)] = true
			}
			for _, k := range rePropKey.FindAllStringSubmatch(fn[2], -1) {
				key := k[1]
				if storage[strings.ToLower(key)] {
					continue // gen already uses the storage name
				}
				// Not a storage name. If it matches an SDK field name whose
				// storage name differs, gen kept the wrong half.
				for sdkName, storageKey := range fields {
					if strings.EqualFold(sdkName, key) && !strings.EqualFold(storageKey, key) {
						found = append(found, keyMismatch{typeName, key, storageKey})
						break
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("cross-checked no types — the init-function parser has drifted")
	}
	t.Logf("cross-checked %d gen types against generated/metamodel", checked)

	key := func(m keyMismatch) string { return m.TypeName + "\x00" + m.GenKey }
	known := map[string]keyMismatch{}
	for _, m := range knownWrongKeys {
		known[key(m)] = m
	}
	actual := map[string]keyMismatch{}
	for _, m := range found {
		actual[key(m)] = m
	}

	var appeared, fixed []string
	for k, m := range actual {
		if want, ok := known[k]; !ok {
			appeared = append(appeared, m.TypeName+"."+m.GenKey+" (should be "+m.StorageKey+")")
		} else if want.StorageKey != m.StorageKey {
			appeared = append(appeared, m.TypeName+"."+m.GenKey+" storage changed: "+want.StorageKey+" -> "+m.StorageKey)
		}
	}
	for k, m := range known {
		if _, ok := actual[k]; !ok {
			fixed = append(fixed, m.TypeName+"."+m.GenKey)
		}
	}
	sort.Strings(appeared)
	sort.Strings(fixed)

	for _, s := range appeared {
		t.Errorf("NEW wrong BSON key in modelsdk/gen: %s\n"+
			"\tgen writes a property Mendix does not read. Fix it with a STORAGE-NAME\n"+
			"\tOVERRIDE in the init<Type> function AND the InitFromRaw decode key, or\n"+
			"\tadd it to knownWrongKeys if nothing writes this type yet.", s)
	}
	for _, s := range fixed {
		t.Errorf("FIXED but still listed in knownWrongKeys: %s\n"+
			"\tStrike it off — a stale ledger stops being evidence of anything.", s)
	}
}
