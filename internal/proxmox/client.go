package proxmox

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"

	pxapi "github.com/luthermonson/go-proxmox"

	"github.com/OneNoted/pvt/internal/config"
)

// Client wraps the Proxmox VE API client with pvt-specific operations.
type Client struct {
	api  *pxapi.Client
	name string
}

// VMClient defines the interface for Proxmox VM operations.
// Implementations can be swapped for testing.
type VMClient interface {
	GetVMConfig(ctx context.Context, node string, vmid int) (*VMConfig, error)
	ListNodeVMs(ctx context.Context, node string) ([]VMSummary, error)
}

// VMSummary is a lightweight VM representation for listing.
type VMSummary struct {
	VMID   int
	Name   string
	Status string
	Node   string
}

// VMConfig holds the VM configuration fields relevant for validation.
type VMConfig struct {
	VMID    int
	Name    string
	Node    string
	CPU     string
	Sockets int
	Cores   int
	Memory  int
	Balloon int
	SCSIHW  string
	Machine string
	Agent   string
	Net     map[int]string
	SCSI    map[int]string
	IDE     map[int]string
	Serial  map[int]string
	Boot    string
}

// NewClient creates a Proxmox API client from a cluster config.
func NewClient(cfg config.ProxmoxCluster) (*Client, error) {
	httpClient := &http.Client{}
	if !cfg.TLSVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	opts := []pxapi.Option{
		pxapi.WithHTTPClient(httpClient),
		pxapi.WithAPIToken(cfg.TokenID, cfg.TokenSecret),
	}

	api := pxapi.NewClient(cfg.Endpoint+"/api2/json", opts...)

	return &Client{
		api:  api,
		name: cfg.Name,
	}, nil
}

// Name returns the cluster name.
func (c *Client) Name() string {
	return c.name
}

// GetVMConfig retrieves the full config for a specific VM.
func (c *Client) GetVMConfig(ctx context.Context, node string, vmid int) (*VMConfig, error) {
	pveNode, err := c.api.Node(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("getting node %q: %w", node, err)
	}

	vm, err := pveNode.VirtualMachine(ctx, vmid)
	if err != nil {
		return nil, fmt.Errorf("getting VM %d on node %q: %w", vmid, node, err)
	}

	cfg := &VMConfig{
		VMID:    vmid,
		Name:    vm.Name,
		Node:    node,
		CPU:     vm.VirtualMachineConfig.CPU,
		Sockets: vm.VirtualMachineConfig.Sockets,
		Cores:   vm.VirtualMachineConfig.Cores,
		Memory:  int(vm.VirtualMachineConfig.Memory),
		Balloon: vm.VirtualMachineConfig.Balloon,
		SCSIHW:  vm.VirtualMachineConfig.SCSIHW,
		Machine: vm.VirtualMachineConfig.Machine,
		Agent:   vm.VirtualMachineConfig.Agent,
		Boot:    vm.VirtualMachineConfig.Boot,
		Net:     make(map[int]string),
		SCSI:    make(map[int]string),
		IDE:     make(map[int]string),
		Serial:  make(map[int]string),
	}

	// Parse network interfaces
	if vm.VirtualMachineConfig.Net0 != "" {
		cfg.Net[0] = vm.VirtualMachineConfig.Net0
	}
	if vm.VirtualMachineConfig.Net1 != "" {
		cfg.Net[1] = vm.VirtualMachineConfig.Net1
	}
	if vm.VirtualMachineConfig.Net2 != "" {
		cfg.Net[2] = vm.VirtualMachineConfig.Net2
	}
	if vm.VirtualMachineConfig.Net3 != "" {
		cfg.Net[3] = vm.VirtualMachineConfig.Net3
	}

	// Parse SCSI devices
	if vm.VirtualMachineConfig.SCSI0 != "" {
		cfg.SCSI[0] = vm.VirtualMachineConfig.SCSI0
	}
	if vm.VirtualMachineConfig.SCSI1 != "" {
		cfg.SCSI[1] = vm.VirtualMachineConfig.SCSI1
	}

	// Parse IDE devices (cloud-init typically on ide2)
	if vm.VirtualMachineConfig.IDE0 != "" {
		cfg.IDE[0] = vm.VirtualMachineConfig.IDE0
	}
	if vm.VirtualMachineConfig.IDE2 != "" {
		cfg.IDE[2] = vm.VirtualMachineConfig.IDE2
	}

	// Parse serial ports
	if vm.VirtualMachineConfig.Serial0 != "" {
		cfg.Serial[0] = vm.VirtualMachineConfig.Serial0
	}

	return cfg, nil
}

// Ping verifies the client can connect to the Proxmox API.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.api.Version(ctx)
	if err != nil {
		return fmt.Errorf("connecting to proxmox cluster %q: %w", c.name, err)
	}
	return nil
}
