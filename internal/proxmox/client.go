package proxmox

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"

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

// StorageSummary is a lightweight Proxmox storage representation.
type StorageSummary struct {
	Name    string
	Node    string
	Type    string
	Content string
	Active  bool
	Enabled bool
	Used    uint64
	Total   uint64
	Avail   uint64
}

// BackupEntry represents a vzdump backup volume on Proxmox storage.
type BackupEntry struct {
	VolID   string
	Node    string
	Storage string
	Format  string
	Size    uint64
	CTime   uint64
	VMID    uint64
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

// ListNodeVMs returns VM summaries for a Proxmox node.
func (c *Client) ListNodeVMs(ctx context.Context, node string) ([]VMSummary, error) {
	pveNode, err := c.api.Node(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("getting node %q: %w", node, err)
	}

	vms, err := pveNode.VirtualMachines(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing VMs on node %q: %w", node, err)
	}

	out := make([]VMSummary, 0, len(vms))
	for _, vm := range vms {
		out = append(out, VMSummary{
			VMID:   int(vm.VMID),
			Name:   vm.Name,
			Status: vm.Status,
			Node:   node,
		})
	}
	return out, nil
}

// ListStorages returns storage summaries for a Proxmox node.
func (c *Client) ListStorages(ctx context.Context, node string) ([]StorageSummary, error) {
	pveNode, err := c.api.Node(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("getting node %q: %w", node, err)
	}

	storages, err := pveNode.Storages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing storages on node %q: %w", node, err)
	}

	out := make([]StorageSummary, 0, len(storages))
	for _, storage := range storages {
		out = append(out, StorageSummary{
			Name:    storage.Name,
			Node:    node,
			Type:    storage.Type,
			Content: storage.Content,
			Active:  storage.Active != 0,
			Enabled: storage.Enabled != 0,
			Used:    storage.Used,
			Total:   storage.Total,
			Avail:   storage.Avail,
		})
	}
	return out, nil
}

// ListBackups returns vzdump backup entries from backup-capable storages.
func (c *Client) ListBackups(ctx context.Context, node string) ([]BackupEntry, error) {
	pveNode, err := c.api.Node(ctx, node)
	if err != nil {
		return nil, fmt.Errorf("getting node %q: %w", node, err)
	}

	storages, err := pveNode.Storages(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing storages on node %q: %w", node, err)
	}

	var backups []BackupEntry
	for _, storage := range storages {
		if !strings.Contains(storage.Content, "backup") {
			continue
		}
		content, err := storage.GetContent(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing backup content on %s/%s: %w", node, storage.Name, err)
		}
		for _, item := range content {
			if item.Volid == "" || !isBackupContent(item.Format, item.Volid) {
				continue
			}
			backups = append(backups, BackupEntry{
				VolID:   item.Volid,
				Node:    node,
				Storage: storage.Name,
				Format:  item.Format,
				Size:    item.Size,
				CTime:   uint64(item.Ctime),
				VMID:    item.VMID,
			})
		}
	}
	return backups, nil
}

// DeleteBackup deletes a backup volume from Proxmox storage.
func (c *Client) DeleteBackup(ctx context.Context, backup BackupEntry) error {
	path := fmt.Sprintf("/nodes/%s/storage/%s/content/%s", backup.Node, backup.Storage, url.PathEscape(backup.VolID))
	var result any
	if err := c.api.Delete(ctx, path, &result); err != nil {
		return fmt.Errorf("deleting backup %q: %w", backup.VolID, err)
	}
	return nil
}

func isBackupContent(format, volID string) bool {
	switch format {
	case "vma.zst", "vma.gz", "vma.lzo", "pbs-vm":
		return true
	}
	return strings.Contains(volID, ":backup/")
}

// Ping verifies the client can connect to the Proxmox API.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.api.Version(ctx)
	if err != nil {
		return fmt.Errorf("connecting to proxmox cluster %q: %w", c.name, err)
	}
	return nil
}
