// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/complytime/complyctl/internal/complytime"
	"github.com/complytime/complyctl/internal/terminal"
	"github.com/complytime/complyctl/pkg/plugin"
)

type providersOptions struct {
	*Common
	pluginDir string
}

func providersCmd(common *Common) *cobra.Command {
	o := &providersOptions{
		Common: common,
	}
	cmd := &cobra.Command{
		Use:               "providers",
		Short:             "List discovered scanning providers and their health status",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := o.complete(); err != nil {
				return err
			}
			return o.run(cmd.Context())
		},
	}
	return cmd
}

func (o *providersOptions) complete() error {
	var err error
	o.pluginDir, err = complytime.ResolvePluginDir()
	if err != nil {
		return fmt.Errorf("failed to resolve plugin directory: %w", err)
	}
	return nil
}

func (o *providersOptions) run(ctx context.Context) error {
	mgr, err := plugin.NewManager(o.pluginDir, logger)
	if err != nil {
		return fmt.Errorf("plugin manager init failed: %w", err)
	}
	defer mgr.Cleanup()

	if err := mgr.LoadPlugins(); err != nil {
		return fmt.Errorf("plugin discovery failed: %w", err)
	}

	plugins := mgr.ListPlugins()
	if len(plugins) == 0 {
		fmt.Fprintf(os.Stderr, "No scanning providers found in %s\n", o.pluginDir)
		return nil
	}

	rows := buildProviderRows(ctx, plugins, o.pluginDir)
	headers := []string{"PROVIDER ID", "PATH", "STATUS", "VERSION"}
	terminal.ShowPlainTable(os.Stdout, headers, rows)
	return nil
}

func buildProviderRows(ctx context.Context, plugins []*plugin.LoadedPlugin, pluginDir string) [][]string {
	rows := make([][]string, 0, len(plugins))
	for _, lp := range plugins {
		status, version := describePlugin(ctx, lp)
		relPath, relErr := filepath.Rel(pluginDir, lp.Info.ExecutablePath)
		if relErr != nil {
			relPath = lp.Info.ExecutablePath
		}
		rows = append(rows, []string{lp.Info.EvaluatorID, relPath, status, version})
	}
	return rows
}

func describePlugin(ctx context.Context, lp *plugin.LoadedPlugin) (string, string) {
	resp, err := lp.Client.Describe(ctx, &plugin.DescribeRequest{})
	if err != nil {
		return "ERROR", ""
	}
	if !resp.Healthy {
		return "unhealthy", ""
	}
	return "healthy", resp.Version
}
