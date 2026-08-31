// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"fmt"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"

	"go.mongodb.org/mongo-driver/bson"
)

// NavigationProfileSpec describes the desired state for a navigation profile.
// Aliased from mdl/types to avoid duplicate definitions.
type NavigationProfileSpec = types.NavigationProfileSpec

// NavHomePageSpec describes a home page entry.
type NavHomePageSpec = types.NavHomePageSpec

// NavMenuItemSpec describes a menu item.
type NavMenuItemSpec = types.NavMenuItemSpec

// UpdateNavigationProfile patches a navigation profile's home pages, login page, and menu.
func (w *Writer) UpdateNavigationProfile(navDocID model.ID, profileName string, spec NavigationProfileSpec) error {
	return w.readPatchWrite(navDocID, func(doc bson.D) (bson.D, error) {
		profiles := getBsonArray(doc, "Profiles")
		if profiles == nil {
			return doc, fmt.Errorf("no Profiles array found in navigation document")
		}

		found := false
		for i, item := range profiles {
			profDoc, ok := item.(bson.D)
			if !ok {
				continue
			}

			// Match profile by name (case-insensitive)
			name := ""
			for _, f := range profDoc {
				if f.Key == "Name" {
					name, _ = f.Value.(string)
					break
				}
			}
			if !strings.EqualFold(name, profileName) {
				continue
			}
			found = true

			// Determine if this is a native profile
			isNative := false
			for _, f := range profDoc {
				if f.Key == "$Type" {
					typeName, _ := f.Value.(string)
					isNative = typeName == "Navigation$NativeNavigationProfile"
					break
				}
			}

			if isNative {
				profDoc = patchNativeProfile(profDoc, spec)
			} else {
				profDoc = patchWebProfile(profDoc, spec)
			}

			profiles[i] = profDoc
			break
		}

		if !found {
			return doc, fmt.Errorf("navigation profile not found: %s", profileName)
		}

		return setBsonField(doc, "Profiles", profiles), nil
	})
}

// patchWebProfile applies the spec to a web navigation profile.
func patchWebProfile(doc bson.D, spec NavigationProfileSpec) bson.D {
	// --- HomePage (default home) ---
	var defaultHome *NavHomePageSpec
	var roleHomes []NavHomePageSpec
	for _, hp := range spec.HomePages {
		if hp.ForRole == "" {
			h := hp
			defaultHome = &h
		} else {
			roleHomes = append(roleHomes, hp)
		}
	}

	if defaultHome != nil {
		doc = setBsonField(doc, "HomePage", buildHomePageBson(defaultHome))
	} else {
		// Clear default home page
		doc = setBsonField(doc, "HomePage", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Navigation$HomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: ""},
		})
	}

	// --- HomeItems (role-based homes) ---
	homeItems := bson.A{int32(1)}
	for _, rh := range roleHomes {
		homeItems = append(homeItems, buildRoleBasedHomeBson(rh))
	}
	doc = setBsonField(doc, "HomeItems", homeItems)

	// --- LoginPageSettings ---
	if spec.LoginPage != "" {
		doc = setBsonField(doc, "LoginPageSettings", buildFormSettingsBson(spec.LoginPage))
	} else {
		doc = setBsonField(doc, "LoginPageSettings", buildFormSettingsBson(""))
	}

	// --- NotFoundHomepage ---
	if spec.NotFoundPage != "" {
		doc = setBsonField(doc, "NotFoundHomepage", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			// Studio Pro's "Fallback page". The $Type is
			// Navigation$NotFoundHomePage, not the Navigation$HomePage the home
			// page slot takes -- measured on ako/TestApp, whose fallback page
			// Studio Pro stored as Navigation$NotFoundHomePage/Page. mxbuild
			// accepts either, so nothing caught this.
			{Key: "$Type", Value: "Navigation$NotFoundHomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: spec.NotFoundPage},
		})
	} else {
		// Mendix uses null when not set
		doc = setBsonField(doc, "NotFoundHomepage", nil)
	}

	// --- Menu ---
	if spec.HasMenu {
		menuItems := bson.A{int32(1)}
		for _, mi := range spec.MenuItems {
			menuItems = append(menuItems, buildMenuItemBson(mi))
		}
		doc = setBsonField(doc, "Menu", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			{Key: "Items", Value: menuItems},
		})
	}

	return doc
}

