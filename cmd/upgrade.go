package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/cluster"
	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/talos"
	"github.com/OneNoted/pvt/internal/ui"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [cluster-name]",
	Short: "Rolling Talos upgrade across all nodes",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runUpgrade,
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
	upgradeCmd.Flags().String("image", "", "Talos installer image (e.g. ghcr.io/siderolabs/installer:v1.12.5)")
	upgradeCmd.Flags().Bool("stage", false, "stage the upgrade (install to disk, reboot later)")
	upgradeCmd.Flags().Bool("force", false, "force upgrade even if pre-flight fails")
	upgradeCmd.Flags().Bool("dry-run", false, "show upgrade plan without executing")
	_ = upgradeCmd.MarkFlagRequired("image")
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.Discover()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	clusterCfg, err := resolveCluster(cfg, args)
	if err != nil {
		return err
	}

	image, _ := cmd.Flags().GetString("image")
	stage, _ := cmd.Flags().GetBool("stage")
	force, _ := cmd.Flags().GetBool("force")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cpNodes := cluster.ControlPlaneNodes(clusterCfg.Nodes)
	workerNodes := cluster.WorkerNodes(clusterCfg.Nodes)
	cpIPs := cluster.NodeIPs(cpNodes)
	workerIPs := cluster.NodeIPs(workerNodes)

	if len(cpNodes) == 0 {
		return fmt.Errorf("cluster %q has no control plane nodes", clusterCfg.Name)
	}

	ctx := context.Background()

	tc, err := talos.NewClient(ctx, cfg.Talos.ConfigPath, cfg.Talos.Context, cpIPs)
	if err != nil {
		return fmt.Errorf("creating Talos client: %w", err)
	}
	defer tc.Close()

	// Pre-flight health check (skip if --force)
	if !force && !dryRun {
		fmt.Println("Running pre-flight health check...")
		if err := tc.WaitHealthy(ctx, cpIPs, workerIPs, 30*time.Second); err != nil {
			return fmt.Errorf("pre-flight health check failed (use --force to skip): %w", err)
		}
		fmt.Println()
	}

	// Determine CP upgrade order (leader last)
	orderedCPNodes := cpNodes
	if !dryRun && len(cpNodes) > 1 {
		ordered, err := cluster.OrderForUpgrade(ctx, tc, cpNodes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: could not determine etcd leader order: %v\n", err)
			fmt.Fprintln(os.Stderr, "  Proceeding with config order.")
		} else {
			orderedCPNodes = ordered
		}
	}

	// Print plan
	fmt.Printf("Upgrade plan for %q\n", clusterCfg.Name)
	fmt.Printf("Image: %s\n\n", image)

	tbl := ui.NewTable("Step", "Node", "Role", "Action")
	step := 1

	for i, node := range workerNodes {
		ui.AddRow(tbl, fmt.Sprintf("%d", step), node.Name, "worker", "upgrade")
		step++
		if i < len(workerNodes)-1 && clusterCfg.Upgrade.PauseBetweenNodes > 0 {
			ui.AddRow(tbl, fmt.Sprintf("%d", step), "-", "-", fmt.Sprintf("pause %s", clusterCfg.Upgrade.PauseBetweenNodes))
			step++
		}
	}

	for i, node := range orderedCPNodes {
		action := "upgrade"
		if i == len(orderedCPNodes)-1 && len(orderedCPNodes) > 1 {
			action = "forfeit leadership + upgrade"
		}
		if len(workerNodes) > 0 && i == 0 && clusterCfg.Upgrade.PauseBetweenNodes > 0 {
			ui.AddRow(tbl, fmt.Sprintf("%d", step), "-", "-", fmt.Sprintf("pause %s", clusterCfg.Upgrade.PauseBetweenNodes))
			step++
		}
		ui.AddRow(tbl, fmt.Sprintf("%d", step), node.Name, "controlplane", action)
		step++
		if i < len(orderedCPNodes)-1 && clusterCfg.Upgrade.PauseBetweenNodes > 0 {
			ui.AddRow(tbl, fmt.Sprintf("%d", step), "-", "-", fmt.Sprintf("pause %s", clusterCfg.Upgrade.PauseBetweenNodes))
			step++
		}
	}

	if clusterCfg.Upgrade.HealthCheckTimeout > 0 {
		ui.AddRow(tbl, fmt.Sprintf("%d", step), "-", "-", fmt.Sprintf("health check (timeout: %s)", clusterCfg.Upgrade.HealthCheckTimeout))
	}

	tbl.Render(os.Stdout)
	fmt.Println()

	if dryRun {
		fmt.Println("Dry run — no changes made.")
		return nil
	}

	ok, err := ui.Confirm(fmt.Sprintf("Upgrade cluster %q to %s?", clusterCfg.Name, image))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Aborted.")
		return nil
	}

	fmt.Println()

	// Etcd snapshot
	if clusterCfg.Upgrade.EtcdBackupBefore {
		snapshotFile := fmt.Sprintf("etcd-snapshot-%s-%s.db", clusterCfg.Name, time.Now().Format("20060102-150405"))
		f, err := os.Create(snapshotFile)
		if err != nil {
			return fmt.Errorf("creating snapshot file: %w", err)
		}

		fmt.Printf("  Taking etcd snapshot...\n")
		if err := tc.EtcdSnapshot(ctx, cpIPs[0], f); err != nil {
			f.Close()
			return err
		}
		f.Close()
		fmt.Printf("  Etcd snapshot saved to %s\n\n", snapshotFile)
	}

	// Upgrade workers
	for i, node := range workerNodes {
		fmt.Printf("  Upgrading %s (%s)...\n", node.Name, node.IP)
		if err := tc.UpgradeNode(ctx, node.IP, image, stage, force); err != nil {
			return err
		}

		if err := waitForNode(ctx, tc, node.IP, clusterCfg.Upgrade.HealthCheckTimeout); err != nil {
			return fmt.Errorf("waiting for %s after upgrade: %w", node.Name, err)
		}
		fmt.Printf("  %s upgraded successfully\n", node.Name)

		if i < len(workerNodes)-1 && clusterCfg.Upgrade.PauseBetweenNodes > 0 {
			fmt.Printf("  Pausing %s...\n", clusterCfg.Upgrade.PauseBetweenNodes)
			time.Sleep(clusterCfg.Upgrade.PauseBetweenNodes)
		}
	}

	// Pause between worker and CP upgrades
	if len(workerNodes) > 0 && len(orderedCPNodes) > 0 && clusterCfg.Upgrade.PauseBetweenNodes > 0 {
		fmt.Printf("  Pausing %s...\n", clusterCfg.Upgrade.PauseBetweenNodes)
		time.Sleep(clusterCfg.Upgrade.PauseBetweenNodes)
	}

	// Upgrade control plane nodes (leader last)
	for i, node := range orderedCPNodes {
		isLast := i == len(orderedCPNodes)-1

		if isLast && len(orderedCPNodes) > 1 {
			fmt.Printf("  Forfeiting etcd leadership on %s...\n", node.Name)
			if err := tc.EtcdForfeitLeadership(ctx, node.IP); err != nil {
				return err
			}
		}

		fmt.Printf("  Upgrading %s (%s)...\n", node.Name, node.IP)
		if err := tc.UpgradeNode(ctx, node.IP, image, stage, force); err != nil {
			return err
		}

		if err := waitForNode(ctx, tc, node.IP, clusterCfg.Upgrade.HealthCheckTimeout); err != nil {
			return fmt.Errorf("waiting for %s after upgrade: %w", node.Name, err)
		}
		fmt.Printf("  %s upgraded successfully\n", node.Name)

		if !isLast && clusterCfg.Upgrade.PauseBetweenNodes > 0 {
			fmt.Printf("  Pausing %s...\n", clusterCfg.Upgrade.PauseBetweenNodes)
			time.Sleep(clusterCfg.Upgrade.PauseBetweenNodes)
		}
	}

	// Final health check
	if clusterCfg.Upgrade.HealthCheckTimeout > 0 {
		fmt.Printf("\n  Running final health check (timeout: %s)...\n", clusterCfg.Upgrade.HealthCheckTimeout)
		if err := tc.WaitHealthy(ctx, cpIPs, workerIPs, clusterCfg.Upgrade.HealthCheckTimeout); err != nil {
			return err
		}
	}

	fmt.Printf("\nCluster %q upgrade to %s complete.\n", clusterCfg.Name, image)
	return nil
}

// waitForNode polls a node's version endpoint until it responds, indicating
// the node has come back after an upgrade reboot.
func waitForNode(ctx context.Context, tc *talos.Client, nodeIP string, timeout time.Duration) error {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := tc.Version(ctx, nodeIP)
		if err == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("node %s did not respond within %s", nodeIP, timeout)
}
