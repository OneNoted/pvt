package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/doctor"
	"github.com/OneNoted/pvt/internal/ui"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose local pvt configuration and tool access",
	RunE:  runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor(cmd *cobra.Command, args []string) error {
	ctx, cancel := liveContext()
	defer cancel()
	checks := doctor.Run(ctx, cfgFile)

	tbl := ui.NewTable("Severity", "Check", "Status", "Detail")
	for _, check := range checks {
		status := "OK"
		if !check.OK {
			status = "FAIL"
		}
		ui.AddRow(tbl, check.Severity.String(), check.Name, status, check.Detail)
	}
	tbl.Render(os.Stdout)
	fmt.Println(doctor.Summary(checks))

	if doctor.HasErrors(checks) {
		return fmt.Errorf("doctor found error-level failures")
	}
	return nil
}
