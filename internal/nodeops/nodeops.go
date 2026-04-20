package nodeops

import (
	"fmt"
	"strings"

	"github.com/OneNoted/pvt/internal/config"
)

// Step is one planned node lifecycle operation.
type Step struct {
	Order   int
	Command string
	Args    []string
	Detail  string
}

// Plan builds a conservative plan for a node lifecycle action.
func Plan(action string, cluster config.ClusterConfig, node config.NodeConfig, replacement string) []Step {
	switch action {
	case "drain":
		return []Step{
			newStep(1, []string{"kubectl", "drain", "--ignore-daemonsets", "--delete-emptydir-data", "--", node.Name}, "evict workloads before host-level maintenance"),
		}
	case "reboot":
		return []Step{
			newStep(1, []string{"kubectl", "cordon", "--", node.Name}, "stop new workload placement"),
			newStep(2, []string{"kubectl", "drain", "--ignore-daemonsets", "--delete-emptydir-data", "--", node.Name}, "evict workloads before reboot"),
			newStep(3, []string{"talosctl", "reboot", "--nodes", node.IP}, "reboot Talos node"),
			newStep(4, []string{"kubectl", "wait", "--for=condition=Ready", "--timeout=10m", "node/" + node.Name}, "wait for Kubernetes readiness"),
			newStep(5, []string{"kubectl", "uncordon", "--", node.Name}, "resume scheduling after readiness is confirmed"),
		}
	case "remove":
		return []Step{
			newStep(1, []string{"kubectl", "drain", "--ignore-daemonsets", "--delete-emptydir-data", "--", node.Name}, "evict workloads"),
			newStep(2, []string{"kubectl", "delete", "node", "--", node.Name}, "remove Kubernetes node object"),
			newStep(3, []string{"talosctl", "reset", "--nodes", node.IP, "--graceful=false"}, "reset Talos installation after data backup is confirmed"),
			newStep(4, []string{"qm", "stop", fmt.Sprintf("%d", node.ProxmoxVMID)}, "stop VM in Proxmox"),
		}
	case "replace":
		target := replacement
		if target == "" {
			target = "<new-node-name>"
		}
		return []Step{
			newStep(1, []string{"pvt", "node", "add", target}, "bootstrap replacement node from config"),
			newStep(2, []string{"pvt", "node", "drain", node.Name, "--execute"}, "move workloads off old node"),
			newStep(3, []string{"pvt", "node", "remove", node.Name}, "review the old-node removal plan after replacement is healthy"),
		}
	case "add":
		return []Step{
			newStep(1, []string{"pvt", "validate", "vm", node.Name}, "verify Proxmox VM settings"),
			newStep(2, []string{"pvt", "bootstrap", cluster.Name, "--dry-run"}, "preview machine config application"),
			newStep(3, []string{"talosctl", "apply-config", "--nodes", node.IP, "--file", "<machine-config>"}, "apply node machine config"),
		}
	default:
		return nil
	}
}

func newStep(order int, args []string, detail string) Step {
	return Step{
		Order:   order,
		Command: strings.Join(args, " "),
		Args:    args,
		Detail:  detail,
	}
}
