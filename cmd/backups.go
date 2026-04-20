package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/OneNoted/pvt/internal/backups"
	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/proxmox"
	"github.com/OneNoted/pvt/internal/ui"
)

var backupsOlderThanDays int
var backupsExecute bool

const maxBackupRetentionDays = 36500

var backupsCmd = &cobra.Command{
	Use:   "backups",
	Short: "Inspect and manage Proxmox backup retention",
}

var backupsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List Proxmox backup entries",
	RunE:  runBackupsList,
}

var backupsStaleCmd = &cobra.Command{
	Use:   "stale",
	Short: "List Proxmox backup entries older than the retention threshold",
	RunE:  runBackupsStale,
}

var backupsPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune stale Proxmox backups",
	RunE:  runBackupsPrune,
}

func init() {
	rootCmd.AddCommand(backupsCmd)
	backupsCmd.AddCommand(backupsListCmd)
	backupsCmd.AddCommand(backupsStaleCmd)
	backupsCmd.AddCommand(backupsPruneCmd)

	for _, command := range []*cobra.Command{backupsStaleCmd, backupsPruneCmd} {
		command.Flags().IntVar(&backupsOlderThanDays, "older-than-days", 30, "backup age threshold in days")
	}
	backupsPruneCmd.Flags().BoolVar(&backupsExecute, "execute", false, "delete stale backups instead of printing the plan")
}

func runBackupsList(cmd *cobra.Command, args []string) error {
	_, cfg, err := loadConfig()
	if err != nil {
		return err
	}
	ctx, cancel := liveContext()
	defer cancel()
	entries, errs := backups.List(ctx, cfg)
	printBackupErrors(errs)
	printBackups(entries)
	return nil
}

func runBackupsStale(cmd *cobra.Command, args []string) error {
	_, cfg, err := loadConfig()
	if err != nil {
		return err
	}
	retention, err := backupRetention()
	if err != nil {
		return err
	}
	ctx, cancel := liveContext()
	defer cancel()
	entries, errs := backups.List(ctx, cfg)
	printBackupErrors(errs)
	printBackups(backups.Stale(entries, retention, time.Now()))
	return nil
}

func runBackupsPrune(cmd *cobra.Command, args []string) error {
	_, cfg, err := loadConfig()
	if err != nil {
		return err
	}
	retention, err := backupRetention()
	if err != nil {
		return err
	}

	ctx, cancel := liveContext()
	defer cancel()
	stale, errs := backups.List(ctx, cfg)
	printBackupErrors(errs)
	stale = backups.Stale(stale, retention, time.Now())
	if len(stale) == 0 {
		fmt.Println("No stale backups found.")
		return nil
	}
	if !backupsExecute {
		printBackups(stale)
		fmt.Println("Dry run. Re-run with --execute to delete these backups.")
		return nil
	}

	clients := proxmoxClientsByName(cfg)
	for _, entry := range stale {
		client := clients[entry.Cluster]
		if client == nil {
			return fmt.Errorf("%s: proxmox client unavailable", entry.Cluster)
		}
		fmt.Printf("Deleting %s on %s/%s\n", entry.VolID, entry.Node, entry.Storage)
		if err := client.DeleteBackup(ctx, entry.BackupEntry); err != nil {
			return err
		}
	}
	return nil
}

func backupRetention() (time.Duration, error) {
	if backupsOlderThanDays < 1 {
		return 0, fmt.Errorf("--older-than-days must be at least 1")
	}
	if backupsOlderThanDays > maxBackupRetentionDays {
		return 0, fmt.Errorf("--older-than-days must be at most %d", maxBackupRetentionDays)
	}
	return time.Duration(backupsOlderThanDays) * 24 * time.Hour, nil
}

func printBackups(entries []backups.Entry) {
	tbl := ui.NewTable("Cluster", "Node", "Storage", "VMID", "Age", "Size", "VolID")
	now := time.Now()
	for _, entry := range entries {
		ui.AddRow(tbl,
			entry.Cluster,
			entry.Node,
			entry.Storage,
			fmt.Sprintf("%d", entry.VMID),
			fmt.Sprintf("%dd", backups.AgeDays(entry, now)),
			fmt.Sprintf("%d", entry.Size),
			entry.VolID,
		)
	}
	tbl.Render(os.Stdout)
}

func printBackupErrors(errs []string) {
	for _, err := range errs {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", err)
	}
}

func proxmoxClientsByName(cfg *config.Config) map[string]*proxmox.Client {
	pxByName := map[string]config.ProxmoxCluster{}
	for _, cluster := range cfg.Proxmox.Clusters {
		pxByName[cluster.Name] = cluster
	}
	out := map[string]*proxmox.Client{}
	for _, cluster := range cfg.Clusters {
		if out[cluster.Name] != nil {
			continue
		}
		client, err := proxmox.NewClient(pxByName[cluster.ProxmoxCluster])
		if err == nil {
			out[cluster.Name] = client
		}
	}
	return out
}
