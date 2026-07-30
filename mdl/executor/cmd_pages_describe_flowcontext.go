// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// dataSourceEntityContext returns the entity a datasource puts in scope for the
// widgets inside it.
//
// For a database or association source the datasource reference *is* the entity.
// For a microflow or nanoflow source it is the flow's own qualified name, so using
// it directly named the flow as the context entity — `-- Context: $currentObject
// (Module.GetOrders)` instead of `(Module.Order)` — and any consumer that resolves
// attributes against the context was looking up the wrong element. The flow's
// return type is resolved instead, falling back to the reference when the flow
// cannot be found or does not return an object/list, which is what the caller
// used before.
func dataSourceEntityContext(ctx *ExecContext, ds *rawDataSource) string {
	if ds == nil || ds.Reference == "" {
		return ""
	}
	switch ds.Type {
	case "microflow", "nanoflow":
		if entity := flowReturnEntity(ctx, ds.Type, ds.Reference); entity != "" {
			return entity
		}
	}
	return ds.Reference
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
		mfs, err := ctx.Backend.ListMicroflows()
		if err != nil {
			return ""
		}
		for _, mf := range mfs {
			if mf != nil && h.GetQualifiedName(mf.ContainerID, mf.Name) == qualifiedName {
				return dataTypeEntity(mf.ReturnType)
			}
		}
	case "nanoflow":
		nfs, err := ctx.Backend.ListNanoflows()
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