// patchNativeProfile applies the spec to a native navigation profile.
func patchNativeProfile(doc bson.D, spec NavigationProfileSpec) bson.D {
	var defaultHome *NavHomePageSpec
	var roleHomes []NavHomePageSpec
	for _, hp := range spec.HomePages {
		if hp.ForRole == "" {
			h := hp
			defaultHome = &h
		} else {
			roleHomes = append(roleHomes, hp)
		}
	}

	if defaultHome != nil {
		page := ""
		nanoflow := ""
		if defaultHome.IsPage {
			page = defaultHome.Target
		} else {
			nanoflow = defaultHome.Target
		}
		doc = setBsonField(doc, "NativeHomePage", bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Navigation$NativeHomePage"},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nanoflow},
		})
	}

	// Role-based native home pages
	roleItems := bson.A{int32(1)}
	for _, rh := range roleHomes {
		page := ""
		nanoflow := ""
		if rh.IsPage {
			page = rh.Target
		} else {
			nanoflow = rh.Target
		}
		roleItems = append(roleItems, bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Navigation$RoleBasedNativeHomePage"},
			{Key: "UserRole", Value: rh.ForRole},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nanoflow},
		})
	}
	doc = setBsonField(doc, "RoleBasedNativeHomePages", roleItems)

	return doc
}

// buildHomePageBson builds a Navigation$HomePage BSON document.
func buildHomePageBson(hp *NavHomePageSpec) bson.D {
	page := ""
	mf := ""
	if hp.IsPage {
		page = hp.Target
	} else {
		mf = hp.Target
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Navigation$HomePage"},
		{Key: "Microflow", Value: mf},
		{Key: "Page", Value: page},
	}
}

// buildRoleBasedHomeBson builds a Navigation$RoleBasedHomePage BSON document.
func buildRoleBasedHomeBson(rh NavHomePageSpec) bson.D {
	page := ""
	mf := ""
	if rh.IsPage {
		page = rh.Target
	} else {
		mf = rh.Target
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Navigation$RoleBasedHomePage"},
		{Key: "Microflow", Value: mf},
		{Key: "Page", Value: page},
		{Key: "UserRole", Value: rh.ForRole},
	}
}

// buildFormSettingsBson builds a Forms$FormSettings BSON document with required fields.
func buildFormSettingsBson(formName string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$FormSettings"},
		{Key: "Form", Value: formName},
		{Key: "ParameterMappings", Value: bson.A{int32(1)}},
		// No override is an explicit null. An empty template overrides the page
		// title with "" and produces CW0263 for every authored menu item (#812).
		{Key: "TitleOverride", Value: nil},
	}
}

// buildMenuItemBson builds a Menus$MenuItem BSON document recursively.
func buildMenuItemBson(mi NavMenuItemSpec) bson.D {
	item := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: buildMenuAction(mi)},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: buildCaptionBson(mi.Caption)},
		{Key: "Icon", Value: buildMenuIconBson(mi.Icon)},
	}

	// Sub-items
	subItems := bson.A{int32(1)}
	for _, sub := range mi.Items {
		subItems = append(subItems, buildMenuItemBson(sub))
	}
	item = append(item, bson.E{Key: "Items", Value: subItems})

	return item
}

// buildMenuIconBson builds a menu item's Icon, or nil when none is set.
//
// The metamodel calls this Pages$IconCollectionIcon, but the storage name is
// Forms$IconCollectionIcon — the same "Form was the original term for Page"
// rename CLAUDE.md documents for ShowFormAction. Verified against a Studio
// Pro-authored navigation document (ako/mxcli-ledger), whose menu icons are all
// Forms$IconCollectionIcon{Image: "Atlas_Core.Atlas.align-center"}, and matching
// the widget icon path already proven in issue #602.
//
// Two sibling variants exist in the same document — Forms$GlyphIcon{Code: int}
// and Forms$ImageIcon{Image: QN} — and are deliberately NOT emitted here. Both
// carry a different payload shape, and ImageIcon's qualified name is
// indistinguishable from an IconCollectionIcon's without resolving which
// collection document it lands in. Guessing between polymorphic variants is the
// failure mode that produces a document mxbuild accepts and Studio Pro cannot
// open.
func buildMenuIconBson(icon string) interface{} {
	if icon == "" {
		return nil
	}
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$IconCollectionIcon"},
		{Key: "Image", Value: icon},
	}
}

// buildCaptionBson builds a Texts$Text BSON document with a single en_US translation.
func buildCaptionBson(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{
			int32(1),
			bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: model.AuthoringLanguage()},
				{Key: "Text", Value: text},
			},
		}},
	}
}

// buildMenuAction builds the Action BSON for a menu item based on its target.
func buildMenuAction(mi NavMenuItemSpec) bson.D {
	if mi.Page != "" {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$FormAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "FormSettings", Value: buildFormSettingsBson(mi.Page)},
			{Key: "NumberOfPagesToClose2", Value: ""},
			{Key: "PagesForSpecializations", Value: bson.A{int32(1)}},
		}
	}
	if mi.Microflow != "" {
		return bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Forms$MicroflowAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: idToBsonBinary(generateUUID())},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: mi.Microflow},
			}},
		}
	}
	// No action (sub-menu container or plain item)
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Forms$NoAction"},
	}
}
