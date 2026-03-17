package rules

import (
	"fmt"
	"strings"

	"github.com/mirceanton/pvt/internal/proxmox"
)

// CPUTypeRule checks that the VM uses cpu type "host".
// kvm64 (the default) lacks x86-64-v2 extensions required by Talos.
type CPUTypeRule struct{}

func (r *CPUTypeRule) Name() string             { return "cpu-type" }
func (r *CPUTypeRule) Description() string       { return "CPU type must be 'host' (kvm64 lacks x86-64-v2)" }
func (r *CPUTypeRule) DefaultSeverity() Severity { return SeverityError }

func (r *CPUTypeRule) Check(vm *proxmox.VMConfig) []Finding {
	cpu := strings.ToLower(vm.CPU)

	// Proxmox default is kvm64 when not set
	if cpu == "" {
		cpu = "kvm64"
	}

	if cpu == "host" {
		return nil
	}

	return []Finding{{
		Rule:     r.Name(),
		Severity: r.DefaultSeverity(),
		Message:  fmt.Sprintf("CPU type is %q, Talos requires x86-64-v2 extensions (use 'host')", cpu),
		Fix:      fmt.Sprintf("qm set %d -cpu host", vm.VMID),
		Current:  cpu,
		Expected: "host",
	}}
}
