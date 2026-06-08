// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// TestSessionModuleResolution verifies that a module registered this session is
// resolvable by ID and name (so a later "create enumeration NewMod.X" finds it),
// without needing the local reader.
func TestSessionModuleResolution(t *testing.T) {
	b := &Backend{}
	mod := &model.Module{Name: "NewMod"}
	mod.ID = model.ID("mcp~module~NewMod")
	b.sessionModules = append(b.sessionModules, mod)

	got, err := b.GetModuleByName("NewMod")
	if err != nil || got.Name != "NewMod" {
		t.Fatalf("GetModuleByName(NewMod) = %+v / %v", got, err)
	}
	got, err = b.GetModule("mcp~module~NewMod")
	if err != nil || got.ID != "mcp~module~NewMod" {
		t.Fatalf("GetModule(by id) = %+v / %v", got, err)
	}
}
