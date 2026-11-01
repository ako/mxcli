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
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// layoutCheckBackend memoises the lookups the flow builder repeats on every
// build. derivedFlowLayout builds the same flow several times over an
// unchanged project, and without this nearly all of its cost was the builder
// re-reading the project from disk to learn the same things each round — which
// microflows exist, what a called one returns, a Java action's parameters.
//
// It lives only for one derivation, which writes nothing, so nothing it caches
// can go stale. Everything it does not override is the real backend.
//
// One `create microflow` build is the same kind of span (flowBuildBackend):
// the builder only reads, and asked for a module and its domain model once per
// member assignment of every `create` action, each a project listing
// (mendixlabs/mxcli#1616).
type layoutCheckBackend struct {
	backend.FullBackend

	modules      *memoResult[[]*model.Module]
	microflows   *memoResult[[]*microflows.Microflow]
	nanoflows    *memoResult[[]*microflows.Nanoflow]
	domainModels *memoResult[[]*domainmodel.DomainModel]

	microflowHeaders *memoResult[[]types.DocumentHeader]
	nanoflowHeaders  *memoResult[[]types.DocumentHeader]
	pageHeaders      *memoResult[[]types.DocumentHeader]
	snippetHeaders   *memoResult[[]types.DocumentHeader]
	layoutHeaders    *memoResult[[]types.DocumentHeader]

	moduleByName    map[string]memoResult[*model.Module]
	microflowsNamed map[string]memoResult[[]*microflows.Microflow]
	domainModel     map[model.ID]memoResult[*domainmodel.DomainModel]
	rawUnit         map[[2]string]memoResult[*types.RawUnitInfo]
	javaAction      map[string]memoResult[*javaactions.JavaAction]
	jsAction        map[string]memoResult[*types.JavaScriptAction]
	isRule          map[string]memoResult[bool]
}

type memoResult[T any] struct {
	value T
	err   error
}

// flowBuildBackend is the backend one flow build reads through: memoised for
// that build, which writes nothing (the flow is stored after the builder is
// done). A nil backend stays nil — the builder checks for it — and one that is
// already memoised is not wrapped again.
func flowBuildBackend(b backend.FullBackend) backend.FullBackend {
	if b == nil {
		return nil
	}
	if _, ok := b.(*layoutCheckBackend); ok {
		return b
	}
	return newLayoutCheckBackend(b)
}

func newLayoutCheckBackend(b backend.FullBackend) *layoutCheckBackend {
	return &layoutCheckBackend{
		FullBackend:     b,
		moduleByName:    map[string]memoResult[*model.Module]{},
		microflowsNamed: map[string]memoResult[[]*microflows.Microflow]{},
		domainModel:     map[model.ID]memoResult[*domainmodel.DomainModel]{},
		rawUnit:         map[[2]string]memoResult[*types.RawUnitInfo]{},
		javaAction:      map[string]memoResult[*javaactions.JavaAction]{},
		jsAction:        map[string]memoResult[*types.JavaScriptAction]{},
		isRule:          map[string]memoResult[bool]{},
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

// ListMicroflowsNamed and ListMicroflowHeaders keep the real backend's
// microflowLookup visible through the memo. The memo embeds the FullBackend
// interface, so without them a type assertion on it fails and microflowsNamed
// falls back to ListMicroflows: describing a microflow rebuilds it to derive
// its layout, and that decoded every microflow of the project once per
// described flow — the bulk of a module dump's time (mendixlabs/mxcli#1624).
func (b *layoutCheckBackend) ListMicroflowsNamed(name string) ([]*microflows.Microflow, error) {
	return memoKeyed(b.microflowsNamed, name, func() ([]*microflows.Microflow, error) {
		if l, ok := b.FullBackend.(microflowLookup); ok {
			return l.ListMicroflowsNamed(name)
		}
		all, err := b.ListMicroflows()
		if err != nil {
			return nil, err
		}
		var out []*microflows.Microflow
		for _, mf := range all {
			if mf.Name == name {
				out = append(out, mf)
			}
		}
		return out, nil
	})
}

func (b *layoutCheckBackend) ListMicroflowHeaders() ([]types.DocumentHeader, error) {
	return memoOnce(&b.microflowHeaders, func() ([]types.DocumentHeader, error) {
		if l, ok := b.FullBackend.(microflowLookup); ok {
			return l.ListMicroflowHeaders()
		}
		all, err := b.ListMicroflows()
		if err != nil {
			return nil, err
		}
		out := make([]types.DocumentHeader, len(all))
		for i, mf := range all {
			out[i] = types.DocumentHeader{ID: mf.ID, ContainerID: mf.ContainerID, Name: mf.Name, Excluded: mf.Excluded}
		}
		return out, nil
	})
}

// The header listings and GetSnippet keep the real backend's by-header
// lookups visible through the memo, for the same reason as the microflow ones
// above: without them a describe that rebuilds a nanoflow to derive its layout
// decoded every nanoflow of the project (#1850).
func (b *layoutCheckBackend) ListNanoflowHeaders() ([]types.DocumentHeader, error) {
	return memoOnce(&b.nanoflowHeaders, func() ([]types.DocumentHeader, error) { return nanoflowHeaders(b.FullBackend) })
}

func (b *layoutCheckBackend) ListPageHeaders() ([]types.DocumentHeader, error) {
	return memoOnce(&b.pageHeaders, func() ([]types.DocumentHeader, error) { return pageHeadersOf(b.FullBackend) })
}

func (b *layoutCheckBackend) ListSnippetHeaders() ([]types.DocumentHeader, error) {
	return memoOnce(&b.snippetHeaders, func() ([]types.DocumentHeader, error) { return snippetHeaders(b.FullBackend) })
}

func (b *layoutCheckBackend) ListLayoutHeaders() ([]types.DocumentHeader, error) {
	return memoOnce(&b.layoutHeaders, func() ([]types.DocumentHeader, error) { return layoutHeaders(b.FullBackend) })
}

func (b *layoutCheckBackend) GetSnippet(id model.ID) (*pages.Snippet, error) {
	if l, ok := b.FullBackend.(snippetLookup); ok {
		return l.GetSnippet(id)
	}
	all, err := b.ListSnippets()
	if err != nil {
		return nil, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return nil, nil
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
