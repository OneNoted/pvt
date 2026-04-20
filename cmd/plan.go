package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/drift"
	"github.com/OneNoted/pvt/internal/health"
	"github.com/OneNoted/pvt/internal/ui"
)

var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Generate safe operational plans",
}

var planRemediateCmd = &cobra.Command{
	Use:   "remediate",
	Short: "Print remediation commands for known drift findings",
	RunE:  runPlanRemediate,
}

func init() {
	rootCmd.AddCommand(planCmd)
	planCmd.AddCommand(planRemediateCmd)
}

func runPlanRemediate(cmd *cobra.Command, args []string) error {
	cfgPath, cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := liveContext()
	defer cancel()
	snapshot := health.Gather(ctx, cfgPath, cfg)
	findings := drift.Remediations(drift.Detect(snapshot))
	if len(findings) == 0 {
		fmt.Println("No known remediations available.")
		return nil
	}

	tbl := ui.NewTable("Cluster", "Node", "Kind", "Command")
	for _, finding := range findings {
		ui.AddRow(tbl, finding.Cluster, finding.Node, finding.Kind, finding.Fix)
	}
	tbl.Render(os.Stdout)
	fmt.Println("Plan only. Review commands before applying them on the appropriate Proxmox or workstation host.")
	return nil
}
