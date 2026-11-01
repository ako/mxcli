// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"strings"

	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// dataSourceEntityContext returns the entity a datasource puts in scope for the
// widgets inside it.
//
// For a database source the datasource reference *is* the entity. For a
// microflow or nanoflow source it is the flow's own qualified name, so using it
// directly named the flow as the context entity — `-- Context: $currentObject
// (Module.GetOrders)` instead of `(Module.Order)` — and any consumer that resolves
// attributes against the context was looking up the wrong element. The flow's
// return type is resolved instead, falling back to the reference when the flow
// cannot be found or does not return an object/list, which is what the caller
// used before.
//
// For an association source the reference is the association's own qualified
// name (`Module.Order_Customer`), the same mistake with a different element. The
// entity it reaches is the destination stored on its last hop, which already
// accounts for the direction travelled (owner to child or back, reference or
// reference set) and for a specialization of the association's end; it is read
// rather than re-derived. A source that stores none keeps the reference.
//
// For a page or snippet parameter the reference is the parameter's name
// (`Entry`), which no consumer can compare with an entity: a pluggable
// property bound to an attribute of another entity described back bare, and
// re-exec bound it to the parameter's entity — CE1613 (mendixlabs/mxcli#1591).
// The parameter's entity, stored beside the name, is used instead.
func dataSourceEntityContext(ctx *ExecContext, ds *rawDataSource) string {
	if ds == nil || ds.Reference == "" {
		return ""
	}
	switch ds.Type {
	case "microflow", "nanoflow":
		if entity := flowReturnEntity(ctx, ds.Type, ds.Reference); entity != "" {
			return entity
		}
	case "association", "parameter":
		if ds.Destination != "" {
			return ds.Destination
		}
	}
	return ds.Reference
}

// flowContextUnresolved reports whether a data container's source is a flow
// whose returned entity cannot be determined — the flow is not in the project
// (Feedback v4.0.2's excluded ShareFeedback_Logo names a nanoflow the module
// does not ship), or it returns no object.
func flowContextUnresolved(ctx *ExecContext, ds *rawDataSource) bool {
	if ds == nil || ds.Reference == "" || (ds.Type != "microflow" && ds.Type != "nanoflow") {
		return false
	}
	return flowReturnEntity(ctx, ds.Type, ds.Reference) == ""
}

// withQualifiedAttrs runs parse with attribute bindings kept qualified when
// ds leaves no entity in scope, restoring the previous setting afterwards.
func withQualifiedAttrs[T any](ctx *ExecContext, ds *rawDataSource, parse func() T) T {
	if ctx == nil || !flowContextUnresolved(ctx, ds) {
		return parse()
	}
	prev := ctx.describeQualifyAttrs
	ctx.describeQualifyAttrs = true
	defer func() { ctx.describeQualifyAttrs = prev }()
	return parse()
}

// describeAttr renders a stored attribute name for MDL: bare, as exec resolves
// it against the data container's entity — or, where there is no entity to
// resolve against (describeQualifyAttrs), the stored Module.Entity.Attr. A bare
// name there cannot be qualified on the way back: written anyway, a bare image
// parameter left a project `mx check` could not load.
func describeAttr(ctx *ExecContext, qn string) string {
	if ctx != nil && ctx.describeQualifyAttrs && strings.Count(qn, ".") >= 2 {
		return qn
	}
	return shortAttributeName(qn)
}

// flowReturnEntity resolves the entity a microflow or nanoflow returns, by object
// or list return type. Returns "" when the project is unavailable, the flow is not
// found, or it returns something other than an object/list.
func flowReturnEntity(ctx *ExecContext, kind, qualifiedName string) string {
	if ctx == nil || ctx.Backend == nil || qualifiedName == "" {
		return ""
	}
	h, err := getHierarchy(ctx)
	if err != nil || h == nil {
		return ""
	}
	switch kind {
	case "microflow":
		// Only the microflows of this name are decoded: a page describe asks
		// once per flow-sourced widget, and listing them all read every
		// microflow of the project each time (ako/mxcli#1791).
		_, local, _ := strings.Cut(qualifiedName, ".")
		mfs, err := microflowsNamed(ctx, local)
		if err != nil {
			return ""
		}
		for _, mf := range mfs {
			if mf != nil && h.GetQualifiedName(mf.ContainerID, mf.Name) == qualifiedName {
				return dataTypeEntity(mf.ReturnType)
			}
		}
	case "nanoflow":
		_, local, _ := strings.Cut(qualifiedName, ".")
		nfs, err := nanoflowsNamed(ctx.Backend, local)
		if err != nil {
			return ""
		}
		for _, nf := range nfs {
			if nf != nil && h.GetQualifiedName(nf.ContainerID, nf.Name) == qualifiedName {
				return dataTypeEntity(nf.ReturnType)
			}
		}
	}
	return ""
}

// dataTypeEntity returns the entity a return type refers to, for the two kinds a
// data container can bind against. Both the pointer and value forms are matched
// because the parsers are not consistent about which they produce.
func dataTypeEntity(dt microflows.DataType) string {
	switch t := dt.(type) {
	case *microflows.ObjectType:
		return t.EntityQualifiedName
	case microflows.ObjectType:
		return t.EntityQualifiedName
	case *microflows.ListType:
		return t.EntityQualifiedName
	case microflows.ListType:
		return t.EntityQualifiedName
	}
	return ""
}
