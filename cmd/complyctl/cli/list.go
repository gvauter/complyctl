// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/oscal-compass/oscal-sdk-go/validation"
	"github.com/spf13/cobra"

	"github.com/complytime/complyctl/cmd/complyctl/option"
	"github.com/complytime/complyctl/internal/complytime"
	"github.com/complytime/complyctl/internal/terminal"
)

// listOptions defines options for the "list" subcommand
type listOptions struct {
	*option.Common
	// print a plain table only
	plain bool
}

// listCmd creates a new cobra.Command for the "list" subcommand
func listCmd(common *option.Common) *cobra.Command {
	listOpts := &listOptions{
		Common: common,
	}
	cmd := &cobra.Command{
		Use:          "list [flags]",
		Short:        "List information about supported frameworks and components.",
		SilenceUsage: true,
		Example:      "complyctl list",
		Args:         cobra.NoArgs,
		RunE:         func(_ *cobra.Command, _ []string) error { return runList(listOpts) },
	}
	cmd.Flags().BoolVarP(&listOpts.plain, "plain", "p", false, "print the table with minimal formatting")
	return cmd
}

func runList(opts *listOptions) error {
	appDir, err := complytime.NewApplicationDirectory(true, logger)
	if err != nil {
		return err
	}
	logger.Debug(fmt.Sprintf("Using application directory: %s", appDir.AppDir()))

	validator := validation.NewSchemaValidator()
	frameworks, err := complytime.LoadFrameworks(appDir, validator, logger)
	if err != nil {
		return err
	}

	if opts.plain {
		showDefinitionTable(opts.Out, frameworks)
	} else {
		model := showPrettyDefinitionTable(frameworks)
		if _, err := tea.NewProgram(model, tea.WithOutput(opts.Out)).Run(); err != nil {
			return fmt.Errorf("failed to display component list: %w", err)
		}
	}
	return nil
}

// ShowDefinitionTable prints a plain table with given framework data.
func showDefinitionTable(writer io.Writer, frameworks []complytime.Framework) {
	columns, rows := getDefinitionColumnsAndRows(frameworks)
	terminal.ShowPlainTable(writer, columns, rows)
}

// ShowPrettyDefinitionTable returns a Model to be used with a `bubbletea` Program that
// renders a table with given Framework data.
func showPrettyDefinitionTable(frameworks []complytime.Framework) terminal.Model {
	columns, rows := getDefinitionColumnsAndRows(frameworks)
	tbl := table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(len(rows) + 1),
	)

	tableStyle := table.DefaultStyles()
	tableStyle.Header = tableStyle.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(false)
	tableStyle.Cell = tableStyle.Cell.
		Foreground(lipgloss.Color("250")).
		Bold(true)
	tbl.SetStyles(tableStyle)

	return terminal.Model{
		Table:   tbl,
		HelpMsg: "Choose an option from the Framework ID column to use with complyctl plan.",
	}
}

// getDefinitionColumnsAndRows returns populate columns and row for printing tables.
func getDefinitionColumnsAndRows(frameworks []complytime.Framework) ([]table.Column, []table.Row) {
	var rows []table.Row
	for _, framework := range frameworks {
		row := table.Row{framework.Title, framework.ID, strings.Join(framework.SupportedComponents, ", ")}
		rows = append(rows, row)
	}
	// Sort the rows slice by the framework short name
	sort.SliceStable(rows, func(i, j int) bool { return rows[i][1] < rows[j][1] })

	// Set columns with default widths
	columns := []table.Column{
		{Title: "Title", Width: 30},
		{Title: "Framework ID", Width: 20},
		{Title: "Supported Components", Width: 30},
	}

	// Calculate column width based on rows
	if len(rows) > 0 {
		for _, row := range rows {
			for i, cell := range row {
				if len(cell) > columns[i].Width {
					columns[i].Width = len(cell)
				}
			}
		}
	}

	return columns, rows
}
