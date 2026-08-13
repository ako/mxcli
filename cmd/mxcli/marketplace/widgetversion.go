// SPDX-License-Identifier: Apache-2.0

package marketplace

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
)

// A module package is not only its model: it bundles copies of every widget its
// pages use. Those copies are pinned to whatever the module's author had at
// release time, and different modules pin different versions of the *same*
// widget. Measured on the 11.13 marketplace: Atlas_Web_Content 4.3.0 ships five
// Data Widgets at 3.4.0, which DataWidgets 3.11.3 ships at 3.11.3.
//
// So installing modules in one order and then another silently downgrades
// widgets — the project keeps whichever module was installed last. Nothing
// reports it: an older widget is not a `mx check` error, so the app just runs
// yesterday's widget code (mxcli-chat FINDINGS §14, where five Data Widgets and
// Charts were rolled back by a later module update).
//
// The rule below is therefore: a bundled widget never replaces a newer copy the
// project already has. Skipped replacements are reported, not silent — the
// caller has to be able to see that the package wanted a different version.

// clientModuleVersion is the version a widget package declares for itself, in
// the `<clientModule version="...">` element of its package.xml. The `<package
// version="1.0">` root attribute is the *manifest schema* version and is 1.0 for
// every widget ever published — reading that one instead makes every comparison
// come out equal, which is exactly as broken as not comparing at all.
func clientModuleVersion(packageXML []byte) string {
	dec := xml.NewDecoder(bytes.NewReader(packageXML))
	for {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "clientModule" {
			continue
		}
		for _, a := range start.Attr {
			if a.Name.Local == "version" {
				return a.Value
			}
		}
		return ""
	}
}

// widgetVersionInMpk reads the declared version out of a widget .mpk given its
// bytes. Returns "" when the file is not a readable widget package, which the
// caller treats as "cannot compare" rather than as "older".
func widgetVersionInMpk(mpk []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(mpk), int64(len(mpk)))
	if err != nil {
		return ""
	}
	for _, f := range zr.File {
		if f.Name != "package.xml" {
			continue
		}
		rc, oerr := f.Open()
		if oerr != nil {
			return ""
		}
		body, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil {
			return ""
		}
		return clientModuleVersion(body)
	}
	return ""
}

// widgetVersionOnDisk is widgetVersionInMpk for a path, returning "" when the
// file is absent or unreadable.
func widgetVersionOnDisk(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return widgetVersionInMpk(body)
}

// versionLess reports whether a is an earlier version than b, comparing dotted
// numeric components. It returns false when either side cannot be parsed, so an
// unparseable version never causes a skip: the caller's default is to install,
// and a wrong "older" verdict would silently withhold the file the package
// shipped.
func versionLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	if len(as) == 0 || len(bs) == 0 {
		return false
	}
	n := max(len(as), len(bs))
	for i := range n {
		ai, bi := 0, 0
		if i < len(as) {
			v, err := strconv.Atoi(strings.TrimSpace(as[i]))
			if err != nil {
				return false
			}
			ai = v
		}
		if i < len(bs) {
			v, err := strconv.Atoi(strings.TrimSpace(bs[i]))
			if err != nil {
				return false
			}
			bi = v
		}
		if ai != bi {
			return ai < bi
		}
	}
	return false
}

// isWidgetPackage reports whether a package entry is a widget .mpk under
// widgets/.
func isWidgetPackage(name string) bool {
	return strings.HasPrefix(name, "widgets/") &&
		strings.EqualFold(path.Ext(name), ".mpk") &&
		!strings.Contains(strings.TrimPrefix(name, "widgets/"), "/")
}

// unpackedTwinOf returns the widgets/<Name>/ prefix that duplicates a
// widgets/<Name>.mpk entry, or "" when name is not such an entry.
//
// Some packages ship a widget both ways. FeedbackModule 5.0.0 carries
// `widgets/SprintrFeedbackWidget.mpk` *and* an unpacked
// `widgets/SprintrFeedbackWidget/` tree of the same widget at the same version
// (12.0.4, measured). Installing both leaves a duplicate in the project that
// `mx check` tolerates and nobody asked for (FINDINGS §18).
func unpackedTwinOf(name string) string {
	if !isWidgetPackage(name) {
		return ""
	}
	return strings.TrimSuffix(name, path.Ext(name)) + "/"
}
