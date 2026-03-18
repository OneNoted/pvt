package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"

	"github.com/OneNoted/pvt/internal/cluster"
	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/machineconfig"
	"github.com/OneNoted/pvt/internal/talos"
	"github.com/OneNoted/pvt/internal/ui"
)

var bootstrapCmd = &cobra.Command{
	Use:   "bootstrap [cluster-name]",
	Short: "Apply machine configs and bootstrap etcd for a new cluster",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runBootstrap,
}

func init() {
	rootCmd.AddCommand(bootstrapCmd)
	bootstrapCmd.Flags().Bool("dry-run", false, "show what would happen without executing")
}

func runBootstrap(cmd *cobra.Command, args []string) error {
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

	dryRun, _ := cmd.Flags().GetBool("dry-run")

	cpNodes := cluster.ControlPlaneNodes(clusterCfg.Nodes)
	workerNodes := cluster.WorkerNodes(clusterCfg.Nodes)

	if len(cpNodes) == 0 {
		return fmt.Errorf("cluster %q has no control plane nodes", clusterCfg.Name)
	}

	// Verify all machine config files exist before proceeding
	var missingConfigs []string
	for _, node := range clusterCfg.Nodes {
		path := machineconfig.ResolvePath(clusterCfg.ConfigSource, clusterCfg.Name, node.Name)
		if _, err := os.Stat(path); err != nil {
			missingConfigs = append(missingConfigs, fmt.Sprintf("  %s: %s", node.Name, path))
		}
	}

	if len(missingConfigs) > 0 && !dryRun {
		fmt.Fprintln(os.Stderr, "Missing machine config files:")
		for _, m := range missingConfigs {
			fmt.Fprintln(os.Stderr, m)
		}
		return fmt.Errorf("resolve missing config files before bootstrapping")
	}

	// Print plan
	fmt.Printf("Bootstrap plan for %q\n\n", clusterCfg.Name)

	tbl := ui.NewTable("Step", "Node", "Role", "Action", "Config File")
	step := 1

	for _, node := range cpNodes {
		path := machineconfig.ResolvePath(clusterCfg.ConfigSource, clusterCfg.Name, node.Name)
		ui.AddRow(tbl, fmt.Sprintf("%d", step), node.Name, node.Role, "apply config", path)
		step++
	}

	ui.AddRow(tbl, fmt.Sprintf("%d", step), cpNodes[0].Name, "controlplane", "bootstrap etcd", "-")
	step++

	for _, node := range workerNodes {
		path := machineconfig.ResolvePath(clusterCfg.ConfigSource, clusterCfg.Name, node.Name)
		ui.AddRow(tbl, fmt.Sprintf("%d", step), node.Name, node.Role, "apply config", path)
		step++
	}

	if clusterCfg.Upgrade.HealthCheckTimeout > 0 {
		ui.AddRow(tbl, fmt.Sprintf("%d", step), "-", "-", "wait for health", fmt.Sprintf("timeout: %s", clusterCfg.Upgrade.HealthCheckTimeout))
	}

	tbl.Render(os.Stdout)
	fmt.Println()

	if dryRun {
		fmt.Println("Dry run — no changes made.")
		return nil
	}

	ok, err := ui.Confirm(fmt.Sprintf("Apply configs and bootstrap cluster %q?", clusterCfg.Name))
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("Aborted.")
		return nil
	}

	fmt.Println()

	ctx := context.Background()
	cpIPs := cluster.NodeIPs(cpNodes)

	tc, err := talos.NewClient(ctx, cfg.Talos.ConfigPath, cfg.Talos.Context, cpIPs)
	if err != nil {
		return fmt.Errorf("creating Talos client: %w", err)
	}
	defer tc.Close()

	// Apply control plane configs
	for _, node := range cpNodes {
		data, err := machineconfig.LoadMachineConfig(clusterCfg.ConfigSource, clusterCfg.Name, node.Name)
		if err != nil {
			return err
		}

		fmt.Printf("  Applying config to %s (%s)...\n", node.Name, node.IP)
		if err := tc.ApplyConfig(ctx, node.IP, data, machineapi.ApplyConfigurationRequest_AUTO); err != nil {
			return err
		}
	}

	// Bootstrap etcd on first CP
	fmt.Printf("  Bootstrapping etcd on %s...\n", cpNodes[0].Name)
	if err := tc.BootstrapEtcd(ctx, cpNodes[0].IP); err != nil {
		return err
	}
	time.Sleep(5 * time.Second)

	// Apply worker configs
	for _, node := range workerNodes {
		data, err := machineconfig.LoadMachineConfig(clusterCfg.ConfigSource, clusterCfg.Name, node.Name)
		if err != nil {
			return err
		}

		fmt.Printf("  Applying config to %s (%s)...\n", node.Name, node.IP)
		if err := tc.ApplyConfig(ctx, node.IP, data, machineapi.ApplyConfigurationRequest_AUTO); err != nil {
			return err
		}
	}

	// Wait for health
	if clusterCfg.Upgrade.HealthCheckTimeout > 0 {
		fmt.Printf("\n  Waiting for cluster health (timeout: %s)...\n", clusterCfg.Upgrade.HealthCheckTimeout)
		workerIPs := cluster.NodeIPs(workerNodes)
		if err := tc.WaitHealthy(ctx, cpIPs, workerIPs, clusterCfg.Upgrade.HealthCheckTimeout); err != nil {
			return err
		}
	}

	fmt.Printf("\nCluster %q bootstrap complete.\n", clusterCfg.Name)
	return nil
}

// resolveCluster finds the target cluster from args or defaults.
func resolveCluster(cfg *config.Config, args []string) (config.ClusterConfig, error) {
	if len(args) > 0 {
		name := args[0]
		for _, c := range cfg.Clusters {
			if c.Name == name {
				return c, nil
			}
		}
		return config.ClusterConfig{}, fmt.Errorf("cluster %q not found in config", name)
	}

	if len(cfg.Clusters) == 1 {
		return cfg.Clusters[0], nil
	}

	return config.ClusterConfig{}, fmt.Errorf("multiple clusters configured — specify a cluster name")
}
