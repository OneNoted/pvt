package cmd

import (
	"testing"

	"github.com/OneNoted/pvt/internal/config"
	"github.com/OneNoted/pvt/internal/health"
)

func TestUpgradeReportNodeStatusPostflightFailures(t *testing.T) {
	tests := []struct {
		name string
		node health.NodeSnapshot
		want string
	}{
		{
			name: "version mismatch",
			node: health.NodeSnapshot{Config: config.NodeConfig{Name: "cp-1"}, VMStatus: "running", TalosVersion: "v1.0.0"},
			want: "version mismatch",
		},
		{
			name: "talos unavailable",
			node: health.NodeSnapshot{Config: config.NodeConfig{Name: "cp-1"}, VMStatus: "running"},
			want: "talos unavailable",
		},
		{
			name: "vm stopped",
			node: health.NodeSnapshot{Config: config.NodeConfig{Name: "cp-1"}, VMStatus: "stopped", TalosVersion: "v1.2.0"},
			want: "vm stopped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, failed := upgradeReportNodeStatus(tt.node, true, "v1.2.0")
			if got != tt.want || !failed {
				t.Fatalf("upgradeReportNodeStatus() = %q, %v; want %q, true", got, failed, tt.want)
			}
		})
	}
}

func TestUpgradeReportNodeStatusReady(t *testing.T) {
	node := health.NodeSnapshot{Config: config.NodeConfig{Name: "cp-1"}, VMStatus: "running", TalosVersion: "v1.2.0"}
	got, failed := upgradeReportNodeStatus(node, true, "v1.2.0")
	if got != "ready" || failed {
		t.Fatalf("upgradeReportNodeStatus() = %q, %v; want ready, false", got, failed)
	}
}
