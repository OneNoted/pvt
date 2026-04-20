package proxmox

import "testing"

func TestIsBackupContent(t *testing.T) {
	tests := []struct {
		name   string
		format string
		volID  string
		want   bool
	}{
		{name: "vma zst", format: "vma.zst", want: true},
		{name: "pbs", format: "pbs-vm", want: true},
		{name: "volid backup path", format: "raw", volID: "local:backup/vzdump-qemu-100.vma.zst", want: true},
		{name: "iso", format: "iso", volID: "local:iso/talos.iso", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBackupContent(tt.format, tt.volID); got != tt.want {
				t.Fatalf("isBackupContent(%q, %q) = %v, want %v", tt.format, tt.volID, got, tt.want)
			}
		})
	}
}
