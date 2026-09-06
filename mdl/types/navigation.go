// SPDX-License-Identifier: Apache-2.0

package types

import (
	"strings"

	"github.com/mendixlabs/mxcli/model"
)

// NavigationDocument represents a parsed navigation document.
type NavigationDocument struct {
	model.BaseElement
	ContainerID model.ID             `json:"containerId"`
	Name        string               `json:"name"`
	Profiles    []*NavigationProfile `json:"profiles,omitempty"`
}

// GetName returns the navigation document's name.
func (nd *NavigationDocument) GetName() string { return nd.Name }

// GetContainerID returns the container ID.
func (nd *NavigationDocument) GetContainerID() model.ID { return nd.ContainerID }

// NavigationProfile represents a single navigation profile.
type NavigationProfile struct {
	Name               string              `json:"name"`
	Kind               string              `json:"kind"`
	IsNative           bool                `json:"isNative"`
	HomePage           *NavHomePage        `json:"homePage,omitempty"`
	RoleBasedHomePages []*NavRoleBasedHome `json:"roleBasedHomePages,omitempty"`
	LoginPage          string              `json:"loginPage,omitempty"`
	NotFoundPage       string              `json:"notFoundPage,omitempty"`
	MenuItems          []*NavMenuItem      `json:"menuItems,omitempty"`
	OfflineEntities    []*NavOfflineEntity `json:"offlineEntities,omitempty"`
}

// NavHomePage holds a profile's default home page.
type NavHomePage struct {
	Page      string `json:"page,omitempty"`
	Microflow string `json:"microflow,omitempty"`
}

// NavRoleBasedHome maps a user role to a home page.
type NavRoleBasedHome struct {
	UserRole  string `json:"userRole"`
	Page      string `json:"page,omitempty"`
	Microflow string `json:"microflow,omitempty"`
}

// NavMenuItem is a recursive navigation menu entry.
type NavMenuItem struct {
	Caption    string `json:"caption"`
	Page       string `json:"page,omitempty"`
	Microflow  string `json:"microflow,omitempty"`
	ActionType string `json:"actionType,omitempty"`
	// Icon is the qualified name an icon-collection or image icon points at.
	// Empty for no icon and for a glyph icon, which carries a numeric Code
	// instead. IconType keeps the storage $Type so a reader can tell the three
	// apart — DESCRIBE only round-trips Forms$IconCollectionIcon.
	Icon     string `json:"icon,omitempty"`
	IconType string `json:"iconType,omitempty"`
	// IconCode is Forms$GlyphIcon's numeric Code — the ONLY thing that
	// identifies a glyph icon, since it carries no qualified name. Without it a
	// reader knows a glyph was there but not which one, so it can neither be
	// re-emitted by DESCRIBE nor carried through a rewrite.
	IconCode int            `json:"iconCode,omitempty"`
	Items    []*NavMenuItem `json:"items,omitempty"`
}

// HasIcon reports whether the item carries an icon of ANY of the three kinds.
//
// Not `Icon != ""`: a glyph icon has a numeric code and no name, so the obvious
// test calls an item with a perfectly good icon iconless. MDL077 asks this
// question, and asking it the obvious way would have made the rule fire on every
// Studio Pro-authored menu.
func (m *NavMenuItem) HasIcon() bool {
	if m == nil {
		return false
	}
	switch MenuIconKindOf(m.IconType) {
	case MenuIconNone:
		return false
	case MenuIconGlyph:
		return true
	default:
		// A collection or image icon without a name is a malformed element, not
		// an icon anyone can see.
		return m.Icon != ""
	}
}

// MenuIconKind names which of Mendix's three icon elements a menu item carries.
//
// They are not variations on one shape: an icon-collection icon and an image
// icon each hold a qualified name (into an icon collection and an image
// collection respectively — different documents), while a glyph icon holds a
// numeric character code and no name at all. Treating them as one "icon string"
// is what made a rewrite silently convert a glyph into nothing.
type MenuIconKind string

