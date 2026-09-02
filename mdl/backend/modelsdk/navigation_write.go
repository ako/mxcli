// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/bsonutil"
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// UpdateNavigationProfile patches a navigation profile's home pages, login page,
// not-found page, and menu in place, preserving the rest of the navigation
// document byte-for-byte (read-unmarshal-patch-marshal). Mirrors the legacy
// writer field-for-field; pure bson.D manipulation, no codec rebuild.
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

func (b *Backend) UpdateNavigationProfile(navDocID model.ID, profileName string, spec types.NavigationProfileSpec) error {
	if b.writer == nil {
		return fmt.Errorf("UpdateNavigationProfile: not connected for writing")
	}
	raw, err := b.reader.GetRawUnitBytes(string(navDocID))
	if err != nil {
		return fmt.Errorf("UpdateNavigationProfile: load unit: %w", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("UpdateNavigationProfile: unmarshal: %w", err)
	}

	profiles := navGetArray(doc, "Profiles")
	if profiles == nil {
		return fmt.Errorf("no Profiles array found in navigation document")
	}
	found := false
	for i, item := range profiles {
		profDoc, ok := item.(bson.D)
		if !ok {
			continue // the leading int32 marker
		}
		if !strings.EqualFold(navGetString(profDoc, "Name"), profileName) {
			continue
		}
		found = true
		if navGetString(profDoc, "$Type") == "Navigation$NativeNavigationProfile" {
			profiles[i] = navPatchNativeProfile(profDoc, spec)
		} else {
			profiles[i] = navPatchWebProfile(profDoc, spec)
		}
		break
	}
	if !found {
		return fmt.Errorf("navigation profile not found: %s", profileName)
	}
	doc = navSetField(doc, "Profiles", profiles)

	out, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("UpdateNavigationProfile: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(navDocID), out)
}

// --- small bson.D helpers (pure) ---

func navGetArray(doc bson.D, key string) bson.A {
	for _, e := range doc {
		if e.Key == key {
			if a, ok := e.Value.(bson.A); ok {
				return a
			}
		}
	}
	return nil
}

func navSetField(doc bson.D, key string, value any) bson.D {
	for i := range doc {
		if doc[i].Key == key {
			doc[i].Value = value
			return doc
		}
	}
	return append(doc, bson.E{Key: key, Value: value})
}

func navGetString(doc bson.D, key string) string {
	for _, e := range doc {
		if e.Key == key {
			s, _ := e.Value.(string)
			return s
		}
	}
	return ""
}

func navID() any { return bsonutil.NewIDBsonBinary() }

// --- profile patchers ---

func navPatchWebProfile(doc bson.D, spec types.NavigationProfileSpec) bson.D {
	var defaultHome *types.NavHomePageSpec
	var roleHomes []types.NavHomePageSpec
	for _, hp := range spec.HomePages {
		if hp.ForRole == "" {
			h := hp
			defaultHome = &h
		} else {
			roleHomes = append(roleHomes, hp)
		}
	}

	if defaultHome != nil {
		doc = navSetField(doc, "HomePage", navHomePageBson(defaultHome.IsPage, defaultHome.Target, ""))
	} else {
		doc = navSetField(doc, "HomePage", navHomePageBson(false, "", ""))
	}

	homeItems := bson.A{navMarkerHomeItems}
	for _, rh := range roleHomes {
		homeItems = append(homeItems, navHomePageBson(rh.IsPage, rh.Target, rh.ForRole))
	}
	doc = navSetField(doc, "HomeItems", homeItems)

	doc = navSetField(doc, "LoginPageSettings", navFormSettingsBson(spec.LoginPage))

	if spec.NotFoundPage != "" {
		doc = navSetField(doc, "NotFoundHomepage", bson.D{
			{Key: "$ID", Value: navID()},
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
		doc = navSetField(doc, "NotFoundHomepage", nil)
	}

	if spec.HasMenu {
		menuItems := bson.A{navMarkerItems}
		for _, mi := range spec.MenuItems {
			menuItems = append(menuItems, navMenuItemBson(mi))
		}
		doc = navSetField(doc, "Menu", bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			{Key: "Items", Value: menuItems},
		})
	}
	return doc
}

func navPatchNativeProfile(doc bson.D, spec types.NavigationProfileSpec) bson.D {
	var defaultHome *types.NavHomePageSpec
	var roleHomes []types.NavHomePageSpec
	for _, hp := range spec.HomePages {
		if hp.ForRole == "" {
			h := hp
			defaultHome = &h
		} else {
			roleHomes = append(roleHomes, hp)
		}
	}

	if defaultHome != nil {
		page, nf := navSplitTarget(defaultHome.IsPage, defaultHome.Target)
		doc = navSetField(doc, "NativeHomePage", bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$NativeHomePage"},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nf},
		})
	}

	roleItems := bson.A{navMarkerHomeItems}
	for _, rh := range roleHomes {
		page, nf := navSplitTarget(rh.IsPage, rh.Target)
		roleItems = append(roleItems, bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$RoleBasedNativeHomePage"},
			{Key: "UserRole", Value: rh.ForRole},
			{Key: "HomePagePage", Value: page},
			{Key: "HomePageNanoflow", Value: nf},
		})
	}
	doc = navSetField(doc, "RoleBasedNativeHomePages", roleItems)
	return doc
}

func navSplitTarget(isPage bool, target string) (page, nanoflow string) {
	if isPage {
		return target, ""
	}
	return "", target
}

// navHomePageBson builds a Navigation$HomePage (default) or
// Navigation$RoleBasedHomePage (when forRole is set).
func navHomePageBson(isPage bool, target, forRole string) bson.D {
	page, mf := navSplitTarget(isPage, target)
	d := bson.D{{Key: "$ID", Value: navID()}}
	if forRole == "" {
		d = append(d, bson.E{Key: "$Type", Value: "Navigation$HomePage"})
		d = append(d, bson.E{Key: "Microflow", Value: mf}, bson.E{Key: "Page", Value: page})
		return d
	}
	d = append(d, bson.E{Key: "$Type", Value: "Navigation$RoleBasedHomePage"})
	d = append(d, bson.E{Key: "Microflow", Value: mf}, bson.E{Key: "Page", Value: page}, bson.E{Key: "UserRole", Value: forRole})
	return d
}

func navFormSettingsBson(formName string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Forms$FormSettings"},
		{Key: "Form", Value: formName},
		{Key: "ParameterMappings", Value: bson.A{navMarkerParameterMappings}},
		// No override is an explicit null. An empty template overrides the page
		// title with "" and produces CW0263 for every authored menu item (#812).
		{Key: "TitleOverride", Value: nil},
	}
}

