// SPDX-License-Identifier: Apache-2.0

package pages

// imageReferenceProperties is the by-name image reference each legacy image
// viewer stores: the qualified name of an image-collection entry
// (Module.Collection.Image) as a plain string, "" when unset. These are the keys
// CREATE writes (StaticImage.ImageName, DynamicImage.DefaultImageName) and
// DESCRIBE reads; ALTER PAGE SET writes them through the same table
// (mendixlabs/mxcli#1464).
var imageReferenceProperties = map[string]string{
	"Forms$StaticImageViewer": "Image",
	"Pages$StaticImageViewer": "Image",
	"Forms$ImageViewer":       "DefaultImage",
	"Pages$ImageViewer":       "DefaultImage",
}

// ImageReferenceProperty returns the property under which a widget of the
// given storage type ($Type) stores its image-collection reference, or "" for a
// widget that stores none there. A pluggable widget (CustomWidgets$CustomWidget)
// returns "": its image is a template property, not a top-level key.
func ImageReferenceProperty(storageType string) string {
	return imageReferenceProperties[storageType]
}
