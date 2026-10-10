// SPDX-License-Identifier: Apache-2.0

package gen_test

import (
	"testing"

	genappservices "github.com/mendixlabs/mxcli/modelsdk/gen/appservices"
	genauthentication "github.com/mendixlabs/mxcli/modelsdk/gen/authentication"
	genbusinessevents "github.com/mendixlabs/mxcli/modelsdk/gen/businessevents"
	genchangedatacapture "github.com/mendixlabs/mxcli/modelsdk/gen/changedatacapture"
	genclient "github.com/mendixlabs/mxcli/modelsdk/gen/client"
	gencodeactions "github.com/mendixlabs/mxcli/modelsdk/gen/codeactions"
	genconnectorkit "github.com/mendixlabs/mxcli/modelsdk/gen/connectorkit"
	genconstants "github.com/mendixlabs/mxcli/modelsdk/gen/constants"
	gencustomblobdocuments "github.com/mendixlabs/mxcli/modelsdk/gen/customblobdocuments"
	gencustomicons "github.com/mendixlabs/mxcli/modelsdk/gen/customicons"
	gencustomwidgets "github.com/mendixlabs/mxcli/modelsdk/gen/customwidgets"
	gendatabaseconnector "github.com/mendixlabs/mxcli/modelsdk/gen/databaseconnector"
	gendatasets "github.com/mendixlabs/mxcli/modelsdk/gen/datasets"
	gendatatransformers "github.com/mendixlabs/mxcli/modelsdk/gen/datatransformers"
	gendatatypes "github.com/mendixlabs/mxcli/modelsdk/gen/datatypes"
	gendocumenttemplates "github.com/mendixlabs/mxcli/modelsdk/gen/documenttemplates"
	gendomainmodels "github.com/mendixlabs/mxcli/modelsdk/gen/domainmodels"
	genenumerations "github.com/mendixlabs/mxcli/modelsdk/gen/enumerations"
	genexceldataimporter "github.com/mendixlabs/mxcli/modelsdk/gen/exceldataimporter"
	genexportmappings "github.com/mendixlabs/mxcli/modelsdk/gen/exportmappings"
	genexpressions "github.com/mendixlabs/mxcli/modelsdk/gen/expressions"
	genimages "github.com/mendixlabs/mxcli/modelsdk/gen/images"
	genimportmappings "github.com/mendixlabs/mxcli/modelsdk/gen/importmappings"
	genintegrationoverview "github.com/mendixlabs/mxcli/modelsdk/gen/integrationoverview"
	genjavaactions "github.com/mendixlabs/mxcli/modelsdk/gen/javaactions"
	genjavascriptactions "github.com/mendixlabs/mxcli/modelsdk/gen/javascriptactions"
	genjsonstructures "github.com/mendixlabs/mxcli/modelsdk/gen/jsonstructures"
	genkafka "github.com/mendixlabs/mxcli/modelsdk/gen/kafka"
	genmappings "github.com/mendixlabs/mxcli/modelsdk/gen/mappings"
	genmenus "github.com/mendixlabs/mxcli/modelsdk/gen/menus"
	genmessagedefinitions "github.com/mendixlabs/mxcli/modelsdk/gen/messagedefinitions"
	genmetamodelversion "github.com/mendixlabs/mxcli/modelsdk/gen/metamodelversion"
	genmicroflows "github.com/mendixlabs/mxcli/modelsdk/gen/microflows"
	genmlmappings "github.com/mendixlabs/mxcli/modelsdk/gen/mlmappings"
	gennanoflows "github.com/mendixlabs/mxcli/modelsdk/gen/nanoflows"
	gennativepages "github.com/mendixlabs/mxcli/modelsdk/gen/nativepages"
	gennavigation "github.com/mendixlabs/mxcli/modelsdk/gen/navigation"
	genodatapublish "github.com/mendixlabs/mxcli/modelsdk/gen/odatapublish"
	genpages "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	genprojects "github.com/mendixlabs/mxcli/modelsdk/gen/projects"
	genqueues "github.com/mendixlabs/mxcli/modelsdk/gen/queues"
	genregularexpressions "github.com/mendixlabs/mxcli/modelsdk/gen/regularexpressions"
	genreports "github.com/mendixlabs/mxcli/modelsdk/gen/reports"
	genrest "github.com/mendixlabs/mxcli/modelsdk/gen/rest"
	genscheduledevents "github.com/mendixlabs/mxcli/modelsdk/gen/scheduledevents"
	gensecurity "github.com/mendixlabs/mxcli/modelsdk/gen/security"
	genservices "github.com/mendixlabs/mxcli/modelsdk/gen/services"
	gensettings "github.com/mendixlabs/mxcli/modelsdk/gen/settings"
	gentexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	genurl "github.com/mendixlabs/mxcli/modelsdk/gen/url"
	genwebservices "github.com/mendixlabs/mxcli/modelsdk/gen/webservices"
	genworkflows "github.com/mendixlabs/mxcli/modelsdk/gen/workflows"
	genxmlschemas "github.com/mendixlabs/mxcli/modelsdk/gen/xmlschemas"
	"github.com/mendixlabs/mxcli/modelsdk/version"
)

