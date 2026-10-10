package version

import (
	"strconv"
	"strings"
)

type Version struct {
	Major, Minor, Patch int
}

func Parse(s string) Version {
	parts := strings.SplitN(s, ".", 4)
	var v Version
	if len(parts) > 0 {
		v.Major, _ = strconv.Atoi(parts[0])
	}
	if len(parts) > 1 {
		v.Minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		v.Patch, _ = strconv.Atoi(parts[2])
	}
	return v
}

func (a Version) Compare(b Version) int {
	if a.Major != b.Major {
		return cmp(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmp(a.Minor, b.Minor)
	}
	return cmp(a.Patch, b.Patch)
}

func (v Version) IsZero() bool { return v.Major == 0 && v.Minor == 0 && v.Patch == 0 }

func (v Version) String() string {
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

func cmp(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

type PropertyVersionInfo struct {
	Introduced string
	Deleted    string
	Required   bool
	Public     bool
}

func (p PropertyVersionInfo) IsAvailableIn(v Version) bool {
	if p.Deleted != "" && v.Compare(Parse(p.Deleted)) >= 0 {
		return false
	}
	if p.Introduced != "" && v.Compare(Parse(p.Introduced)) < 0 {
		return false
	}
	return true
}

type TypeVersionInfo struct {
	Properties map[string]PropertyVersionInfo
}

//go:generate go run gen_metamodel.go

// registry is the metamodel's property version data by storage $Type, merged
// from metamodelVersions (metamodel_versions_gen.go). A type that appears in
// several generated packages keeps the union of its properties.
var registry = func() map[string]TypeVersionInfo {
	out := map[string]TypeVersionInfo{}
	for _, infos := range metamodelVersions {
		for typ, info := range infos {
			cur, ok := out[typ]
			if !ok {
				cur = TypeVersionInfo{Properties: map[string]PropertyVersionInfo{}}
				out[typ] = cur
			}
			for k, p := range info.Properties {
				cur.Properties[k] = p
			}
		}
	}
	return out
}()

// MetamodelProperty returns the version data the metamodel has for a property,
// by storage $Type and the SDK property name (as the generated data keys it).
func MetamodelProperty(typeName, sdkName string) (PropertyVersionInfo, bool) {
	p, ok := registry[typeName].Properties[sdkName]
	return p, ok
}

// PropertyIntroduced returns the version a property was introduced in, looked
// up by its storage $Type and BSON key (the generated data keys a property by
// its SDK name, which is the BSON key with a lower-case first letter). ok is
// false when there is nothing to go on — no version data, a property bound
// under a different storage name, or one declared on a supertype.
func PropertyIntroduced(typeName, key string) (Version, bool) {
	if key == "" {
		return Version{}, false
	}
	p, ok := MetamodelProperty(typeName, strings.ToLower(key[:1])+key[1:])
	if !ok || p.Introduced == "" {
		return Version{}, false
	}
	return Parse(p.Introduced), true
}