const (
	MenuIconNone       MenuIconKind = ""
	MenuIconCollection MenuIconKind = "collection"
	MenuIconGlyph      MenuIconKind = "glyph"
	MenuIconImage      MenuIconKind = "image"
	// MenuIconUnknown is a stored $Type this build does not know. It is
	// deliberately NOT MenuIconNone: reporting an unrecognised element as "no
	// icon" is how a future fourth variant would get silently dropped by a
	// rewrite, which is the bug this vocabulary exists to prevent.
	MenuIconUnknown MenuIconKind = "unknown"
)

// MenuIconKindOf maps a stored $Type onto the vocabulary.
//
// Matched on the suffix because the same element has two spellings: the
// metamodel calls it Pages$IconCollectionIcon and storage calls it
// Forms$IconCollectionIcon ("Form" was the original term for "Page"). A reader
// handing over either name must land on the same kind.
func MenuIconKindOf(iconType string) MenuIconKind {
	switch {
	case iconType == "":
		return MenuIconNone
	case strings.HasSuffix(iconType, "IconCollectionIcon"):
		return MenuIconCollection
	case strings.HasSuffix(iconType, "GlyphIcon"):
		return MenuIconGlyph
	case strings.HasSuffix(iconType, "ImageIcon"):
		return MenuIconImage
	}
	return MenuIconUnknown
}

// MenuIconStorageType is the inverse: the $Type a writer must emit for a kind.
// Empty for MenuIconNone (no Icon element at all) and for MenuIconUnknown,
// which a writer must never invent a name for.
func MenuIconStorageType(kind MenuIconKind) string {
	switch kind {
	case MenuIconCollection:
		return "Forms$IconCollectionIcon"
	case MenuIconGlyph:
		return "Forms$GlyphIcon"
	case MenuIconImage:
		return "Forms$ImageIcon"
	}
	return ""
}

// MenuDocument is a standalone `Menus$MenuDocument` — a reusable menu that menu
// widgets point at, stored as its own document rather than inside a navigation
// profile. Atlas_Core ships two of them (Phone_Menu, Tablet_Menu).
//
// Its entries are the same `Menus$MenuItem` elements a navigation profile holds,
// so they are modelled as NavMenuItem rather than a parallel type — one item
// shape, one parser, one renderer.
type MenuDocument struct {
	ID            model.ID       `json:"id"`
	ContainerID   model.ID       `json:"containerId"`
	Name          string         `json:"name"`
	Documentation string         `json:"documentation,omitempty"`
	ExportLevel   string         `json:"exportLevel,omitempty"`
	Excluded      bool           `json:"excluded,omitempty"`
	Items         []*NavMenuItem `json:"items,omitempty"`
}

// GetName returns the menu document's name.
func (m *MenuDocument) GetName() string { return m.Name }

// NavOfflineEntity declares offline sync rules for an entity.
type NavOfflineEntity struct {
	Entity     string `json:"entity"`
	SyncMode   string `json:"syncMode"`
	Constraint string `json:"constraint,omitempty"`
}

// NavigationProfileSpec specifies changes to a navigation profile.
type NavigationProfileSpec struct {
	HomePages    []NavHomePageSpec
	LoginPage    string
	NotFoundPage string
	MenuItems    []NavMenuItemSpec
	HasMenu      bool
}

// NavHomePageSpec specifies a home page assignment.
type NavHomePageSpec struct {
	IsPage  bool
	Target  string
	ForRole string
}

// NavMenuItemSpec specifies a menu item (recursive).
type NavMenuItemSpec struct {
	Caption   string
	Page      string
	Microflow string
	// SignOut is the third action a menu item can carry. Studio Pro stores it
	// as the same Forms$SignOutClientAction a button uses, so it needs no
	// target — which is why it is a flag rather than another name field.
	SignOut bool
	// Icon is the qualified name for a collection or image icon
	// (Atlas_Core.Atlas.home). Empty with IconKind None means no icon, which
	// serializes as a null Icon.
	Icon string
	// IconKind selects which of the three icon elements to write. The spec used
	// to carry a name and nothing else, so every icon became a
	// Forms$IconCollectionIcon and a glyph turned into nothing on rewrite.
	IconKind MenuIconKind
	// IconCode is the glyph's numeric Code, meaningful only for MenuIconGlyph.
	IconCode int
	Items    []NavMenuItemSpec
}