// TestVersionDataCopyMatchesGenerated guards modelsdk/version's plain-data copy
// of every package's VersionInfos (metamodel_versions_gen.go): the storage layer
// reads the copy, because it cannot import these packages. A re-vendored gen
// that changes version data fails here until `go generate ./modelsdk/version`.
//
// Importing every generated package also runs every init — which is how
// webservices' RpcMessagePartElement, whose TypeName property shadowed the
// element's $Type accessors, was found panicking: nothing had ever imported it.
func TestVersionDataCopyMatchesGenerated(t *testing.T) {
	all := map[string]map[string]version.TypeVersionInfo{
		"appservices":         genappservices.VersionInfos,
		"authentication":      genauthentication.VersionInfos,
		"businessevents":      genbusinessevents.VersionInfos,
		"changedatacapture":   genchangedatacapture.VersionInfos,
		"client":              genclient.VersionInfos,
		"codeactions":         gencodeactions.VersionInfos,
		"connectorkit":        genconnectorkit.VersionInfos,
		"constants":           genconstants.VersionInfos,
		"customblobdocuments": gencustomblobdocuments.VersionInfos,
		"customicons":         gencustomicons.VersionInfos,
		"customwidgets":       gencustomwidgets.VersionInfos,
		"databaseconnector":   gendatabaseconnector.VersionInfos,
		"datasets":            gendatasets.VersionInfos,
		"datatransformers":    gendatatransformers.VersionInfos,
		"datatypes":           gendatatypes.VersionInfos,
		"documenttemplates":   gendocumenttemplates.VersionInfos,
		"domainmodels":        gendomainmodels.VersionInfos,
		"enumerations":        genenumerations.VersionInfos,
		"exceldataimporter":   genexceldataimporter.VersionInfos,
		"exportmappings":      genexportmappings.VersionInfos,
		"expressions":         genexpressions.VersionInfos,
		"images":              genimages.VersionInfos,
		"importmappings":      genimportmappings.VersionInfos,
		"integrationoverview": genintegrationoverview.VersionInfos,
		"javaactions":         genjavaactions.VersionInfos,
		"javascriptactions":   genjavascriptactions.VersionInfos,
		"jsonstructures":      genjsonstructures.VersionInfos,
		"kafka":               genkafka.VersionInfos,
		"mappings":            genmappings.VersionInfos,
		"menus":               genmenus.VersionInfos,
		"messagedefinitions":  genmessagedefinitions.VersionInfos,
		"metamodelversion":    genmetamodelversion.VersionInfos,
		"microflows":          genmicroflows.VersionInfos,
		"mlmappings":          genmlmappings.VersionInfos,
		"nanoflows":           gennanoflows.VersionInfos,
		"nativepages":         gennativepages.VersionInfos,
		"navigation":          gennavigation.VersionInfos,
		"odatapublish":        genodatapublish.VersionInfos,
		"pages":               genpages.VersionInfos,
		"projects":            genprojects.VersionInfos,
		"queues":              genqueues.VersionInfos,
		"regularexpressions":  genregularexpressions.VersionInfos,
		"reports":             genreports.VersionInfos,
		"rest":                genrest.VersionInfos,
		"scheduledevents":     genscheduledevents.VersionInfos,
		"security":            gensecurity.VersionInfos,
		"services":            genservices.VersionInfos,
		"settings":            gensettings.VersionInfos,
		"texts":               gentexts.VersionInfos,
		"url":                 genurl.VersionInfos,
		"webservices":         genwebservices.VersionInfos,
		"workflows":           genworkflows.VersionInfos,
		"xmlschemas":          genxmlschemas.VersionInfos,
	}
	n := 0
	for pkg, infos := range all {
		for typ, info := range infos {
			for prop, want := range info.Properties {
				got, ok := version.MetamodelProperty(typ, prop)
				if !ok || got != want {
					t.Errorf("%s: %s.%s = %+v (present %v) in modelsdk/version, %+v in gen — run go generate ./modelsdk/version",
						pkg, typ, prop, got, ok, want)
				}
				n++
			}
		}
	}
	if n == 0 {
		t.Fatal("no version data compared")
	}
}
