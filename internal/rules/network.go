package rules

import (
	"fmt"
	"strings"

	"github.com/OneNoted/pvt/internal/proxmox"
)

// NetworkModelRule checks that network interfaces use the virtio model.
type NetworkModelRule struct{}

func (r *NetworkModelRule) Name() string             { return "network-model" }
func (r *NetworkModelRule) Description() string       { return "Network interfaces should use virtio model" }
func (r *NetworkModelRule) DefaultSeverity() Severity { return SeverityWarn }

func (r *NetworkModelRule) Check(vm *proxmox.VMConfig) []Finding {
	var findings []Finding

	for idx, netCfg := range vm.Net {
		// PVE net config format: "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0,..."
		// The model is the first key before '='
		model := parseNetModel(netCfg)
		if model == "" || model == "virtio" {
			continue
		}

		findings = append(findings, Finding{
			Rule:     r.Name(),
			Severity: r.DefaultSeverity(),
			Message:  fmt.Sprintf("net%d uses model %q, virtio recommended for best performance", idx, model),
			Fix:      fmt.Sprintf("qm set %d -net%d %s", vm.VMID, idx, replaceNetModel(netCfg, "virtio")),
			Current:  model,
			Expected: "virtio",
		})
	}

	return findings
}

// parseNetModel extracts the network model from a PVE net config string.
// Format: "model=MAC,bridge=..." where model is one of: virtio, e1000, rtl8139, etc.
func parseNetModel(netCfg string) string {
	parts := strings.SplitN(netCfg, "=", 2)
	if len(parts) < 2 {
		return ""
	}
	return strings.ToLower(parts[0])
}

// replaceNetModel replaces the network model in a PVE net config string.
func replaceNetModel(netCfg string, newModel string) string {
	eqIdx := strings.Index(netCfg, "=")
	if eqIdx < 0 {
		return netCfg
	}
	return newModel + netCfg[eqIdx:]
}

// QEMUAgentRule checks that the QEMU guest agent is enabled.
type QEMUAgentRule struct{}

func (r *QEMUAgentRule) Name() string             { return "qemu-agent" }
func (r *QEMUAgentRule) Description() string       { return "QEMU guest agent should be enabled" }
func (r *QEMUAgentRule) DefaultSeverity() Severity { return SeverityWarn }

func (r *QEMUAgentRule) Check(vm *proxmox.VMConfig) []Finding {
	// Agent field: "1" or "enabled=1,..." means enabled
	agent := vm.Agent
	if agent == "" || agent == "0" || strings.HasPrefix(agent, "enabled=0") {
		return []Finding{{
			Rule:     r.Name(),
			Severity: r.DefaultSeverity(),
			Message:  "QEMU guest agent is not enabled",
			Fix:      fmt.Sprintf("qm set %d -agent enabled=1", vm.VMID),
			Current:  agent,
			Expected: "enabled=1",
		}}
	}

	return nil
}

// MachineTypeRule checks that the VM uses q35 machine type.
type MachineTypeRule struct{}

func (r *MachineTypeRule) Name() string             { return "machine-type" }
func (r *MachineTypeRule) Description() string       { return "q35 machine type recommended for modern feature support" }
func (r *MachineTypeRule) DefaultSeverity() Severity { return SeverityInfo }

func (r *MachineTypeRule) Check(vm *proxmox.VMConfig) []Finding {
	machine := strings.ToLower(vm.Machine)

	// Empty defaults to i440fx in PVE
	if machine == "" {
		machine = "i440fx"
	}

	if strings.Contains(machine, "q35") {
		return nil
	}

	return []Finding{{
		Rule:     r.Name(),
		Severity: r.DefaultSeverity(),
		Message:  fmt.Sprintf("Machine type is %q, q35 recommended", machine),
		Fix:      fmt.Sprintf("qm set %d -machine q35", vm.VMID),
		Current:  machine,
		Expected: "q35",
	}}
}

// SerialConsoleRule checks that a serial console is configured for boot debugging.
type SerialConsoleRule struct{}

func (r *SerialConsoleRule) Name() string             { return "serial-console" }
func (r *SerialConsoleRule) Description() string       { return "Serial console recommended for boot debugging" }
func (r *SerialConsoleRule) DefaultSeverity() Severity { return SeverityInfo }

func (r *SerialConsoleRule) Check(vm *proxmox.VMConfig) []Finding {
	if _, ok := vm.Serial[0]; ok {
		return nil
	}

	return []Finding{{
		Rule:     r.Name(),
		Severity: r.DefaultSeverity(),
		Message:  "No serial console configured (serial0), recommended for boot debugging",
		Fix:      fmt.Sprintf("qm set %d -serial0 socket", vm.VMID),
		Current:  "not configured",
		Expected: "socket",
	}}
}
