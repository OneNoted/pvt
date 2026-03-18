package rules

import (
	"fmt"

	"github.com/OneNoted/pvt/internal/proxmox"
)

// MemoryMinRule checks that the VM has at least a minimum amount of memory.
type MemoryMinRule struct {
	MinMiB int
}

func (r *MemoryMinRule) Name() string              { return "memory-min" }
func (r *MemoryMinRule) Description() string       { return "Minimum memory requirement for Talos nodes" }
func (r *MemoryMinRule) DefaultSeverity() Severity { return SeverityError }

func (r *MemoryMinRule) Check(vm *proxmox.VMConfig) []Finding {
	if vm.Memory >= r.MinMiB {
		return nil
	}

	return []Finding{{
		Rule:     r.Name(),
		Severity: r.DefaultSeverity(),
		Message:  fmt.Sprintf("Memory is %d MiB, minimum is %d MiB", vm.Memory, r.MinMiB),
		Fix:      fmt.Sprintf("qm set %d -memory %d", vm.VMID, r.MinMiB),
		Current:  fmt.Sprintf("%d", vm.Memory),
		Expected: fmt.Sprintf(">= %d", r.MinMiB),
	}}
}

// BalloonRule checks that memory ballooning is disabled.
// Talos has no balloon agent, so ballooning can cause instability.
type BalloonRule struct{}

func (r *BalloonRule) Name() string { return "balloon" }
func (r *BalloonRule) Description() string {
	return "Memory ballooning should be disabled (Talos has no balloon agent)"
}
func (r *BalloonRule) DefaultSeverity() Severity { return SeverityWarn }

func (r *BalloonRule) Check(vm *proxmox.VMConfig) []Finding {
	if vm.Balloon == 0 {
		return nil
	}

	return []Finding{{
		Rule:     r.Name(),
		Severity: r.DefaultSeverity(),
		Message:  fmt.Sprintf("Memory balloon is set to %d MiB, should be 0 (Talos has no balloon agent)", vm.Balloon),
		Fix:      fmt.Sprintf("qm set %d -balloon 0", vm.VMID),
		Current:  fmt.Sprintf("%d", vm.Balloon),
		Expected: "0",
	}}
}
