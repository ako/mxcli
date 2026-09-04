// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

func mod(name string, marketplace bool) *model.Module {
	m := &model.Module{Name: name, FromAppStore: marketplace}
	m.ID = model.ID("id-" + name)
	if marketplace {
		m.AppStoreGuid = "guid-" + name
	}
	return m
}

func layoutFixture() []*model.Module {
	return []*model.Module{
		mod("System", false),
		mod("Administration", true),
		mod("Atlas_Core", true),
		mod("CapTrack", false),
		mod("MyFirstModule", false),
	}
}

// With no --module, only the modules the project owns are touched. Rearranging
// a Marketplace module is work the next update throws away, and System is not
// the user's to arrange.
func TestLayoutTargets_SkipsMarketplaceAndSystemByDefault(t *testing.T) {
	defer func(prev bool) { layoutIncludeMarketplace = prev }(layoutIncludeMarketplace)
	layoutIncludeMarketplace = false

	got, err := layoutTargets(layoutFixture(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	want := "CapTrack,MyFirstModule"
	if strings.Join(names, ",") != want {
		t.Errorf("targets = %v, want %s", names, want)
	}
}

// Naming a Marketplace module explicitly is a mistake worth reporting, not
// something to silently drop — a silent skip reports success having done
// nothing, which is the failure mode this whole session kept finding.
func TestLayoutTargets_NamedMarketplaceModuleIsRefused(t *testing.T) {
	defer func(prev bool) { layoutIncludeMarketplace = prev }(layoutIncludeMarketplace)
	layoutIncludeMarketplace = false

	_, err := layoutTargets(layoutFixture(), map[string]string{"administration": "Administration"})
	if err == nil {
		t.Fatal("naming a Marketplace module was accepted")
	}
	if !strings.Contains(err.Error(), "--include-marketplace") {
		t.Errorf("the error should name the escape hatch: %v", err)
	}
}

// ...and the escape hatch works.
func TestLayoutTargets_IncludeMarketplaceOptsIn(t *testing.T) {
	defer func(prev bool) { layoutIncludeMarketplace = prev }(layoutIncludeMarketplace)
	layoutIncludeMarketplace = true

	got, err := layoutTargets(layoutFixture(), map[string]string{"administration": "Administration"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "Administration" {
		t.Errorf("targets = %v, want [Administration]", got)
	}
}

// A typo must fail loudly, quoting what was typed rather than a normalised form.
func TestLayoutTargets_UnknownModuleIsAnError(t *testing.T) {
	_, err := layoutTargets(layoutFixture(), map[string]string{"nope": "Nope"})
	if err == nil {
		t.Fatal("an unknown module was accepted")
	}
	if !strings.Contains(err.Error(), "Nope") {
		t.Errorf("the error should quote the name as typed, got: %v", err)
	}
}

// Module names resolve case-insensitively in Mendix, so --module captrack has to
// find CapTrack.
func TestLayoutTargets_MatchesCaseInsensitively(t *testing.T) {
	got, err := layoutTargets(layoutFixture(), map[string]string{"captrack": "captrack"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "CapTrack" {
		t.Errorf("targets = %v, want [CapTrack]", got)
	}
}

// System is refused by name for the same reason the Marketplace one is: a silent
// skip would look like success.
func TestLayoutTargets_SystemIsRefusedByName(t *testing.T) {
	_, err := layoutTargets(layoutFixture(), map[string]string{"system": "System"})
	if err == nil || !strings.Contains(err.Error(), "System") {
		t.Errorf("naming System should be refused, got: %v", err)
	}
}

// The order the modules are processed in must not depend on the order the
// backend happened to list them — the output is a report someone reads.
func TestLayoutTargets_IsSorted(t *testing.T) {
	shuffled := []*model.Module{
		mod("MyFirstModule", false),
		mod("CapTrack", false),
		mod("Zeta", false),
		mod("Alpha", false),
	}
	got, err := layoutTargets(shuffled, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var names []string
	for _, m := range got {
		names = append(names, m.Name)
	}
	if strings.Join(names, ",") != "Alpha,CapTrack,MyFirstModule,Zeta" {
		t.Errorf("targets = %v, want them sorted", names)
	}
}
