package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/machineconfig"
)

var machineConfigCmd = &cobra.Command{
	Use:   "machineconfig",
	Short: "Inspect Talos machine config files",
}

var machineConfigDiffCmd = &cobra.Command{
	Use:   "diff [node]",
	Short: "Diff configured machine config files against another file or directory",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runMachineConfigDiff,
}

var machineConfigAgainst string

func init() {
	rootCmd.AddCommand(machineConfigCmd)
	machineConfigCmd.AddCommand(machineConfigDiffCmd)
	machineConfigDiffCmd.Flags().StringVar(&machineConfigAgainst, "against", "", "file or directory to compare against")
	_ = machineConfigDiffCmd.MarkFlagRequired("against")
}

func runMachineConfigDiff(cmd *cobra.Command, args []string) error {
	_, cfg, err := loadConfig()
	if err != nil {
		return err
	}

	foundDiff := false
	matched := false
	for _, cluster := range cfg.Clusters {
		for _, node := range cluster.Nodes {
			if len(args) > 0 && args[0] != node.Name {
				continue
			}
			matched = true
			left := machineconfig.ResolvePath(cluster.ConfigSource, cluster.Name, node.Name)
			right := comparePath(machineConfigAgainst, left)
			lines, different, err := machineconfig.DiffFiles(left, right)
			if err != nil {
				return err
			}
			if !different {
				fmt.Printf("%s: no diff\n", node.Name)
				continue
			}
			foundDiff = true
			fmt.Printf("%s: %s -> %s\n", node.Name, left, right)
			for _, line := range lines {
				fmt.Fprintln(os.Stdout, line)
			}
		}
	}

	if len(args) > 0 && !matched {
		return fmt.Errorf("node %q not found in config", args[0])
	}
	if len(args) > 0 && !foundDiff {
		fmt.Printf("%s: no diff\n", args[0])
	}
	return nil
}

func comparePath(against, configured string) string {
	info, err := os.Stat(against)
	if err == nil && info.IsDir() {
		return filepath.Join(against, filepath.Base(configured))
	}
	return against
}
