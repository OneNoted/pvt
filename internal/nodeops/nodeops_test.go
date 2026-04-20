package nodeops

import (
	"strings"
	"testing"

	"github.com/OneNoted/pvt/internal/config"
)

func TestPlanRebootIsCordonRebootUncordon(t *testing.T) {
	node := config.NodeConfig{Name: "worker-1", IP: "10.0.0.11"}
	steps := Plan("reboot", config.ClusterConfig{Name: "lab"}, node, "")
	if len(steps) != 5 {
		t.Fatalf("Plan(reboot) produced %d steps, want 5", len(steps))
	}
	if !strings.Contains(steps[1].Command, "kubectl drain") {
		t.Fatalf("second command = %q, want kubectl drain", steps[1].Command)
	}
	if !strings.Contains(steps[2].Command, "talosctl reboot --nodes 10.0.0.11") {
		t.Fatalf("third command = %q, want talos reboot", steps[2].Command)
	}
	if !strings.Contains(steps[3].Command, "kubectl wait") {
		t.Fatalf("fourth command = %q, want readiness wait", steps[3].Command)
	}
	if len(steps[0].Args) == 0 || steps[0].Args[len(steps[0].Args)-1] != "worker-1" {
		t.Fatalf("first step args = %#v, want structured node argument", steps[0].Args)
	}
}

func TestPlanReplaceUsesReplacementWhenProvided(t *testing.T) {
	node := config.NodeConfig{Name: "old-worker"}
	steps := Plan("replace", config.ClusterConfig{Name: "lab"}, node, "new-worker")
	if len(steps) == 0 || !strings.Contains(steps[0].Command, "new-worker") {
		t.Fatalf("Plan(replace) first step = %#v, want replacement node", steps)
	}
	if strings.Contains(steps[len(steps)-1].Command, "--execute") {
		t.Fatalf("replace removal step = %q, want plan-only remove", steps[len(steps)-1].Command)
	}
}
