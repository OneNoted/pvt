package rules_test

import (
	"testing"

	"github.com/OneNoted/pvt/internal/proxmox"
	"github.com/OneNoted/pvt/internal/rules"
)

// validVM returns a VM config that passes all rules.
func validVM() *proxmox.VMConfig {
	return &proxmox.VMConfig{
		VMID:    100,
		Name:    "talos-cp-1",
		Node:    "hermes",
		CPU:     "host",
		Sockets: 1,
		Cores:   4,
		Memory:  4096,
		Balloon: 0,
		SCSIHW:  "virtio-scsi-pci",
		Machine: "q35",
		Agent:   "enabled=1",
		Net:     map[int]string{0: "virtio=AA:BB:CC:DD:EE:FF,bridge=vmbr0"},
		SCSI:    map[int]string{0: "local-lvm:vm-100-disk-0,size=50G"},
		IDE:     map[int]string{},
		Serial:  map[int]string{0: "socket"},
	}
}

func TestCPUTypeRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.CPUTypeRule{}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d: %v", len(findings), findings)
	}
}

func TestCPUTypeRule_Fail(t *testing.T) {
	tests := []struct {
		name string
		cpu  string
	}{
		{"kvm64", "kvm64"},
		{"empty defaults to kvm64", ""},
		{"x86-64-v2-AES", "x86-64-v2-AES"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := validVM()
			vm.CPU = tt.cpu
			rule := &rules.CPUTypeRule{}
			findings := rule.Check(vm)
			if len(findings) != 1 {
				t.Errorf("expected 1 finding, got %d", len(findings))
			}
			if findings[0].Severity != rules.SeverityError {
				t.Errorf("expected ERROR severity, got %s", findings[0].Severity)
			}
		})
	}
}

func TestSCSIHWRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.SCSIHWRule{}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestSCSIHWRule_Fail(t *testing.T) {
	vm := validVM()
	vm.SCSIHW = "virtio-scsi-single"
	rule := &rules.SCSIHWRule{}
	findings := rule.Check(vm)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestMemoryMinRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.MemoryMinRule{MinMiB: 2048}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestMemoryMinRule_Fail(t *testing.T) {
	vm := validVM()
	vm.Memory = 1024
	rule := &rules.MemoryMinRule{MinMiB: 2048}
	findings := rule.Check(vm)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestBalloonRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.BalloonRule{}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestBalloonRule_Fail(t *testing.T) {
	vm := validVM()
	vm.Balloon = 512
	rule := &rules.BalloonRule{}
	findings := rule.Check(vm)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != rules.SeverityWarn {
		t.Errorf("expected WARN severity, got %s", findings[0].Severity)
	}
}

func TestNetworkModelRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.NetworkModelRule{}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestNetworkModelRule_Fail(t *testing.T) {
	vm := validVM()
	vm.Net[0] = "e1000=AA:BB:CC:DD:EE:FF,bridge=vmbr0"
	rule := &rules.NetworkModelRule{}
	findings := rule.Check(vm)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestQEMUAgentRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.QEMUAgentRule{}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestQEMUAgentRule_Fail(t *testing.T) {
	vm := validVM()
	vm.Agent = ""
	rule := &rules.QEMUAgentRule{}
	findings := rule.Check(vm)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestMachineTypeRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.MachineTypeRule{}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestMachineTypeRule_Fail(t *testing.T) {
	vm := validVM()
	vm.Machine = ""
	rule := &rules.MachineTypeRule{}
	findings := rule.Check(vm)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestSerialConsoleRule_Pass(t *testing.T) {
	vm := validVM()
	rule := &rules.SerialConsoleRule{}
	findings := rule.Check(vm)
	if len(findings) != 0 {
		t.Errorf("expected no findings, got %d", len(findings))
	}
}

func TestSerialConsoleRule_Fail(t *testing.T) {
	vm := validVM()
	vm.Serial = map[int]string{}
	rule := &rules.SerialConsoleRule{}
	findings := rule.Check(vm)
	if len(findings) != 1 {
		t.Errorf("expected 1 finding, got %d", len(findings))
	}
}

func TestDefaultRegistry_AllRulesPass(t *testing.T) {
	vm := validVM()
	reg := rules.DefaultRegistry()
	findings := reg.Validate(vm)
	if len(findings) != 0 {
		for _, f := range findings {
			t.Errorf("[%s] %s: %s", f.Severity, f.Rule, f.Message)
		}
	}
}

func TestDefaultRegistry_RuleCount(t *testing.T) {
	reg := rules.DefaultRegistry()
	all := reg.All()
	if len(all) != 8 {
		t.Errorf("expected 8 built-in rules, got %d", len(all))
	}
}
