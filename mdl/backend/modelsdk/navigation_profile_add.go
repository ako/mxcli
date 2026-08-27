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
// The shape is pinned against Studio Pro 11.14 over PED, and the measurement that
// matters was taken by adding a profile through Studio Pro's OWN model API and
// reading back what it materialised, rather than by copying a profile the user
// had already configured:
//
//	                            a profile Studio Pro has just created
//	ProgressiveWebAppSettings   null
//	OfflineEntityConfigs        []
//	ThrowPartialSyncError       true
//	AppIcon                     ""
//
// That distinction is the whole point. The reference project's Phone and
// TabletOffline profiles both carry {Precaching: true, InstallPrompt: true} — but
// the schema's default for Precaching is FALSE and ProgressiveWebAppSettings is
// optional, so those values are the user's choices, not Studio Pro's. Copying
// them would have shipped every Phone profile with precaching silently on.
//
// Every kind stores the same fourteen keys; only Kind and Name differ. An offline
// profile is not a different document — what changes is what Mendix then demands
// of the pages it reaches (CE6206), which is why creating one runs MDL-OFFLINE01
// over the project instead of just writing the profile.
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
		// Optional in the schema, and null on every profile Studio Pro has just
		// created — including the offline kinds, which is the opposite of what the
		// reference project suggested. Turning a PWA on is a user decision.
		{Key: "ProgressiveWebAppSettings", Value: nil},
		{Key: "ThrowPartialSyncError", Value: true},
	}
}
