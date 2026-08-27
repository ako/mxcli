// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/model"
)

// AddNavigationProfile appends a new web navigation profile to the navigation
// document. It is the half `CREATE OR REPLACE NAVIGATION` was missing: the
// statement could only ever replace a profile that already existed, so a
// device-specific front end — the whole reason Phone and Tablet profiles exist —
// could not be built from MDL at all (ako/mxcli-maintenance §7).
//
// The shape is pinned against a Studio Pro 11.14 document carrying all three web
// profiles, read over PED. Every one of them stores the SAME fourteen keys; only
// three values differ, and the interesting one is not guessable:
//
//	                            Responsive   Phone                        Tablet
//	ProgressiveWebAppSettings   null         {Precaching, InstallPrompt}  null
//	AppIcon                     Atlas icon   ""                           ""
//	Name / Kind                 Responsive   Phone                        Tablet
//
// So a Phone profile is NOT a copy of the Responsive one with the name changed —
// deriving it from its sibling, which is how this file was first going to be
// written, would have produced a Phone profile with no PWA settings.
func (b *Backend) AddNavigationProfile(navDocID model.ID, name string) error {
	if b.writer == nil {
		return fmt.Errorf("AddNavigationProfile: not connected for writing")
	}
	kind, ok := types.CanonicalProfileKind(name)
	if !ok {
		return fmt.Errorf("cannot create navigation profile %q: Mendix's web profiles are %s", name, strings.Join(types.WebProfileKindNames(), ", "))
	}

	raw, err := b.reader.GetRawUnitBytes(string(navDocID))
	if err != nil {
		return fmt.Errorf("AddNavigationProfile: load unit: %w", err)
	}
	var doc bson.D
	if err := bson.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("AddNavigationProfile: unmarshal: %w", err)
	}

	profiles := navGetArray(doc, "Profiles")
	if profiles == nil {
		return fmt.Errorf("no Profiles array found in navigation document")
	}
	for _, item := range profiles {
		p, ok := item.(bson.D)
		if !ok {
			continue // the leading typed-array marker
		}
		if strings.EqualFold(navGetString(p, "Name"), kind) {
			return fmt.Errorf("navigation profile %s already exists", kind)
		}
	}

	doc = navSetField(doc, "Profiles", append(profiles, newWebProfileBson(kind)))
	out, err := bson.Marshal(doc)
	if err != nil {
		return fmt.Errorf("AddNavigationProfile: marshal: %w", err)
	}
	return b.writer.UpdateRawUnit(string(navDocID), out)
}

// newWebProfileBson builds an empty profile of the given kind, with every key a
// Studio Pro-authored one carries and nothing it does not.
//
// The empty sub-elements are written rather than omitted because that is what
// the reference document has: a profile Studio Pro has just added already holds
// an empty HomePage, AppTitle, LoginPageSettings and MenuItemCollection. The
// caller then fills them through the ordinary UpdateNavigationProfile path, so
// this function deliberately knows nothing about menus or home pages.
func newWebProfileBson(kind string) bson.D {
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Navigation$NavigationProfile"},
		{Key: "AppIcon", Value: ""},
		{Key: "AppTitle", Value: bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "HomeItems", Value: bson.A{int32(3)}},
		{Key: "HomePage", Value: bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Navigation$HomePage"},
			{Key: "Microflow", Value: ""},
			{Key: "Page", Value: ""},
		}},
		{Key: "Kind", Value: kind},
		{Key: "LoginPageSettings", Value: navFormSettingsBson("")},
		{Key: "Menu", Value: bson.D{
			{Key: "$ID", Value: navID()},
			{Key: "$Type", Value: "Menus$MenuItemCollection"},
			{Key: "Items", Value: bson.A{int32(1)}},
		}},
		{Key: "Name", Value: kind},
		{Key: "NotFoundHomepage", Value: nil},
		{Key: "OfflineEntityConfigs", Value: bson.A{int32(3)}},
		{Key: "ProgressiveWebAppSettings", Value: progressiveWebAppBson(kind)},
		{Key: "ThrowPartialSyncError", Value: true},
	}
}

// progressiveWebAppBson returns the PWA settings for a kind. Only Phone carries
// them; Responsive and Tablet store null. Measured on the reference document —
// this is the asymmetry that makes a profile something to build per kind rather
// than copy from a sibling.
func progressiveWebAppBson(kind string) any {
	if kind != "Phone" {
		return nil
	}
	return bson.D{
		{Key: "$ID", Value: navID()},
		{Key: "$Type", Value: "Navigation$ProgressiveWebAppSettings"},
		{Key: "InstallPrompt", Value: true},
		{Key: "Precaching", Value: true},
	}
}
