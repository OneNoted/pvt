package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/nodeops"
	"github.com/OneNoted/pvt/internal/ui"
)

var nodeExecute bool
var nodeReplacement string

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Plan and run node lifecycle operations",
}

func init() {
	rootCmd.AddCommand(nodeCmd)
	for _, action := range []string{"add", "drain", "reboot", "remove", "replace"} {
		action := action
		command := &cobra.Command{
			Use:   action + " [node]",
			Short: action + " a configured node",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return runNodeAction(action, args[0])
			},
		}
		command.Flags().BoolVar(&nodeExecute, "execute", false, "execute supported commands instead of printing the plan")
		if action == "replace" {
			command.Flags().StringVar(&nodeReplacement, "replacement", "", "configured replacement node name")
		}
		nodeCmd.AddCommand(command)
	}
}

func runNodeAction(action, nodeName string) error {
	_, cfg, err := loadConfig()
	if err != nil {
		return err
	}

	cluster, node, err := findConfiguredNode(cfg, nodeName)
	if err != nil {
		return err
	}

	steps := nodeops.Plan(action, cluster, node, nodeReplacement)
	if len(steps) == 0 {
		return fmt.Errorf("unknown node action %q", action)
	}

	tbl := ui.NewTable("Step", "Command", "Detail")
	for _, step := range steps {
		ui.AddRow(tbl, fmt.Sprintf("%d", step.Order), step.Command, step.Detail)
	}
	tbl.Render(os.Stdout)

	if !nodeExecute {
		fmt.Println("Plan only. Re-run with --execute for supported direct actions.")
		return nil
	}
	if action != "drain" && action != "reboot" {
		return fmt.Errorf("%s is plan-only in this version", action)
	}
	for _, step := range steps {
		if err := runShellStep(step); err != nil {
			return err
		}
	}
	return nil
}

func findConfiguredNode(cfg *config.Config, name string) (config.ClusterConfig, config.NodeConfig, error) {
	for _, cluster := range cfg.Clusters {
		for _, node := range cluster.Nodes {
			if node.Name == name {
				return cluster, node, nil
			}
		}
	}
	return config.ClusterConfig{}, config.NodeConfig{}, fmt.Errorf("node %q not found in config", name)
}

func runShellStep(step nodeops.Step) error {
	if len(step.Args) == 0 {
		return nil
	}
	for _, arg := range step.Args[1:] {
		if strings.ContainsAny(arg, "\t\n\r") {
			return fmt.Errorf("refusing to execute command with control whitespace in argument %q", arg)
		}
	}
	cmd := exec.Command(step.Args[0], step.Args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
