package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage pvt configuration",
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a starter pvt config file",
	RunE:  runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Display the resolved configuration",
	RunE:  runConfigShow,
}

var configValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate config file syntax and references",
	RunE:  runConfigValidate,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configValidateCmd)

	configInitCmd.Flags().StringP("output", "o", "pvt.yaml", "output file path")
}

const starterConfig = `version: "1"

proxmox:
  clusters:
    - name: homelab
      endpoint: "https://proxmox.local:8006"
      token_id: "pvt@pve!automation"
      token_secret: "${PVT_PVE_TOKEN}"
      tls_verify: true

talos:
  config_path: "~/.talos/config"
  context: ""

clusters:
  - name: my-cluster
    proxmox_cluster: homelab
    endpoint: "https://192.168.1.100:6443"
    config_source:
      type: directory
      path: "~/talos/my-cluster/"
    nodes:
      - name: talos-cp-1
        role: controlplane
        proxmox_vmid: 100
        proxmox_node: pve1
        ip: "192.168.1.100"
      - name: talos-worker-1
        role: worker
        proxmox_vmid: 101
        proxmox_node: pve1
        ip: "192.168.1.101"
    validation:
      rules:
        cpu_type:
          allowed: ["host"]
        scsihw:
          allowed: ["virtio-scsi-pci"]
    upgrade:
      etcd_backup_before: true
      health_check_timeout: 5m
      pause_between_nodes: 30s
`

func runConfigInit(cmd *cobra.Command, args []string) error {
	output, _ := cmd.Flags().GetString("output")

	// Resolve to absolute path
	absPath, err := filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	if _, err := os.Stat(absPath); err == nil {
		return fmt.Errorf("file already exists: %s (use a different path with -o)", absPath)
	}

	if err := os.WriteFile(absPath, []byte(starterConfig), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("Config file created: %s\n", absPath)
	fmt.Println("Edit the file to match your environment, then run: pvt config validate")
	return nil
}

func runConfigShow(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.Discover()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	fmt.Printf("# Config file: %s\n", cfgPath)
	fmt.Println(string(data))
	return nil
}

func runConfigValidate(cmd *cobra.Command, args []string) error {
	cfgPath, err := config.Discover()
	if err != nil {
		return err
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Printf("Config file: %s\n", cfgPath)
	fmt.Printf("  Version: %s\n", cfg.Version)
	fmt.Printf("  Proxmox clusters: %d\n", len(cfg.Proxmox.Clusters))
	for _, pc := range cfg.Proxmox.Clusters {
		fmt.Printf("    - %s (%s)\n", pc.Name, pc.Endpoint)
	}
	fmt.Printf("  Managed clusters: %d\n", len(cfg.Clusters))
	for _, c := range cfg.Clusters {
		cpCount := 0
		workerCount := 0
		for _, n := range c.Nodes {
			if n.Role == "controlplane" {
				cpCount++
			} else {
				workerCount++
			}
		}
		fmt.Printf("    - %s: %d control plane, %d worker nodes\n", c.Name, cpCount, workerCount)
	}
	fmt.Println("\nConfig is valid.")
	return nil
}
