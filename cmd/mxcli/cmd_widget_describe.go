// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"strings"

	"github.com/mendixlabs/mxcli/mdl/executor"
	"github.com/spf13/cobra"
)

var widgetDescribeCmd = &cobra.Command{
	Use:   "describe <widget>",
	Short: "Inspect a pluggable widget's discovered format",
	Long: `Show the format mxcli has discovered for a pluggable widget: its expected
properties (key, type, caption, category, required, default, enum options) and the
dynamic property rules (which properties the widget's editor hides under which
configuration) lifted from the widget package's editorConfig.

The widget can be named by its MDL keyword (e.g. COMBOBOX, DATAGRID2) or its full
widget id (e.g. com.mendix.widget.web.combobox.Combobox).

With -p, the properties and dynamic rules come from the widget package actually
installed in the project (widgets/*.mpk) — the version-accurate "discovered" format,
including marketplace widgets mxcli has no built-in knowledge of. Without -p, they
come from mxcli's embedded template for that widget.`,
	Example: `  mxcli widget describe COMBOBOX
  mxcli widget describe DATAGRID2 -p app.mpr
  mxcli widget describe com.mendix.widget.web.combobox.Combobox -p app.mpr --format json`,
	Args: cobra.ExactArgs(1),
	RunE: runWidgetDescribe,
}

func init() {
	widgetDescribeCmd.Flags().StringP("project", "p", "", "Path to .mpr project file (use the project's installed widget version)")
	widgetDescribeCmd.Flags().String("format", "text", "Output format: text or json")
	widgetCmd.AddCommand(widgetDescribeCmd)
}

// describedProperty is one property of a widget's discovered format.
// The description itself is built by executor.DescribeWidget, which the MDL
// statement `DESCRIBE WIDGET x` also calls. One code path on purpose: a widget
// was the only MDL extension point with no in-language DESCRIBE, and the
// generated documentation that filled the gap could drift from what the parser
// accepts (mendixlabs/mxcli#1036). Two implementations would reopen that.
func runWidgetDescribe(cmd *cobra.Command, args []string) error {
	projectPath, _ := cmd.Flags().GetString("project")
	format, _ := cmd.Flags().GetString("format")

	desc, err := executor.DescribeWidget(args[0], projectPath)
	if err != nil {
		return err
	}

	if strings.EqualFold(format, "json") {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(desc)
	}
	executor.PrintWidgetDescription(cmd.OutOrStdout(), *desc)
	return nil
}
