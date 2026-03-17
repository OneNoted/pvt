package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/proxmox"
	"github.com/OneNoted/pvt/internal/rules"
	"github.com/OneNoted/pvt/internal/ui"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Pre-flight VM validation",
}

var validateVMsCmd = &cobra.Command{
	Use:   "vms",
	Short: "Validate all VMs in the cluster against best practices",
	RunE:  runValidateVMs,
}

var validateVMCmd = &cobra.Command{
	Use:   "vm [name]",
	Short: "Validate a specific VM",
	Args:  cobra.ExactArgs(1),
	RunE:  runValidateVM,
}

var validateConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Validate pvt config file (alias for pvt config validate)",
	RunE:  runConfigValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
	validateCmd.AddCommand(validateVMsCmd)
	validateCmd.AddCommand(validateVMCmd)
	validateCmd.AddCommand(validateConfigCmd)
}

func runValidateVMs(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.Discover()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	reg := rules.DefaultRegistry()

	totalErrors := 0
	totalWarnings := 0
	totalInfo := 0

	for _, cluster := range cfg.Clusters {
		fmt.Printf("Cluster: %s\n", cluster.Name)
		fmt.Printf("  Proxmox cluster: %s\n\n", cluster.ProxmoxCluster)

		// Find the Proxmox cluster config
		var pxCfg config.ProxmoxCluster
		for _, pc := range cfg.Proxmox.Clusters {
			if pc.Name == cluster.ProxmoxCluster {
				pxCfg = pc
				break
			}
		}

		pxClient, err := proxmox.NewClient(pxCfg)
		if err != nil {
			return fmt.Errorf("connecting to Proxmox %q: %w", pxCfg.Name, err)
		}

		if err := pxClient.Ping(ctx); err != nil {
			return fmt.Errorf("Proxmox API unreachable: %w", err)
		}

		for _, node := range cluster.Nodes {
			fmt.Printf("  Validating %s (VMID %d on %s)...\n", node.Name, node.ProxmoxVMID, node.ProxmoxNode)

			vmCfg, err := pxClient.GetVMConfig(ctx, node.ProxmoxNode, node.ProxmoxVMID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "    Error: could not retrieve VM config: %v\n", err)
				totalErrors++
				continue
			}

			findings := reg.Validate(vmCfg)
			e, w, i := printFindings(findings)
			totalErrors += e
			totalWarnings += w
			totalInfo += i

			if len(findings) == 0 {
				fmt.Println("    All checks passed.")
			}
			fmt.Println()
		}
	}

	printSummary(totalErrors, totalWarnings, totalInfo)

	if totalErrors > 0 {
		return fmt.Errorf("validation failed with %d error(s)", totalErrors)
	}

	return nil
}

func runValidateVM(cmd *cobra.Command, args []string) error {
	vmName := args[0]

	cfgPath, err := config.Discover()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}

	ctx := context.Background()
	reg := rules.DefaultRegistry()

	for _, cluster := range cfg.Clusters {
		for _, node := range cluster.Nodes {
			if node.Name != vmName {
				continue
			}

			var pxCfg config.ProxmoxCluster
			for _, pc := range cfg.Proxmox.Clusters {
				if pc.Name == cluster.ProxmoxCluster {
					pxCfg = pc
					break
				}
			}

			pxClient, err := proxmox.NewClient(pxCfg)
			if err != nil {
				return fmt.Errorf("connecting to Proxmox: %w", err)
			}

			if err := pxClient.Ping(ctx); err != nil {
				return fmt.Errorf("Proxmox API unreachable: %w", err)
			}

			fmt.Printf("Validating %s (VMID %d on %s)...\n", node.Name, node.ProxmoxVMID, node.ProxmoxNode)

			vmCfg, err := pxClient.GetVMConfig(ctx, node.ProxmoxNode, node.ProxmoxVMID)
			if err != nil {
				return fmt.Errorf("could not retrieve VM config: %w", err)
			}

			findings := reg.Validate(vmCfg)
			e, w, _ := printFindings(findings)

			if len(findings) == 0 {
				fmt.Println("  All checks passed.")
			}

			if e > 0 {
				return fmt.Errorf("validation failed with %d error(s)", e)
			}
			if w > 0 {
				fmt.Printf("\nValidation passed with %d warning(s).\n", w)
			} else {
				fmt.Println("\nValidation passed.")
			}

			return nil
		}
	}

	return fmt.Errorf("VM %q not found in any configured cluster", vmName)
}

func printFindings(findings []rules.Finding) (errors, warnings, infos int) {
	if len(findings) == 0 {
		return 0, 0, 0
	}

	tbl := ui.NewTable("Severity", "Rule", "Message", "Fix")
	for _, f := range findings {
		switch f.Severity {
		case rules.SeverityError:
			errors++
		case rules.SeverityWarn:
			warnings++
		case rules.SeverityInfo:
			infos++
		}

		ui.AddRow(tbl,
			f.Severity.String(),
			f.Rule,
			f.Message,
			f.Fix,
		)
	}

	tbl.Render(os.Stdout)
	return errors, warnings, infos
}

func printSummary(errors, warnings, infos int) {
	fmt.Printf("Summary: %d error(s), %d warning(s), %d info(s)\n", errors, warnings, infos)
}
