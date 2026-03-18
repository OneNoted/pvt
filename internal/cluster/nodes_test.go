package cluster

import (
	"testing"

	"github.com/OneNoted/pvt/internal/config"
)

var testNodes = []config.NodeConfig{
	{Name: "cp-1", Role: "controlplane", IP: "10.0.0.1"},
	{Name: "cp-2", Role: "controlplane", IP: "10.0.0.2"},
	{Name: "worker-1", Role: "worker", IP: "10.0.0.10"},
	{Name: "worker-2", Role: "worker", IP: "10.0.0.11"},
}

func TestControlPlaneNodes(t *testing.T) {
	got := ControlPlaneNodes(testNodes)
	if len(got) != 2 {
		t.Fatalf("ControlPlaneNodes() returned %d nodes, want 2", len(got))
	}
	if got[0].Name != "cp-1" || got[1].Name != "cp-2" {
		t.Errorf("ControlPlaneNodes() = %v, want cp-1 and cp-2", got)
	}
}

func TestWorkerNodes(t *testing.T) {
	got := WorkerNodes(testNodes)
	if len(got) != 2 {
		t.Fatalf("WorkerNodes() returned %d nodes, want 2", len(got))
	}
	if got[0].Name != "worker-1" || got[1].Name != "worker-2" {
		t.Errorf("WorkerNodes() = %v, want worker-1 and worker-2", got)
	}
}

func TestNodeIPs(t *testing.T) {
	got := NodeIPs(testNodes)
	want := []string{"10.0.0.1", "10.0.0.2", "10.0.0.10", "10.0.0.11"}
	if len(got) != len(want) {
		t.Fatalf("NodeIPs() returned %d IPs, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("NodeIPs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlPlaneNodes_Empty(t *testing.T) {
	workers := []config.NodeConfig{
		{Name: "w-1", Role: "worker", IP: "10.0.0.1"},
	}
	got := ControlPlaneNodes(workers)
	if len(got) != 0 {
		t.Errorf("ControlPlaneNodes() with no CPs returned %d nodes", len(got))
	}
}

func TestWorkerNodes_Empty(t *testing.T) {
	cps := []config.NodeConfig{
		{Name: "cp-1", Role: "controlplane", IP: "10.0.0.1"},
	}
	got := WorkerNodes(cps)
	if len(got) != 0 {
		t.Errorf("WorkerNodes() with no workers returned %d nodes", len(got))
	}
}
