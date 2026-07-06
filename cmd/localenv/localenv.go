package localenv

import (
	"github.com/databricks/cli/cmd/root"
	libslocalenv "github.com/databricks/cli/libs/localenv"
	"github.com/spf13/cobra"
)

// New returns the local-env command group. The group, subgroup, and verb names
// come from the single command-name constants in libs/localenv so a rename is a
// one-location change (spec §0 / invariant 8).
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     libslocalenv.CommandGroup,
		Short:   "Manage local development environments matched to Databricks compute",
		GroupID: "development",
		Long: `Manage local development environments matched to a Databricks compute target.

Derives the Python version, databricks-connect version, and dependency
constraints from the selected compute (cluster, serverless, or job) so that
local resolution matches the Databricks runtime.`,
		RunE: root.ReportUnknownSubcommand,
	}
	cmd.AddCommand(newPythonCommand())
	return cmd
}

// newPythonCommand returns the "python" subgroup. It is a parent-only node: with
// no verb it reports an unknown-subcommand error (mirroring the generated command
// groups) rather than doing nothing.
func newPythonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   libslocalenv.CommandSubgroup,
		Short: "Manage the local Python environment",
		Long:  `Manage the local Python environment matched to a Databricks compute target.`,
		RunE:  root.ReportUnknownSubcommand,
	}
	cmd.AddCommand(newSyncCommand())
	return cmd
}
