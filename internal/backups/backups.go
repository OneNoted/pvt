package backups

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/proxmox"
)

// Entry is a Proxmox backup annotated with cluster context.
type Entry struct {
	Cluster string
	proxmox.BackupEntry
}

// List returns all known PVE backup entries.
func List(ctx context.Context, cfg *config.Config) ([]Entry, []string) {
	var entries []Entry
	var errs []string

	pxByName := make(map[string]config.ProxmoxCluster)
	for _, cluster := range cfg.Proxmox.Clusters {
		pxByName[cluster.Name] = cluster
	}

	seenNodes := map[string]map[string]bool{}
	for _, cluster := range cfg.Clusters {
		pxCfg, ok := pxByName[cluster.ProxmoxCluster]
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: unknown proxmox cluster %q", cluster.Name, cluster.ProxmoxCluster))
			continue
		}
		configuredVMIDs := vmIDsForCluster(cluster)
		client, err := proxmox.NewClient(pxCfg)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: proxmox client: %v", cluster.Name, err))
			continue
		}
		if seenNodes[cluster.ProxmoxCluster] == nil {
			seenNodes[cluster.ProxmoxCluster] = map[string]bool{}
		}
		for _, node := range cluster.Nodes {
			if seenNodes[cluster.ProxmoxCluster][node.ProxmoxNode] {
				continue
			}
			seenNodes[cluster.ProxmoxCluster][node.ProxmoxNode] = true
			backups, err := client.ListBackups(ctx, node.ProxmoxNode)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s/%s: backups: %v", cluster.Name, node.ProxmoxNode, err))
				continue
			}
			for _, backup := range filterConfiguredBackups(backups, configuredVMIDs) {
				entries = append(entries, Entry{Cluster: cluster.Name, BackupEntry: backup})
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Cluster != entries[j].Cluster {
			return entries[i].Cluster < entries[j].Cluster
		}
		if entries[i].Node != entries[j].Node {
			return entries[i].Node < entries[j].Node
		}
		return entries[i].CTime > entries[j].CTime
	})
	return entries, errs
}

func vmIDsForCluster(cluster config.ClusterConfig) map[uint64]bool {
	vmIDs := make(map[uint64]bool, len(cluster.Nodes))
	for _, node := range cluster.Nodes {
		if node.ProxmoxVMID > 0 {
			vmIDs[uint64(node.ProxmoxVMID)] = true
		}
	}
	return vmIDs
}

func filterConfiguredBackups(backups []proxmox.BackupEntry, configuredVMIDs map[uint64]bool) []proxmox.BackupEntry {
	filtered := make([]proxmox.BackupEntry, 0, len(backups))
	for _, backup := range backups {
		if configuredVMIDs[backup.VMID] {
			filtered = append(filtered, backup)
		}
	}
	return filtered
}

// Stale filters entries older than the given duration.
func Stale(entries []Entry, olderThan time.Duration, now time.Time) []Entry {
	cutoff := now.Add(-olderThan).Unix()
	out := []Entry{}
	for _, entry := range entries {
		if int64(entry.CTime) < cutoff {
			out = append(out, entry)
		}
	}
	return out
}

// AgeDays returns a whole-day age for display.
func AgeDays(entry Entry, now time.Time) int {
	if entry.CTime == 0 {
		return 0
	}
	return int(now.Sub(time.Unix(int64(entry.CTime), 0)).Hours() / 24)
}
