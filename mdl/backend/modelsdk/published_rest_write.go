// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
	"github.com/mendixlabs/mxcli/modelsdk/property"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

func init() {
	// List markers as Studio Pro 11 writes them (measured on ako/TestApp's
	// Services.OrdersRestApi): Resources and operation Parameters use 3,
	// Operations uses 2, and the service's empty Parameters list is [3]. The
	// service's AllowedRoles and AuthenticationTypes are marker-1 string lists,
	// and CorsConfiguration is BSON null.
	codec.RegisterListMarker("Rest$PublishedRestServiceResource", 3)
	codec.RegisterListMarker("Rest$PublishedRestServiceOperation", 2)
	codec.RegisterListMarker("Rest$RestOperationParameter", 3)
	codec.RegisterTypeDefaults("Rest$PublishedRestService", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"AllowedRoles": 1, "AuthenticationTypes": 1, "Parameters": 3},
		NullFields:           []string{"CorsConfiguration"},
	})
	codec.RegisterTypeDefaults("Rest$PublishedRestServiceResource", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Operations": 2},
	})
	codec.RegisterTypeDefaults("Rest$PublishedRestServiceOperation", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"Parameters": 3},
	})
}

// CreatePublishedRestService inserts a new Rest$PublishedRestService document
// (resources → operations → path parameters). Mirrors the legacy serializer.
func (b *Backend) CreatePublishedRestService(svc *model.PublishedRestService) error {
	if svc == nil {
		return fmt.Errorf("CreatePublishedRestService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("CreatePublishedRestService: not connected for writing")
	}
	if svc.ID == "" {
		svc.ID = model.ID(mmpr.GenerateID())
	}
	svc.TypeName = "Rest$PublishedRestService"
	contents, err := (&codec.Encoder{}).Encode(publishedRestServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("CreatePublishedRestService: encode: %w", err)
	}
	return b.writer.InsertUnit(string(svc.ID), string(svc.ContainerID), "Documents", "Rest$PublishedRestService", contents)
}

// UpdatePublishedRestService rewrites an existing published REST service in place
// (CREATE OR MODIFY / ALTER PUBLISHED REST SERVICE).
func (b *Backend) UpdatePublishedRestService(svc *model.PublishedRestService) error {
	if svc == nil {
		return fmt.Errorf("UpdatePublishedRestService: nil service")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdatePublishedRestService: not connected for writing")
	}
	contents, err := (&codec.Encoder{}).Encode(publishedRestServiceToGen(svc))
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestService: encode: %w", err)
	}
	// publishedRestServiceToGen writes ExportLevel "Hidden" as a constant (#816).
	contents, err = b.keepStoredExportLevel(string(svc.ID), contents)
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestService: %w", err)
	}
	// ... and the service-level properties MDL has no spelling for as
	// constants too: carry the stored ones (ako/mxcli#571).
	contents, err = b.keepStoredTopLevel(string(svc.ID), contents, publishedRestServiceUnauthored)
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestService: %w", err)
	}
	return b.writer.UpdateRawUnit(string(svc.ID), contents)
}

