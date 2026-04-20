package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/health"
	"github.com/OneNoted/pvt/internal/ui"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Cluster health overview",
}

var statusSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "One-line-per-node status table",
	RunE:  runStatusSummary,
}

func init() {
	rootCmd.AddCommand(statusCmd)
	statusCmd.AddCommand(statusSummaryCmd)

	// Make summary the default subcommand
	statusCmd.RunE = runStatusSummary
}

func runStatusSummary(cmd *cobra.Command, args []string) error {
	cfgPath, cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := liveContext()
	defer cancel()
	snapshot := health.Gather(ctx, cfgPath, cfg)
	for _, cluster := range snapshot.Clusters {
		fmt.Printf("Cluster: %s (%s)\n\n", cluster.Name, cluster.Endpoint)
		tbl := ui.NewTable("Name", "Role", "IP", "PVE Node", "VMID", "Talos Version", "VM Status")

		for _, node := range cluster.Nodes {
			version := node.TalosVersion
			if version == "" {
				version = "unknown"
			}
			vmStatus := node.VMStatus
			if vmStatus == "" {
				vmStatus = "unknown"
			}
			ui.AddRow(tbl,
				node.Config.Name,
				node.Config.Role,
				node.Config.IP,
				node.Config.ProxmoxNode,
				fmt.Sprintf("%d", node.Config.ProxmoxVMID),
				version,
				vmStatus,
			)
		}
		tbl.Render(os.Stdout)
		for _, err := range cluster.Errors {
			fmt.Fprintf(os.Stderr, "  Warning: %s\n", err)
		}
		fmt.Println()
	}

	return nil
}
