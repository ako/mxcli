// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/modelsdk/codec"
	"github.com/mendixlabs/mxcli/modelsdk/element"
	genDm "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genProj "github.com/mendixlabs/mxcli/modelsdk/gen/projects"
	genSec "github.com/mendixlabs/mxcli/modelsdk/gen/security"
	mmpr "github.com/mendixlabs/mxcli/modelsdk/mpr"
)

func init() {
	// A freshly-created module's contained units carry empty mandatory collections
	// Studio Pro always serializes (verified against the legacy writer + real BSON).
	codec.RegisterTypeDefaults("DomainModels$DomainModel", codec.TypeDefaults{
		MandatoryLists: []string{"Annotations", "Entities", "Associations", "CrossAssociations"},
	})
	codec.RegisterTypeDefaults("Security$ModuleSecurity", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"ModuleRoles": 1}, // by-name role list
	})
	codec.RegisterTypeDefaults("Projects$ModuleSettings", codec.TypeDefaults{
		MandatoryListMarkers: map[string]int32{"JarDependencies": 2},
	})
}

// CreateModule creates a new module and its mandatory contained units (an empty
// domain model, module security, and module settings), mirroring the legacy
// writer. Modules live under the project root.
func (b *Backend) CreateModule(m *model.Module) error {
	if m == nil {
		return fmt.Errorf("CreateModule: nil module")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateModule: not connected for writing")
	}
	if m.ID == "" {
		m.ID = model.ID(mmpr.GenerateID())
	}
	rootID, err := b.reader.GetProjectRootID()
	if err != nil {
		return fmt.Errorf("CreateModule: project root: %w", err)
	}
	enc := &codec.Encoder{}

	// Append at the end of the module list (NewSortIndex is a display order).
	sortIdx := float64(len(b.mustListModules()))

	// 1. ModuleImpl (the module unit itself).
	gm := genProj.NewModule()
	gm.SetID(element.ID(m.ID))
	gm.SetName(m.Name)
	gm.SetFromAppStore(m.FromAppStore)
	gm.SetAppStoreGuid(m.AppStoreGuid)
	gm.SetAppStoreVersion(m.AppStoreVersion)
	gm.SetAppStoreVersionGuid("")
	gm.SetAppStorePackageIdString("")
	gm.SetIsThemeModule(false)
	gm.SetSortIndex(sortIdx)
	contents, err := enc.Encode(gm)
	if err != nil {
		return fmt.Errorf("CreateModule: encode module: %w", err)
	}
	if err := b.writer.InsertUnit(string(m.ID), rootID, "Modules", "Projects$ModuleImpl", contents); err != nil {
		return fmt.Errorf("CreateModule: insert module: %w", err)
	}

	// 2. Empty domain model.
	if err := b.insertChildUnit(enc, m.ID, "DomainModel", "DomainModels$DomainModel", func() element.Element {
		d := genDm.NewDomainModel()
		d.SetDocumentation("")
		return d
	}); err != nil {
		return err
	}

	// 3. Module security (empty role list).
	if err := b.insertChildUnit(enc, m.ID, "ModuleSecurity", "Security$ModuleSecurity", func() element.Element {
		return genSec.NewModuleSecurity()
	}); err != nil {
		return err
	}

	// 4. Module settings (Studio Pro defaults for a new module).
	if err := b.insertChildUnit(enc, m.ID, "ModuleSettings", "Projects$ModuleSettings", func() element.Element {
		s := genProj.NewModuleSettings()
		s.SetExportLevel("Source")
		s.SetProtectedModuleType("AddOn")
		s.SetVersion("1.0.0")
		s.SetSolutionIdentifier("")
		s.SetExtensionName("")
		s.SetBasedOnVersion("")
		return s
	}); err != nil {
		return err
	}
	return nil
}

// UpdateModule rewrites an existing module unit. The read path (GetModule) only
// surfaces Name+ID, so a rename is the only meaningful change; we decode the gen
// module and set Name alone, preserving FromAppStore / SortIndex / IsThemeModule
// and every other field the semantic model does not carry (ADR-0005: mutate only
// what changed, never round-trip through a lossy model). References to the old
// qualified name are fixed separately via UpdateQualifiedNameInAllUnits.
func (b *Backend) UpdateModule(m *model.Module) error {
	if m == nil {
		return fmt.Errorf("UpdateModule: nil module")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateModule: not connected for writing")
	}
	if m.ID == "" {
		return fmt.Errorf("UpdateModule: module has no ID")
	}
	raw, err := b.reader.GetRawUnitBytes(string(m.ID))
	if err != nil {
		return fmt.Errorf("UpdateModule: read unit %s: %w", m.ID, err)
	}
	el, err := codec.NewDecoder(codec.DefaultRegistry).Decode(raw)
	if err != nil {
		return fmt.Errorf("UpdateModule: decode unit %s: %w", m.ID, err)
	}
	gm, ok := el.(*genProj.Module)
	if !ok {
		return fmt.Errorf("UpdateModule: unit %s is not a Module (%s)", m.ID, el.TypeName())
	}
	gm.SetName(m.Name)
	return b.persistUnit(m.ID, gm)
}

// insertChildUnit builds, encodes, and inserts a fresh-ID'd unit contained in the
// module under the given containment name.
func (b *Backend) insertChildUnit(enc *codec.Encoder, moduleID model.ID, containment, unitType string, build func() element.Element) error {
	id := mmpr.GenerateID()
	el := build()
	if ider, ok := el.(interface{ SetID(element.ID) }); ok {
		ider.SetID(element.ID(id))
	}
	contents, err := enc.Encode(el)
	if err != nil {
		return fmt.Errorf("CreateModule: encode %s: %w", unitType, err)
	}
	if err := b.writer.InsertUnit(id, string(moduleID), containment, unitType, contents); err != nil {
		return fmt.Errorf("CreateModule: insert %s: %w", unitType, err)
	}
	return nil
}

func (b *Backend) mustListModules() []*model.Module {
	mods, err := b.ListModules()
	if err != nil {
		return nil
	}
	return mods
}
