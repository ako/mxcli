// SPDX-License-Identifier: Apache-2.0

// Package executor - rendering both sides of a diff through one describer.
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/ast"
	mdlerrors "github.com/mendixlabs/mxcli/mdl/errors"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// flowNameMaps builds the ID → qualified-name maps renderMicroflowMDL needs to
// print entity and flow references. Shared so that the two sides of a diff
// resolve names identically; a map built for one side only would show a
// reference as a name on one side and a stub on the other.
func flowNameMaps(ctx *ExecContext) (entityNames, microflowNames map[model.ID]string, err error) {
	h, err := getHierarchy(ctx)
	if err != nil {
		return nil, nil, mdlerrors.NewBackend("build hierarchy", err)
	}

	entityNames = make(map[model.ID]string)
	domainModels, _ := ctx.Backend.ListDomainModels()
	for _, dm := range domainModels {
		modName := h.GetModuleName(dm.ContainerID)
		for _, entity := range dm.Entities {
			entityNames[entity.ID] = modName + "." + entity.Name
		}
	}

	microflowNames = make(map[model.ID]string)
	allMicroflows, err := ctx.Backend.ListMicroflows()
	if err != nil {
		return nil, nil, mdlerrors.NewBackend("list microflows", err)
	}
	for _, mf := range allMicroflows {
		microflowNames[mf.ID] = h.GetQualifiedName(mf.ContainerID, mf.Name)
	}
	allNanoflows, _ := ctx.Backend.ListNanoflows()
	for _, nf := range allNanoflows {
		microflowNames[nf.ID] = h.GetQualifiedName(nf.ContainerID, nf.Name)
	}
	return entityNames, microflowNames, nil
}

// renderFlowFromModel renders an in-memory flow as MDL through the same
// describer DESCRIBE and diff-local use.
//
// This is the whole point of the #997 fix. diff used to render its script side
// with a second AST-to-MDL renderer, which covered 18 of 43 activity types and
// silently emitted nothing for the rest — so a java-action call, a `download
// file` or a canvas annotation appeared in the diff as a deletion, and mxcli
// confidently reported that a script would gut a microflow that exec proved
// was a no-op. One renderer for both sides makes that class of false report
// unrepresentable rather than fixed case by case.
func renderFlowFromModel(ctx *ExecContext, flowType string, mf *microflows.Microflow, name ast.QualifiedName) (string, error) {
	entityNames, microflowNames, err := flowNameMaps(ctx)
	if err != nil {
		return "", err
	}
	return renderMicroflowMDL(ctx, flowType, mf, name, entityNames, microflowNames, nil), nil
}

// nanoflowAsMicroflow wraps a Nanoflow so renderMicroflowMDL can print it.
// ContainerID is carried so the `folder` line matches the microflow path;
// without it one side of a nanoflow diff would print a folder and the other
// would not.
func nanoflowAsMicroflow(nf *microflows.Nanoflow) *microflows.Microflow {
	if nf == nil {
		return nil
	}
	return &microflows.Microflow{
		BaseElement:        nf.BaseElement,
		ContainerID:        nf.ContainerID,
		Name:               nf.Name,
		Documentation:      nf.Documentation,
		Excluded:           nf.Excluded,
		Parameters:         nf.Parameters,
		ReturnType:         nf.ReturnType,
		ObjectCollection:   nf.ObjectCollection,
		AllowedModuleRoles: nf.AllowedModuleRoles,
	}
}
