// SPDX-License-Identifier: Apache-2.0

package types

import "github.com/mendixlabs/mxcli/model"

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
	Icon     string         `json:"icon,omitempty"`
	IconType string         `json:"iconType,omitempty"`
	Items    []*NavMenuItem `json:"items,omitempty"`
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
	// Icon is a qualified icon-collection name (Atlas_Core.Atlas.home). Empty
	// means no icon, which serializes as a null Icon.
	Icon  string
	Items []NavMenuItemSpec
}
