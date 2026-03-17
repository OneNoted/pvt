package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/talos"
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
	cfgPath, err := config.Discover()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	ctx := context.Background()

	for _, cluster := range cfg.Clusters {
		fmt.Printf("Cluster: %s (%s)\n\n", cluster.Name, cluster.Endpoint)

		// Collect endpoints from node IPs for querying
		var cpEndpoints []string
		for _, n := range cluster.Nodes {
			if n.Role == "controlplane" {
				cpEndpoints = append(cpEndpoints, n.IP)
			}
		}

		if len(cpEndpoints) == 0 {
			fmt.Println("  No control plane nodes configured")
			continue
		}

		tc, err := talos.NewClient(ctx, cfg.Talos.ConfigPath, cfg.Talos.Context, cpEndpoints)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not connect to Talos API: %v\n", err)
			printOfflineTable(cluster)
			continue
		}
		defer tc.Close()

		// Get all node IPs to query
		var allNodes []string
		for _, n := range cluster.Nodes {
			allNodes = append(allNodes, n.IP)
		}

		printNodeTable(ctx, tc, cluster, allNodes)
		fmt.Println()
	}

	return nil
}

func printNodeTable(ctx context.Context, tc *talos.Client, cluster config.ClusterConfig, allNodes []string) {
	tbl := ui.NewTable("Name", "Role", "IP", "PVE Node", "VMID", "Talos Version", "Status")

	// Try to get versions for all nodes — index by both hostname and IP
	versions, vErr := tc.Version(ctx, allNodes...)
	versionMap := make(map[string]string)
	if vErr == nil {
		for _, v := range versions {
			versionMap[v.Node] = v.TalosVersion
			versionMap[v.Endpoint] = v.TalosVersion
		}
	}

	for _, node := range cluster.Nodes {
		ver := "unknown"
		status := "unreachable"

		if v, ok := versionMap[node.Name]; ok {
			ver = v
			status = "ready"
		} else if v, ok := versionMap[node.IP]; ok {
			ver = v
			status = "ready"
		}

		ui.AddRow(tbl,
			node.Name,
			node.Role,
			node.IP,
			node.ProxmoxNode,
			fmt.Sprintf("%d", node.ProxmoxVMID),
			ver,
			status,
		)
	}

	tbl.Render(os.Stdout)
}

func printOfflineTable(cluster config.ClusterConfig) {
	tbl := ui.NewTable("Name", "Role", "IP", "PVE Node", "VMID", "Status")

	for _, node := range cluster.Nodes {
		ui.AddRow(tbl,
			node.Name,
			node.Role,
			node.IP,
			node.ProxmoxNode,
			fmt.Sprintf("%d", node.ProxmoxVMID),
			"offline",
		)
	}

	tbl.Render(os.Stdout)
}
