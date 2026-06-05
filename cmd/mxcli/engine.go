// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/backend"
	mcpbackend "github.com/mendixlabs/mxcli/mdl/backend/mcp"
	mprbackend "github.com/mendixlabs/mxcli/mdl/backend/mpr"
)

// engineKind selects which model engine backs the local FullBackend.
//
// This is the single selection seam for the engine swap described in
// docs/plans/2026-06-05-adopt-modelsdk-engine.md. Today only "legacy" is wired;
// "modelsdk" and "compare" are recognised so the contract is stable, and fail
// fast with a pointer to the plan rather than silently falling back.
type engineKind string

const (
	engineLegacy   engineKind = "legacy"   // sdk/mpr write path (default)
	engineModelSDK engineKind = "modelsdk" // roundtrip codec engine (Phase 2+)
	engineCompare  engineKind = "compare"  // run both engines, diff BSON (Phase 2+)
)

// globalEngineFlag holds the value of the hidden --engine flag; it overrides
// the MXCLI_ENGINE environment variable. Set in PersistentPreRun.
var globalEngineFlag string

// resolveEngine reads the active engine from --engine, then MXCLI_ENGINE,
// defaulting to legacy. An unrecognised value is fatal so typos are loud.
func resolveEngine() engineKind {
	v := strings.TrimSpace(globalEngineFlag)
	if v == "" {
		v = strings.TrimSpace(os.Getenv("MXCLI_ENGINE"))
	}
	switch engineKind(strings.ToLower(v)) {
	case "", engineLegacy:
		return engineLegacy
	case engineModelSDK:
		return engineModelSDK
	case engineCompare:
		return engineCompare
	default:
		fmt.Fprintf(os.Stderr, "mxcli: unknown MXCLI_ENGINE %q (expected: legacy, modelsdk, compare)\n", v)
		os.Exit(2)
		return engineLegacy // unreachable
	}
}

// newBackendFactory returns the FullBackend factory for the active engine.
//
// Phase 0: only legacy is wired. modelsdk/compare are recognised but not yet
// backed by an implementation, so they fail fast here (the factory is built once
// at executor setup, before any project work). Phase 2 adds the modelsdk branch.
func newBackendFactory() func() backend.FullBackend {
	// The MCP backend (live Studio Pro) is selected by --mcp / --mcp-dial,
	// independent of MXCLI_ENGINE: writes route to Studio Pro, reads come from -p.
	if globalMCPURL != "" {
		url, dial := globalMCPURL, globalMCPDial
		concordURL, concordDial := globalMCPConcord, globalMCPConcordDial
		saveOnExit, checkOnExit, runOnExit := globalMCPSave, globalMCPCheck, globalMCPRun
		return func() backend.FullBackend {
			b := mcpbackend.New(url, dial)
			if concordURL != "" || saveOnExit || checkOnExit || runOnExit {
				b = b.WithConcord(mcpbackend.ConcordConfig{
					URL: concordURL, Dial: concordDial,
					SaveOnExit: saveOnExit, CheckOnExit: checkOnExit, RunOnExit: runOnExit,
				})
			}
			return b
		}
	}
	switch resolveEngine() {
	case engineModelSDK:
		fmt.Fprintln(os.Stderr, "mxcli: engine 'modelsdk' is not yet wired (Phase 2 of "+
			"docs/plans/2026-06-05-adopt-modelsdk-engine.md). Only 'legacy' is available today.")
		os.Exit(2)
	case engineCompare:
		fmt.Fprintln(os.Stderr, "mxcli: engine 'compare' requires the modelsdk backend (Phase 2 of "+
			"docs/plans/2026-06-05-adopt-modelsdk-engine.md). Only 'legacy' is available today.")
		os.Exit(2)
	}
	return func() backend.FullBackend { return mprbackend.New() }
}
