package config_test

import (
	"strings"
	"testing"

	"github.com/OneNoted/pvt/internal/config"
)

const validConfig = `
version: "1"
proxmox:
  clusters:
    - name: homelab
      endpoint: "https://proxmox.local:8006"
      token_id: "pvt@pve!token"
      token_secret: "secret"
      tls_verify: true
clusters:
  - name: apollo
    proxmox_cluster: homelab
    endpoint: "https://192.168.50.120:6443"
    nodes:
      - name: talos-cp-1
        role: controlplane
        proxmox_vmid: 100
        proxmox_node: hermes
        ip: "192.168.50.120"
`

func TestParse_Valid(t *testing.T) {
	cfg, err := config.Parse([]byte(validConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Version != "1" {
		t.Errorf("expected version \"1\", got %q", cfg.Version)
	}
	if len(cfg.Clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(cfg.Clusters))
	}
	if cfg.Clusters[0].Name != "apollo" {
		t.Errorf("expected cluster name \"apollo\", got %q", cfg.Clusters[0].Name)
	}
}

func TestParse_MissingVersion(t *testing.T) {
	input := strings.Replace(validConfig, `version: "1"`, ``, 1)
	_, err := config.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for missing version")
	}
	if !strings.Contains(err.Error(), "version is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_InvalidVersion(t *testing.T) {
	input := strings.Replace(validConfig, `version: "1"`, `version: "99"`, 1)
	_, err := config.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid version")
	}
	if !strings.Contains(err.Error(), "unsupported version") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_MissingProxmoxCluster(t *testing.T) {
	input := `
version: "1"
proxmox:
  clusters: []
clusters:
  - name: test
    proxmox_cluster: missing
    endpoint: "https://test:6443"
    nodes:
      - name: n1
        role: controlplane
        proxmox_vmid: 100
        proxmox_node: pve1
        ip: "1.2.3.4"
`
	_, err := config.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for empty proxmox clusters")
	}
}

func TestParse_InvalidNodeRole(t *testing.T) {
	input := strings.Replace(validConfig, `role: controlplane`, `role: master`, 1)
	_, err := config.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if !strings.Contains(err.Error(), "must be \"controlplane\" or \"worker\"") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_DanglingProxmoxRef(t *testing.T) {
	input := strings.Replace(validConfig, `proxmox_cluster: homelab`, `proxmox_cluster: nonexistent`, 1)
	_, err := config.Parse([]byte(input))
	if err == nil {
		t.Fatal("expected error for dangling proxmox reference")
	}
	if !strings.Contains(err.Error(), "does not match") {
		t.Errorf("unexpected error: %v", err)
	}
}