// DeletePublishedRestService removes a published REST service unit by ID.
func (b *Backend) DeletePublishedRestService(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeletePublishedRestService: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// UpdatePublishedRestServiceRoles patches just the AllowedRoles field (marker-1
// reference-string array) on an existing service, preserving the rest of the
// document. Used by GRANT/REVOKE on a published REST service.
func (b *Backend) UpdatePublishedRestServiceRoles(unitID model.ID, roles []string) error {
	if b.writer == nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(unitID))
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: load unit: %w", err)
	}
	var d bson.D
	if err := bson.Unmarshal(raw, &d); err != nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: unmarshal: %w", err)
	}
	arr := bson.A{int32(1)}
	for _, r := range roles {
		arr = append(arr, r)
	}
	set := false
	for i := range d {
		if d[i].Key == "AllowedRoles" {
			d[i].Value = arr
			set = true
			break
		}
	}
	if !set {
		d = append(d, bson.E{Key: "AllowedRoles", Value: arr})
	}
	out, err := bson.Marshal(d)
	if err != nil {
		return fmt.Errorf("UpdatePublishedRestServiceRoles: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(unitID), out)
}

// publishedRestServiceUnauthored are the Rest$PublishedRestService keys a
// create or modify / alter cannot state: the writer emits a constant for each,
// so a rewrite carries the stored value instead.
//
// AuthenticationTypes and AuthenticationMicroflow were here until MDL could
// state them (mendixlabs/mxcli#1331); before that, executing the describe
// output of a Studio Pro service turned its Basic and Session authentication
// off (ako/mxcli#571). They are written from the model now, and an unstated
// setting is carried by the executor, which reads it with the service.
var publishedRestServiceUnauthored = []string{
	"CorsConfiguration",
	"Documentation",
	"Parameters",
	"PublicDocumentation",
}

func publishedRestServiceToGen(svc *model.PublishedRestService) element.Element {
	g := newElem("Rest$PublishedRestService", string(svc.ID))
	addStr(g, "Name", svc.Name)
	addStr(g, "Documentation", "")
	addBool(g, "Excluded", svc.Excluded)
	addStr(g, "ExportLevel", "Hidden")
	addStr(g, "Path", svc.Path)
	addStr(g, "Version", svc.Version)
	addStr(g, "ServiceName", svc.ServiceName)
	if len(svc.AllowedRoles) > 0 {
		addByNameRefList(g, "AllowedRoles", "Security$ModuleRole", svc.AllowedRoles)
	}
	// AllowedRoles (empty), Parameters, CorsConfiguration: emitted via the
	// registered TypeDefaults. AuthenticationTypes is a marker-1 string list in
	// the order given, which is the order Studio Pro stores the ticked methods
	// in; empty is "Requires authentication = No" (ako/TestApp, #1331).
	addStrList(g, "AuthenticationTypes", svc.AuthenticationTypes)
	addStr(g, "AuthenticationMicroflow", svc.AuthenticationMicroflow)

	resources := make([]element.Element, 0, len(svc.Resources))
	for _, res := range svc.Resources {
		r := newElem("Rest$PublishedRestServiceResource", string(res.ID))
		addStr(r, "Name", res.Name)
		addStr(r, "Documentation", "")
		ops := make([]element.Element, 0, len(res.Operations))
		for _, op := range res.Operations {
			ops = append(ops, publishedRestOperationToGen(op))
		}
		if len(ops) > 0 {
			addPartList(r, "Operations", ops)
		}
		resources = append(resources, r)
	}
	if len(resources) > 0 {
		addPartList(g, "Resources", resources)
	}
	return g
}

func publishedRestOperationToGen(op *model.PublishedRestOperation) element.Element {
	g := newElem("Rest$PublishedRestServiceOperation", string(op.ID))
	addStr(g, "HttpMethod", httpMethodToMendix(op.HTTPMethod))
	addStr(g, "Path", op.Path)
	addStr(g, "Microflow", op.Microflow)
	addStr(g, "Summary", op.Summary)
	addBool(g, "Deprecated", op.Deprecated)
	addStr(g, "Commit", orDefault(op.Commit, "Yes"))
	addStr(g, "Documentation", op.Documentation)
	addStr(g, "ExportMapping", op.ExportMapping)
	addStr(g, "ImportMapping", op.ImportMapping)
	addStr(g, "ObjectHandlingBackup", orDefault(op.ObjectHandlingBackup, "Create"))
	// The executor derives the parameters from the microflow (path, query, body),
	// as Studio Pro does. When it could not read the microflow only the path's
	// {name} placeholders are known: those are written as String path
	// parameters wired to the microflow parameter of that name, since without
	// that wiring mx check raises CE6538 / CE0350.
	opParams := op.OperationParameters
	if len(opParams) == 0 {
		for _, name := range op.PathParameterNames() {
			mfParam := ""
			if op.Microflow != "" {
				mfParam = op.Microflow + "." + name
			}
			opParams = append(opParams, &model.PublishedRestOperationParameter{
				Name: name, ParameterType: "Path", MicroflowParameter: mfParam, DataType: "String",
			})
		}
	}
	params := make([]element.Element, 0, len(opParams))
	for _, param := range opParams {
		p := newElem("Rest$RestOperationParameter", "")
		addStr(p, "Name", param.Name)
		addPart(p, "Type", publishedRestParameterTypeToGen(param))
		addStr(p, "ParameterType", param.ParameterType)
		addStr(p, "MicroflowParameter", param.MicroflowParameter)
		addStr(p, "Description", param.Description)
		params = append(params, p)
	}
	if len(params) > 0 {
		addPartList(g, "Parameters", params)
	}
	return g
}

// publishedRestParameterTypeToGen builds the DataTypes$* element of an
// operation parameter. Long is not among an operation parameter's types
// (Studio Pro 11.14's schema for Rest$RestOperationParameter.type): it is
// written as Integer, as microflowDataTypeToGen writes it.
func publishedRestParameterTypeToGen(p *model.PublishedRestOperationParameter) element.Element {
	switch p.DataType {
	case "Float":
		return newElem("DataTypes$FloatType", "")
	case "Empty":
		return newElem("DataTypes$EmptyType", "")
	case "Unknown":
		return newElem("DataTypes$UnknownType", "")
	}
	return microflowDataTypeToGen(publishedRestParameterDataType(p))
}

// publishedRestParameterDataType is the microflow data type an operation
// parameter carries.
func publishedRestParameterDataType(p *model.PublishedRestOperationParameter) microflows.DataType {
	switch p.DataType {
	case "Boolean":
		return &microflows.BooleanType{}
	case "Integer", "Long":
		return &microflows.IntegerType{}
	case "Decimal":
		return &microflows.DecimalType{}
	case "DateTime", "Date":
		return &microflows.DateTimeType{}
	case "Binary":
		return &microflows.BinaryType{}
	case "Enumeration":
		return &microflows.EnumerationType{EnumerationQualifiedName: p.QualifiedName}
	case "Object":
		return &microflows.ObjectType{EntityQualifiedName: p.QualifiedName}
	case "List":
		return &microflows.ListType{EntityQualifiedName: p.QualifiedName}
	case "Void":
		return nil
	default:
		return &microflows.StringType{}
	}
}

// addByNameRefList adds a marker-1 reference-string list property (qualified
// names), the form Mendix uses for AllowedRoles / AllowedModuleRoles.
func addByNameRefList(b *element.Base, name, targetType string, qnames []string) {
	p := property.NewByNameRefList[element.Element](name, targetType)
	b.AddProperty(p, uint(len(b.Properties())))
	for _, qn := range qnames {
		p.Append(qn)
	}
}

// httpMethodToMendix converts an HTTP method name to Mendix casing.
func httpMethodToMendix(method string) string {
	switch strings.ToUpper(method) {
	case "GET":
		return "Get"
	case "POST":
		return "Post"
	case "PUT":
		return "Put"
	case "PATCH":
		return "Patch"
	case "DELETE":
		return "Delete"
	case "HEAD":
		return "Head"
	case "OPTIONS":
		return "Options"
	default:
		return method
	}
}
