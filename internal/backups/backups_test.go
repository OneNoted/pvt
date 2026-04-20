package backups

import (
	"testing"
	"time"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/proxmox"
)

func TestStaleFiltersByAge(t *testing.T) {
	now := time.Unix(2000, 0)
	entries := []Entry{
		{BackupEntry: proxmox.BackupEntry{VolID: "recent", CTime: uint64(now.Add(-2 * 24 * time.Hour).Unix())}},
		{BackupEntry: proxmox.BackupEntry{VolID: "old", CTime: uint64(now.Add(-40 * 24 * time.Hour).Unix())}},
	}

	got := Stale(entries, 30*24*time.Hour, now)
	if len(got) != 1 || got[0].VolID != "old" {
		t.Fatalf("Stale() = %#v, want only old backup", got)
	}
}

func TestAgeDays(t *testing.T) {
	now := time.Unix(10*24*60*60, 0)
	entry := Entry{BackupEntry: proxmox.BackupEntry{CTime: uint64(now.Add(-3 * 24 * time.Hour).Unix())}}
	if got := AgeDays(entry, now); got != 3 {
		t.Fatalf("AgeDays() = %d, want 3", got)
	}
}

func TestVMIDsForCluster(t *testing.T) {
	got := vmIDsForCluster(config.ClusterConfig{
		Nodes: []config.NodeConfig{
			{Name: "cp-1", ProxmoxVMID: 100},
			{Name: "worker-1", ProxmoxVMID: 101},
		},
	})
	if !got[100] || !got[101] {
		t.Fatalf("vmIDsForCluster() = %#v, want configured VMIDs", got)
	}
	if got[999] {
		t.Fatalf("vmIDsForCluster() includes unrelated VMID")
	}
}
