// SPDX-License-Identifier: Apache-2.0

package marketplace

import "testing"

func content(name, publisher string) Content {
	return Content{Publisher: publisher, LatestVersion: &Version{Name: name}}
}

// TestFilterItemsMatchesTheWrittenName is the regression test for a real search
// miss: the module everyone calls "Database Replication" is packaged as
// "DatabaseReplication", and the API exposes only the packaged name. Searching
// the name as written returned nothing, and the user had to guess that the space
// was the problem.
func TestFilterItemsMatchesTheWrittenName(t *testing.T) {
	items := []Content{
		content("DatabaseReplication", "Mendix"),
		content("MxModelReflection", "Mendix"),
		content("Excel Importer", "Community"),
	}
	cases := []struct{ query, want string }{
		{"Database Replication", "DatabaseReplication"},
		{"database replication", "DatabaseReplication"},
		{"databasereplication", "DatabaseReplication"},
		{"replication", "DatabaseReplication"},
		{"database-replication", "DatabaseReplication"},
		{"excelimporter", "Excel Importer"},
		{"Excel Importer", "Excel Importer"},
	}
	for _, c := range cases {
		got := filterItems(items, c.query)
		if len(got) != 1 {
			t.Errorf("%q matched %d items, want 1", c.query, len(got))
			continue
		}
		if got[0].LatestVersion.Name != c.want {
			t.Errorf("%q matched %q, want %q", c.query, got[0].LatestVersion.Name, c.want)
		}
	}
}

// TestFilterItemsStillDiscriminates — normalising separators must not make the
// search match everything.
func TestFilterItemsStillDiscriminates(t *testing.T) {
	items := []Content{content("DatabaseReplication", "Mendix"), content("Excel Importer", "Community")}
	if got := filterItems(items, "workflow"); len(got) != 0 {
		t.Errorf("unrelated query matched %d items", len(got))
	}
	if got := filterItems(items, "mendix"); len(got) != 1 {
		t.Errorf("publisher query matched %d items, want 1", len(got))
	}
}