func navMenuItemBson(mi types.NavMenuItemSpec) bson.D {
	item := bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Menus$MenuItem"},
		{Key: "Action", Value: navMenuAction(mi)},
		{Key: "AlternativeText", Value: nil},
		{Key: "Caption", Value: navCaptionBson(mi.Caption)},
		{Key: "Icon", Value: navMenuIconBson(mi.Icon)},
	}
	subItems := bson.A{navMarkerItems}
	for _, sub := range mi.Items {
		subItems = append(subItems, navMenuItemBson(sub))
	}
	item = append(item, bson.E{Key: "Items", Value: subItems})
	return item
}

// navMenuIconBson mirrors sdk/mpr's buildMenuIconBson: the storage name is
// Forms$IconCollectionIcon (not the metamodel's Pages$…), and only that variant
// is emitted. See the comment there for why GlyphIcon/ImageIcon are excluded.
func navMenuIconBson(icon string) interface{} {
	if icon == "" {
		return nil
	}
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Forms$IconCollectionIcon"},
		{Key: "Image", Value: icon},
	}
}

func navCaptionBson(text string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Texts$Text"},
		{Key: "Items", Value: bson.A{
			navMarkerItems,
			bson.D{
				{Key: "$ID", Value: navID()},
				{Key: "$Type", Value: "Texts$Translation"},
				{Key: "LanguageCode", Value: model.AuthoringLanguage()},
				{Key: "Text", Value: text},
			},
		}},
	}
}

func navMenuAction(mi types.NavMenuItemSpec) bson.D {
	if mi.Page != "" {
		return bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Forms$FormAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "FormSettings", Value: navFormSettingsBson(mi.Page)},
			{Key: "NumberOfPagesToClose2", Value: ""},
			{Key: "PagesForSpecializations", Value: bson.A{navMarkerParameterMappings}},
		}
	}
	if mi.Microflow != "" {
		return bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Forms$MicroflowAction"},
			{Key: "DisabledDuringExecution", Value: false},
			{Key: "MicroflowSettings", Value: bson.D{
				{Key: "$ID", Value: navID()},
				{Key: "$Type", Value: "Forms$MicroflowSettings"},
				{Key: "Microflow", Value: mi.Microflow},
			}},
		}
	}
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Forms$NoAction"},
	}
}
