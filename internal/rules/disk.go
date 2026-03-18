package rules

import (
	"fmt"
	"strings"

	"github.com/OneNoted/pvt/internal/proxmox"
)

// SCSIHWRule checks that the SCSI hardware controller is virtio-scsi-pci.
// virtio-scsi-single causes issues with Talos disk detection.
type SCSIHWRule struct{}

func (r *SCSIHWRule) Name() string              { return "scsihw" }
func (r *SCSIHWRule) Description() string       { return "SCSI HW must be 'virtio-scsi-pci'" }
func (r *SCSIHWRule) DefaultSeverity() Severity { return SeverityError }

func (r *SCSIHWRule) Check(vm *proxmox.VMConfig) []Finding {
	hw := strings.ToLower(vm.SCSIHW)

	// Proxmox default
	if hw == "" {
		hw = "lsi"
	}

	if hw == "virtio-scsi-pci" {
		return nil
	}

	return []Finding{{
		Rule:     r.Name(),
		Severity: r.DefaultSeverity(),
		Message:  fmt.Sprintf("SCSI hardware is %q, should be 'virtio-scsi-pci'", hw),
		Fix:      fmt.Sprintf("qm set %d -scsihw virtio-scsi-pci", vm.VMID),
		Current:  hw,
		Expected: "virtio-scsi-pci",
	}}
}
