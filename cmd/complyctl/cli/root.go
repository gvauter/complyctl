// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"os"

	"github.com/hashicorp/go-hclog"
	"github.com/spf13/cobra"

	"github.com/complytime/complyctl/cmd/complyctl/option"
	"github.com/complytime/complyctl/pkg/log"
)

var logger hclog.Logger

func init() {
	logger = log.NewLogger(os.Stdout)
}

func Error(msg string) {
	logger.Error(msg)
}

func enableDebug(opts *option.Common) {
	if opts.Debug {
		logger.SetLevel(hclog.Debug)
	}
}

// New creates a new cobra.Command root for complyctl
func New() *cobra.Command {

	cmd := &cobra.Command{
		Use:           "complyctl [command]",
		Aliases:       []string{"complytime"},
		SilenceErrors: true,
		SilenceUsage:  false,
	}

	opts := option.Common{
		Output: option.Output{
			Out:    cmd.OutOrStdout(),
			ErrOut: cmd.ErrOrStderr(),
		},
	}
	opts.BindFlags(cmd.PersistentFlags())

	cmd.AddCommand(
		versionCmd(&opts),
		scanCmd(&opts),
		generateCmd(&opts),
		planCmd(&opts),
		listCmd(&opts),
		infoCmd(&opts),
		transformCmd(&opts),
	)
	cmd.PersistentPreRun = func(_ *cobra.Command, _ []string) { enableDebug(&opts) }

	return cmd
}
