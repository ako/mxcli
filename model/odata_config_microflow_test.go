// SPDX-License-Identifier: Apache-2.0

package model

import "testing"

// TestODataConfigMicroflowBSONKey guards the version boundary for issue #728:
// Mendix deleted the ConfigurationMicroflow storage field in 11.10.0 and
// replaced it with ConfigurationEntityMicroflow. Writing the wrong key makes
// Studio Pro show "Constants only".
func TestODataConfigMicroflowBSONKey(t *testing.T) {
	cases := []struct {
		major, minor int
		want         string
	}{
		{10, 12, "ConfigurationMicroflow"},
		{11, 9, "ConfigurationMicroflow"},
		{11, 10, "ConfigurationEntityMicroflow"}, // the boundary
		{11, 11, "ConfigurationEntityMicroflow"}, // the reporter's version
		{11, 12, "ConfigurationEntityMicroflow"},
		{12, 0, "ConfigurationEntityMicroflow"},
	}
	for _, c := range cases {
		if got := ODataConfigMicroflowBSONKey(c.major, c.minor); got != c.want {
			t.Errorf("ODataConfigMicroflowBSONKey(%d.%d) = %q, want %q", c.major, c.minor, got, c.want)
		}
	}
}

// TestODataHeadersMicroflowBSONKey guards the headers-microflow slot (issue
// #728): distinct from the configuration slot on 11.10+ (HeaderListMicroflow);
// before 11.10 both shared ConfigurationMicroflow. Writing a headers microflow
// into the configuration slot triggers CE6808.
func TestODataHeadersMicroflowBSONKey(t *testing.T) {
	cases := []struct {
		major, minor int
		want         string
	}{
		{11, 9, "ConfigurationMicroflow"}, // pre-11.10 shared slot
		{11, 10, "HeaderListMicroflow"},   // split introduced
		{11, 11, "HeaderListMicroflow"},   // reporter's version
		{11, 12, "HeaderListMicroflow"},   // doctype project
		{12, 0, "HeaderListMicroflow"},
	}
	for _, c := range cases {
		if got := ODataHeadersMicroflowBSONKey(c.major, c.minor); got != c.want {
			t.Errorf("ODataHeadersMicroflowBSONKey(%d.%d) = %q, want %q", c.major, c.minor, got, c.want)
		}
	}
}
