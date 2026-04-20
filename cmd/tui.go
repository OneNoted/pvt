package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch the interactive TUI (vitui)",
	Long:  `Launches vitui, the interactive terminal UI for monitoring and managing your Talos-on-Proxmox cluster.`,
	RunE:  runTUI,
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	binary, err := findVitui()
	if err != nil {
		return fmt.Errorf("vitui binary not found: %w\n\nBuild the Rust TUI by running: cd tui && cargo build --release", err)
	}

	// Build vitui args, forwarding the resolved config path
	var vituiArgs []string
	configPath := cfgFile
	if configPath == "" {
		// Use viper's resolved config path if available
		if f := viper.ConfigFileUsed(); f != "" {
			configPath = f
		}
	}
	if configPath != "" {
		vituiArgs = append(vituiArgs, "--config", configPath)
	}

	proc := exec.Command(binary, vituiArgs...)
	proc.Stdin = os.Stdin
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr

	return proc.Run()
}

// findVitui searches for the vitui binary in standard locations.
func findVitui() (string, error) {
	if override := os.Getenv("PVT_VITUI_BIN"); override != "" {
		if _, err := os.Stat(override); err == nil {
			return override, nil
		}
	}

	// 1. Adjacent to the pvt binary
	self, err := os.Executable()
	selfDir := ""
	if err == nil {
		selfDir = filepath.Dir(self)
		adjacent := filepath.Join(filepath.Dir(self), "vitui")
		if _, err := os.Stat(adjacent); err == nil {
			return adjacent, nil
		}
	}

	// 2. In Rust cargo target directories relative to common roots
	searchRoots := []string{"."}
	if selfDir != "" {
		searchRoots = append(searchRoots, selfDir, filepath.Dir(selfDir))
	}
	for _, root := range searchRoots {
		for _, local := range []string{
			filepath.Join(root, "tui", "target", "release", "vitui"),
			filepath.Join(root, "tui", "target", "debug", "vitui"),
		} {
			if _, err := os.Stat(local); err == nil {
				return local, nil
			}
		}
	}

	return "", fmt.Errorf("not adjacent to pvt binary and not in tui/target/{release,debug}; set PVT_VITUI_BIN to override")
}
