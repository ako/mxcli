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
// Typed-array markers for the lists these writers emit.
//
// The leading int32 of a Mendix array is a per-FIELD constant, not a function of
// the list's contents: Forms$FormSettings.ParameterMappings is 2 in 816 empty
// and 306 non-empty real occurrences alike. So each list below takes the value
// Studio Pro writes for that field, censused over 19,078 unit files in 54
// projects on this machine:
//
//	Forms$FormSettings.ParameterMappings        2  (1122 documents)
//	Forms$FormAction.PagesForSpecializations    2  (357)
//	Menus$MenuItemCollection.Items              3  (153)
//	Menus$MenuItem.Items                        3  (459)
//	Texts$Text.Items                            3  (169,486 vs 7 at 2)
//	Navigation$NavigationProfile.HomeItems      2  (51)
//
// These writers previously emitted 1 for all of them. Note what that was NOT:
// 1 is a perfectly legitimate Mendix marker -- a Marketplace .mpk mxcli has
// never touched carries it on CustomWidgets$WidgetValueType.AllowedTypes (212k
// occurrences) and on Forms$Page.AllowedModuleRoles. debug-bson.md's rule that
// "any other value is invalid and Studio Pro ignores the array" is too strong
// and is corrected there. The defect is narrower: for THESE fields, no
// Studio Pro document uses 1, and mxcli's own menu-document codec path already
// writes 3 for the same Menus$ item collections, so the two paths disagreed.
//
// HomeItems needed a second source, because all 51 census observations are empty
// lists and navigation_profile_add.go wrote 3 there from a PED session that
// cannot be re-run here. ako/TestApp settles it: its Studio Pro-authored profile
// carries HomeItems [marker 2] holding two Navigation$RoleBasedHomePage
// elements -- a NON-empty list, which is the case the census could not reach.
// navigation_profile_add.go now writes 2 as well.
const (
	navMarkerItems             = int32(3)
	navMarkerParameterMappings = int32(2)
	navMarkerHomeItems         = int32(2)
)

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
	homeItems := bson.A{navMarkerHomeItems}
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
			// Studio Pro stored as Navigation$NotFoundHomePage/Page.
			//
			// The wrong $Type here is not cosmetic: Mendix cannot LOAD the
			// project. Both `mx check` and `mxbuild --target=deploy` exit 1 with
			// "Object of type '...Navigation.HomePage' cannot be converted to
			// type '...Navigation.NotFoundHomePage'" (measured on 11.13, against
			// a build of this file emitting the old spelling). Nothing caught it
			// because nothing ever BUILT a project with a fallback page set --
			// the automated mx-check coverage runs doctype-tests/ only, and no
			// script there sets one.
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
		menuItems := bson.A{navMarkerItems}
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
	roleItems := bson.A{navMarkerHomeItems}
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
		{Key: "ParameterMappings", Value: bson.A{navMarkerParameterMappings}},
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
		{Key: "Icon", Value: buildMenuIconBson(mi)},
	}

	// Sub-items
	subItems := bson.A{navMarkerItems}
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
// and Forms$ImageIcon{Image: QN}. They used to be excluded because a name alone
// cannot tell an image icon from a collection icon without resolving which
// document it lands in, and guessing between polymorphic variants is the failure
// mode that produces a document mxbuild accepts and Studio Pro cannot open.
//
// Nothing is guessed now: the KIND is carried explicitly, from the author's own
// `icon image …` / `icon glyph …` or from the kind the reader saw in storage. So
// all three are emitted, and the branch is a dispatch rather than an inference.
//
// Excluding them was not neutral. `create or replace navigation` is a full
// replacement, so an icon the writer would not emit was an icon the statement
// DELETED — measured on testdata/expr-checker, exec of DESCRIBE's own output
// destroyed a glyph icon at exit 0.
func buildMenuIconBson(spec NavMenuItemSpec) interface{} {
	kind := spec.IconKind
	if kind == types.MenuIconNone && spec.Icon != "" {
		// A spec built before the kind existed carries a name and nothing else,
		// and that name has only ever meant an icon-collection icon.
		kind = types.MenuIconCollection
	}
	storage := types.MenuIconStorageType(kind)
	if storage == "" {
		return nil
	}
	doc := bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: storage},
	}
	if kind == types.MenuIconGlyph {
		// A glyph with no code identifies no glyph. Emit no icon rather than an
		// element nobody can see.
		if spec.IconCode == 0 {
			return nil
		}
		return append(doc, bson.E{Key: "Code", Value: int32(spec.IconCode)})
	}
	if spec.Icon == "" {
		return nil
	}
	return append(doc, bson.E{Key: "Image", Value: spec.Icon})
}

// buildCaptionBson builds a Texts$Text BSON document with a single en_US translation.
func buildCaptionBson(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(generateUUID())},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{
			navMarkerItems,
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
			{Key: "PagesForSpecializations", Value: bson.A{navMarkerParameterMappings}},
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
