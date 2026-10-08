// SPDX-License-Identifier: Apache-2.0

// Package executor - the read cache derivedFlowLayout builds through.
package executor

import (
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/domainmodel"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// layoutCheckBackend memoises the lookups the flow builder repeats on every
// build. derivedFlowLayout builds the same flow several times over an
// unchanged project, and without this nearly all of its cost was the builder
// re-reading the project from disk to learn the same things each round — which
// microflows exist, what a called one returns, a Java action's parameters.
//
// It lives only for one derivation, which writes nothing, so nothing it caches
// can go stale. Everything it does not override is the real backend.
type layoutCheckBackend struct {
	backend.FullBackend

	modules      *memoResult[[]*model.Module]
	microflows   *memoResult[[]*microflows.Microflow]
	nanoflows    *memoResult[[]*microflows.Nanoflow]
	domainModels *memoResult[[]*domainmodel.DomainModel]

	moduleByName map[string]memoResult[*model.Module]
	domainModel  map[model.ID]memoResult[*domainmodel.DomainModel]
	rawUnit      map[[2]string]memoResult[*types.RawUnitInfo]
	javaAction   map[string]memoResult[*javaactions.JavaAction]
	jsAction     map[string]memoResult[*types.JavaScriptAction]
	isRule       map[string]memoResult[bool]
}

type memoResult[T any] struct {
	value T
	err   error
}

func newLayoutCheckBackend(b backend.FullBackend) *layoutCheckBackend {
	return &layoutCheckBackend{
		FullBackend:  b,
		moduleByName: map[string]memoResult[*model.Module]{},
		domainModel:  map[model.ID]memoResult[*domainmodel.DomainModel]{},
		rawUnit:      map[[2]string]memoResult[*types.RawUnitInfo]{},
		javaAction:   map[string]memoResult[*javaactions.JavaAction]{},
		jsAction:     map[string]memoResult[*types.JavaScriptAction]{},
		isRule:       map[string]memoResult[bool]{},
	}
}

func memoOnce[T any](slot **memoResult[T], load func() (T, error)) (T, error) {
	if *slot == nil {
		v, err := load()
		*slot = &memoResult[T]{value: v, err: err}
	}
	return (*slot).value, (*slot).err
}

func memoKeyed[K comparable, T any](m map[K]memoResult[T], key K, load func() (T, error)) (T, error) {
	if r, ok := m[key]; ok {
		return r.value, r.err
	}
	v, err := load()
	m[key] = memoResult[T]{value: v, err: err}
	return v, err
}

func (b *layoutCheckBackend) ListModules() ([]*model.Module, error) {
	return memoOnce(&b.modules, b.FullBackend.ListModules)
}

func (b *layoutCheckBackend) ListMicroflows() ([]*microflows.Microflow, error) {
	return memoOnce(&b.microflows, b.FullBackend.ListMicroflows)
}

func (b *layoutCheckBackend) ListNanoflows() ([]*microflows.Nanoflow, error) {
	return memoOnce(&b.nanoflows, b.FullBackend.ListNanoflows)
}

func (b *layoutCheckBackend) ListDomainModels() ([]*domainmodel.DomainModel, error) {
	return memoOnce(&b.domainModels, b.FullBackend.ListDomainModels)
}

func (b *layoutCheckBackend) GetModuleByName(name string) (*model.Module, error) {
	return memoKeyed(b.moduleByName, name, func() (*model.Module, error) { return b.FullBackend.GetModuleByName(name) })
}

func (b *layoutCheckBackend) GetDomainModel(moduleID model.ID) (*domainmodel.DomainModel, error) {
	return memoKeyed(b.domainModel, moduleID, func() (*domainmodel.DomainModel, error) { return b.FullBackend.GetDomainModel(moduleID) })
}

func (b *layoutCheckBackend) GetRawUnitByName(objectType, qualifiedName string) (*types.RawUnitInfo, error) {
	return memoKeyed(b.rawUnit, [2]string{objectType, qualifiedName}, func() (*types.RawUnitInfo, error) {
		return b.FullBackend.GetRawUnitByName(objectType, qualifiedName)
	})
}

func (b *layoutCheckBackend) ReadJavaActionByName(qualifiedName string) (*javaactions.JavaAction, error) {
	return memoKeyed(b.javaAction, qualifiedName, func() (*javaactions.JavaAction, error) {
		return b.FullBackend.ReadJavaActionByName(qualifiedName)
	})
}

func (b *layoutCheckBackend) ReadJavaScriptActionByName(qualifiedName string) (*types.JavaScriptAction, error) {
	return memoKeyed(b.jsAction, qualifiedName, func() (*types.JavaScriptAction, error) {
		return b.FullBackend.ReadJavaScriptActionByName(qualifiedName)
	})
}

// IsRule is asked once per `if Module.Name(...)` split in every build, and the
// answer cannot change while nothing is written.
func (b *layoutCheckBackend) IsRule(qualifiedName string) (bool, error) {
	return memoKeyed(b.isRule, qualifiedName, func() (bool, error) { return b.FullBackend.IsRule(qualifiedName) })
}
