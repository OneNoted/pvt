package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/drift"
	"github.com/OneNoted/pvt/internal/health"
	"github.com/OneNoted/pvt/internal/ui"
)

var driftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect drift between pvt config and live cluster state",
	RunE:  runDrift,
}

func init() {
	rootCmd.AddCommand(driftCmd)
}

func runDrift(cmd *cobra.Command, args []string) error {
	cfgPath, cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := liveContext()
	defer cancel()
	snapshot := health.Gather(ctx, cfgPath, cfg)
	findings := drift.Detect(snapshot)
	if len(findings) == 0 {
		fmt.Println("No drift detected.")
		return nil
	}

	tbl := ui.NewTable("Severity", "Cluster", "Node", "Kind", "Message", "Fix")
	for _, finding := range findings {
		ui.AddRow(tbl, finding.Severity.String(), finding.Cluster, finding.Node, finding.Kind, finding.Message, finding.Fix)
	}
	tbl.Render(os.Stdout)

	if drift.HasErrors(findings) {
		return fmt.Errorf("drift detected")
	}
	return nil
}
